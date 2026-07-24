package trace

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// records decodes an encoded trace into its typed records, keyed loosely so a
// test can assert over the message stream without re-deriving the schema.
func records(t *testing.T, data []byte) (Session, []Record) {
	t.Helper()
	var sess Session
	var recs []Record
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 1024*1024), 8*1024*1024)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var head struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(line, &head) != nil {
			t.Fatalf("bad json line: %s", line)
		}
		switch head.Type {
		case "session":
			if json.Unmarshal(line, &sess) != nil {
				t.Fatalf("bad session: %s", line)
			}
		case "message":
			var r Record
			if json.Unmarshal(line, &r) != nil {
				t.Fatalf("bad record: %s", line)
			}
			recs = append(recs, r)
		}
	}
	return sess, recs
}

func write(t *testing.T, dir, rel, content string) {
	t.Helper()
	p := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestEncodeChat reconstructs a chat-completions capture: the richest request
// carries the system, user, assistant-with-tool-call, and tool-result turns, and
// the last resp-N.txt carries the final assistant reply that no request echoes.
func TestEncodeChat(t *testing.T) {
	dir := t.TempDir()
	body := map[string]any{
		"messages": []map[string]any{
			{"role": "system", "content": "you are a coder"},
			{"role": "user", "content": "fix the bug"},
			{"role": "assistant", "content": "", "reasoning": "let me look", "tool_calls": []map[string]any{
				{"id": "call_1", "index": 0, "function": map[string]any{"name": "read", "arguments": `{"path":"a.py"}`}},
			}},
			{"role": "tool", "tool_call_id": "call_1", "content": "line one"},
		},
	}
	raw, _ := json.Marshal(map[string]any{"path": "/v1/chat/completions", "body": body})
	write(t, dir, "requests.jsonl", string(raw)+"\n")

	// A short chat SSE stream: reasoning delta, text delta, done.
	sse := strings.Join([]string{
		`data: {"choices":[{"delta":{"reasoning":"the fix is"}}]}`,
		`data: {"choices":[{"delta":{"content":"patched it"}}]}`,
		`data: [DONE]`,
	}, "\n") + "\n"
	write(t, dir, "resp-1.txt", sse)

	data, err := Encode(dir, Header{Model: "m", Scenario: "s", Timestamp: "20260722T101112Z"})
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	sess, recs := records(t, data)
	if sess.Version != 3 {
		t.Fatalf("version = %d", sess.Version)
	}
	if len(recs) != 5 {
		t.Fatalf("want 5 messages, got %d", len(recs))
	}

	// The assistant tool-call turn: a thinking block then a toolCall block whose
	// arguments are a parsed object, not a re-escaped string.
	tc := recs[2]
	if tc.Message.Role != "assistant" || len(tc.Message.Content) != 2 {
		t.Fatalf("assistant turn shape: %+v", tc.Message)
	}
	if tc.Message.Content[0].Type != "thinking" || tc.Message.Content[0].Thinking != "let me look" {
		t.Fatalf("thinking block: %+v", tc.Message.Content[0])
	}
	call := tc.Message.Content[1]
	if call.Type != "toolCall" || call.Name != "read" {
		t.Fatalf("toolCall block: %+v", call)
	}
	var args map[string]string
	if json.Unmarshal(call.Arguments, &args) != nil || args["path"] != "a.py" {
		t.Fatalf("arguments not an object: %s", call.Arguments)
	}

	// The tool result turn.
	if recs[3].Message.Role != "tool" || recs[3].Message.Content[0].Output != "line one" {
		t.Fatalf("tool result: %+v", recs[3].Message)
	}

	// The final reply, decoded from the response stream.
	final := recs[4].Message
	if final.Role != "assistant" {
		t.Fatalf("final role: %s", final.Role)
	}
	if final.Content[0].Type != "thinking" || final.Content[0].Thinking != "the fix is" {
		t.Fatalf("final thinking: %+v", final.Content[0])
	}
	if final.Content[1].Type != "text" || final.Content[1].Text != "patched it" {
		t.Fatalf("final text: %+v", final.Content[1])
	}
}

// TestEncodeBridge reconstructs a Responses/bridge capture: the richest request
// holds the instructions, a user message, a reasoning item, a function_call, and
// its output, and the teed .resp holds the final assistant message.
func TestEncodeBridge(t *testing.T) {
	dir := t.TempDir()
	req := map[string]any{
		"instructions": "system rules",
		"input": []map[string]any{
			{"type": "message", "role": "user", "content": []map[string]any{{"type": "input_text", "text": "go"}}},
			{"type": "reasoning", "summary": []map[string]any{{"type": "summary_text", "text": "thinking hard"}}},
			{"type": "function_call", "name": "shell", "arguments": `{"cmd":"ls"}`, "call_id": "c1"},
			{"type": "function_call_output", "call_id": "c1", "output": "a.py\nb.py"},
		},
	}
	raw, _ := json.Marshal(req)
	write(t, dir, "bridgetrace/0001.req.json", string(raw))

	// The teed final response: a Responses SSE stream whose completed event lists
	// an assistant message output item.
	completed := map[string]any{
		"response": map[string]any{
			"output": []map[string]any{
				{"type": "message", "role": "assistant", "content": []map[string]any{{"type": "output_text", "text": "done"}}},
			},
		},
	}
	cj, _ := json.Marshal(completed)
	resp := "event: response.completed\ndata: " + string(cj) + "\n\ndata: [DONE]\n"
	write(t, dir, "bridgetrace/0001.resp", resp)

	data, err := Encode(dir, Header{Model: "m", Timestamp: "20260722T101112Z"})
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	_, recs := records(t, data)
	// system, user, reasoning, tool call, tool result, final assistant = 6.
	if len(recs) != 6 {
		var roles []string
		for _, r := range recs {
			roles = append(roles, r.Message.Role)
		}
		t.Fatalf("want 6 messages, got %d (%v)", len(recs), roles)
	}
	if recs[0].Message.Role != "system" || recs[0].Message.Content[0].Text != "system rules" {
		t.Fatalf("system turn: %+v", recs[0].Message)
	}
	if recs[2].Message.Content[0].Type != "thinking" {
		t.Fatalf("reasoning turn: %+v", recs[2].Message)
	}
	if recs[3].Message.Content[0].Type != "toolCall" || recs[3].Message.Content[0].Name != "shell" {
		t.Fatalf("tool call turn: %+v", recs[3].Message)
	}
	if recs[4].Message.Content[0].Output != "a.py\nb.py" {
		t.Fatalf("tool result turn: %+v", recs[4].Message)
	}
	if recs[5].Message.Role != "assistant" || recs[5].Message.Content[0].Text != "done" {
		t.Fatalf("final turn: %+v", recs[5].Message)
	}
}

// TestEncodeBridgeItemDoneFinal reconstructs the final turn from a codex-shaped
// stream where the closing message arrives as a response.output_item.done and
// the terminal response.completed reports an EMPTY output array. This is the real
// shape the native-session audit caught dropping a run's closing summary: reading
// only completed.output loses the message, so the reconstruction must read the
// per-item done events.
func TestEncodeBridgeItemDoneFinal(t *testing.T) {
	dir := t.TempDir()
	req := map[string]any{
		"instructions": "system rules",
		"input": []map[string]any{
			{"type": "message", "role": "user", "content": []map[string]any{{"type": "input_text", "text": "go"}}},
		},
	}
	raw, _ := json.Marshal(req)
	write(t, dir, "bridgetrace/0001.req.json", string(raw))

	// The final response streams a reasoning item then a message item, each as its
	// own response.output_item.done, and closes with a completed event whose output
	// array is empty, exactly as codex tees it.
	reasoningDone, _ := json.Marshal(map[string]any{
		"type": "response.output_item.done",
		"item": map[string]any{"type": "reasoning", "summary": []map[string]any{{"type": "summary_text", "text": "wrapping up"}}},
	})
	msgDone, _ := json.Marshal(map[string]any{
		"type": "response.output_item.done",
		"item": map[string]any{"type": "message", "role": "assistant", "content": []map[string]any{{"type": "output_text", "text": "Implemented the ports."}}},
	})
	completed, _ := json.Marshal(map[string]any{
		"type":     "response.completed",
		"response": map[string]any{"output": []any{}},
	})
	resp := "data: " + string(reasoningDone) + "\n\ndata: " + string(msgDone) + "\n\ndata: " + string(completed) + "\n\ndata: [DONE]\n"
	write(t, dir, "bridgetrace/0001.resp", resp)

	data, err := Encode(dir, Header{Model: "m", Timestamp: "20260722T101112Z"})
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	_, recs := records(t, data)
	// system, user, then the final turn's reasoning + assistant message = 4.
	if len(recs) != 4 {
		var roles []string
		for _, r := range recs {
			roles = append(roles, r.Message.Role)
		}
		t.Fatalf("want 4 messages, got %d (%v)", len(recs), roles)
	}
	last := recs[len(recs)-1].Message
	if last.Role != "assistant" || last.Content[0].Type != "text" || last.Content[0].Text != "Implemented the ports." {
		t.Fatalf("final closing message not reconstructed: %+v", last)
	}
}

// TestEncodeBridgeMultiThread reconstructs a capture whose calls are split across
// separate request threads, the shape opencode produces when it spawns a sub-task:
// no single request holds every call, so reconstructing from the richest one alone
// would drop the calls that live only in the other thread. The union across all
// requests must recover every distinct call. Thread A (the main loop) makes c1 and
// c2; thread B (a side task) starts small again and makes c3. Each request echoes
// its own thread's earlier turns, so c1 appears in two requests and must be counted
// once.
func TestEncodeBridgeMultiThread(t *testing.T) {
	dir := t.TempDir()
	msg := func(role, text string) map[string]any {
		return map[string]any{"type": "message", "role": role, "content": []map[string]any{{"type": "input_text", "text": text}}}
	}
	call := func(id, cmd string) map[string]any {
		return map[string]any{"type": "function_call", "name": "shell", "arguments": `{"cmd":"` + cmd + `"}`, "call_id": id}
	}
	out := func(id, text string) map[string]any {
		return map[string]any{"type": "function_call_output", "call_id": id, "output": text}
	}
	req := func(input ...map[string]any) string {
		raw, _ := json.Marshal(map[string]any{"instructions": "system rules", "input": input})
		return string(raw)
	}
	// Thread A, round 1: user + c1.
	write(t, dir, "bridgetrace/0001.req.json", req(msg("user", "fix it"), call("c1", "ls")))
	// Thread B (side task) round 1: its own fresh user turn + c3, interleaved before
	// A's second round, and NOT carrying any of A's calls.
	write(t, dir, "bridgetrace/0002.req.json", req(msg("user", "summarize"), call("c3", "cat notes")))
	// Thread A, round 2: echoes c1 and its output, then adds c2.
	write(t, dir, "bridgetrace/0003.req.json", req(msg("user", "fix it"), call("c1", "ls"), out("c1", "a.py"), call("c2", "pytest")))

	data, err := Encode(dir, Header{Model: "m", Timestamp: "20260722T101112Z"})
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	_, recs := records(t, data)
	var calls, results int
	for _, r := range recs {
		for _, b := range r.Message.Content {
			switch b.Type {
			case "toolCall":
				calls++
			case "toolResult":
				results++
			}
		}
	}
	// c1, c2, c3 are three distinct calls; c1 echoed twice counts once.
	if calls != 3 {
		t.Fatalf("want 3 unioned tool calls (c1,c2,c3), got %d", calls)
	}
	if results != 1 {
		t.Fatalf("want 1 tool result (c1 output), got %d", results)
	}
}

// TestRedactAtBlock proves a credential echoed in a captured request never
// reaches a block: an Authorization bearer in a user turn and an sk- key in a
// tool argument are both masked in the encoded trace, and the pre-commit Scan
// finds nothing left.
func TestRedactAtBlock(t *testing.T) {
	dir := t.TempDir()
	body := map[string]any{
		"messages": []map[string]any{
			{"role": "user", "content": "curl -H 'Authorization: Bearer sk-abcdef0123456789abcd'"},
			{"role": "assistant", "tool_calls": []map[string]any{
				{"id": "c", "index": 0, "function": map[string]any{"name": "run", "arguments": `{"key":"sk-abcdef0123456789abcd"}`}},
			}},
		},
	}
	raw, _ := json.Marshal(map[string]any{"body": body})
	write(t, dir, "requests.jsonl", string(raw)+"\n")

	data, err := Encode(dir, Header{Model: "m"})
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if bytes.Contains(data, []byte("sk-abcdef0123456789abcd")) {
		t.Fatalf("raw key survived redaction:\n%s", data)
	}
	if shape := Scan(data); shape != "" {
		t.Fatalf("Scan still finds a secret shape %q in a redacted trace", shape)
	}
}
