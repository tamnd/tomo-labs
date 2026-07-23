package publish

import (
	"bufio"
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// piRec is the union of the record types a trace file carries, decoded loosely
// so one test can walk every line: the session, the model_change, and the
// message records with their typed content blocks.
type piRec struct {
	Type      string `json:"type"`
	Version   int    `json:"version"`
	ID        string `json:"id"`
	ParentID  string `json:"parentId"`
	Timestamp string `json:"timestamp"`
	ModelID   string `json:"modelId"`
	Meta      struct {
		Harness  string `json:"harness"`
		Eval     string `json:"eval"`
		Scenario string `json:"scenario"`
		Passed   bool   `json:"passed"`
	} `json:"meta"`
	Message struct {
		Role    string `json:"role"`
		Content []struct {
			Type       string          `json:"type"`
			Text       string          `json:"text"`
			Thinking   string          `json:"thinking"`
			ID         string          `json:"id"`
			Name       string          `json:"name"`
			Arguments  json.RawMessage `json:"arguments"`
			ToolCallID string          `json:"toolCallId"`
			Output     string          `json:"output"`
		} `json:"content"`
	} `json:"message"`
}

// TestEncodeTrace reconstructs the fixture trace and checks the native session
// schema: a session record, a model_change record, then one message record per
// turn, each with a top-level id/parentId/timestamp and content as typed
// blocks. Tool-call ids and their parsed arguments are preserved so the viewer
// can stitch a result to its call.
func TestEncodeTrace(t *testing.T) {
	meta := SessionMeta{
		Harness:  "tomo-oi",
		ID:       "20260722T101010Z",
		Name:     "tomo-oi on dynaconf-1225 (gpt-5.6-luna)",
		Eval:     "swebench-live",
		Scenario: "dynaconf-1225",
		Model:    "gpt-5.6-luna",
		Passed:   true,
	}
	data, err := EncodeTrace("testdata/trace", meta)
	if err != nil {
		t.Fatalf("EncodeTrace: %v", err)
	}

	lines := splitJSONL(t, data)
	if len(lines) < 7 {
		t.Fatalf("want session + model_change + 5 messages, got %d lines: %s", len(lines), data)
	}

	recs := make([]piRec, 0, len(lines))
	for _, l := range lines {
		var r piRec
		mustJSON(t, l, &r)
		recs = append(recs, r)
	}

	// Session record.
	if recs[0].Type != "session" || recs[0].Version != 3 || recs[0].ID != meta.ID {
		t.Fatalf("bad session record: %+v", recs[0])
	}
	if recs[0].Meta.Eval != "swebench-live" || recs[0].Meta.Scenario != "dynaconf-1225" || !recs[0].Meta.Passed {
		t.Fatalf("session meta lost provenance: %+v", recs[0].Meta)
	}
	// model_change record, parented to nothing.
	if recs[1].Type != "model_change" || recs[1].ModelID != "gpt-5.6-luna" || recs[1].ParentID != "" {
		t.Fatalf("bad model_change: %+v", recs[1])
	}

	msgs := recs[2:]
	for _, m := range msgs {
		if m.Type != "message" {
			t.Fatalf("non-message record in message range: %+v", m)
		}
	}
	// The parent chain threads model_change -> msg-0 -> msg-1 -> ...
	if msgs[0].ParentID != "model-0" {
		t.Fatalf("first message not parented to model: %+v", msgs[0])
	}
	if msgs[1].ParentID != msgs[0].ID {
		t.Fatalf("parent chain broken: %+v -> %+v", msgs[0], msgs[1])
	}

	// The request history: system, user, assistant-with-call, tool-result.
	if msgs[0].Message.Role != "system" || msgs[1].Message.Role != "user" {
		t.Fatalf("history head wrong: %+v", msgs[:2])
	}
	// The assistant turn in history made a tool call, rendered as a toolCall block.
	call := findBlock(msgs[2], "toolCall")
	if call == nil || call.ID != "call_1" || call.Name != "read_file" {
		t.Fatalf("history tool call lost: %+v", msgs[2].Message.Content)
	}
	// The tool result is a toolResult block carrying the id linking it to the call.
	tr := findBlock(msgs[3], "toolResult")
	if msgs[3].Message.Role != "tool" || tr == nil || tr.ToolCallID != "call_1" {
		t.Fatalf("tool result link lost: %+v", msgs[3])
	}

	// The final message is the assistant reply decoded from the SSE stream.
	final := msgs[len(msgs)-1]
	if final.Message.Role != "assistant" {
		t.Fatalf("final not assistant: %+v", final)
	}
	if tb := findBlock(final, "thinking"); tb == nil || tb.Thinking != "The bug is an off-by-one." {
		t.Fatalf("reasoning not a thinking block: %+v", final.Message.Content)
	}
	if txt := findBlock(final, "text"); txt == nil || txt.Text != "I will patch foo.py." {
		t.Fatalf("content not a text block: %+v", final.Message.Content)
	}
	fc := findBlock(final, "toolCall")
	if fc == nil || fc.ID != "call_2" {
		t.Fatalf("final tool call lost: %+v", final.Message.Content)
	}
	// Streamed arguments reassemble and are emitted as a JSON object, not a string.
	if string(fc.Arguments) != `{"path":"foo.py"}` {
		t.Fatalf("arguments not a parsed object: %s", fc.Arguments)
	}
}

// findBlock returns the first content block of the given type in a message
// record, or nil when the message has none.
func findBlock(r piRec, typ string) *struct {
	Type       string          `json:"type"`
	Text       string          `json:"text"`
	Thinking   string          `json:"thinking"`
	ID         string          `json:"id"`
	Name       string          `json:"name"`
	Arguments  json.RawMessage `json:"arguments"`
	ToolCallID string          `json:"toolCallId"`
	Output     string          `json:"output"`
} {
	for i := range r.Message.Content {
		if r.Message.Content[i].Type == typ {
			return &r.Message.Content[i]
		}
	}
	return nil
}

// TestEncodeTraceMissingResponse asserts a trace with no decodable final
// response still produces a valid file ending at the request history rather than
// failing.
func TestEncodeTraceMissingResponse(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir+"/requests.jsonl",
		`{"path":"/v1/chat/completions","body":{"model":"m","messages":[{"role":"user","content":"hi"}]}}`+"\n")
	data, err := EncodeTrace(dir, SessionMeta{Harness: "tomo", ID: "x"})
	if err != nil {
		t.Fatalf("EncodeTrace: %v", err)
	}
	lines := splitJSONL(t, data)
	// session + model_change + the one user message.
	if len(lines) != 3 {
		t.Fatalf("want session + model_change + 1 message, got %d", len(lines))
	}
}

func splitJSONL(t *testing.T, data []byte) []string {
	t.Helper()
	var out []string
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for sc.Scan() {
		if line := strings.TrimSpace(sc.Text()); line != "" {
			out = append(out, line)
		}
	}
	return out
}

func mustJSON(t *testing.T, line string, v any) {
	t.Helper()
	if err := json.Unmarshal([]byte(line), v); err != nil {
		t.Fatalf("unmarshal %q: %v", line, err)
	}
}
