package publish

import (
	"bufio"
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// TestEncodeTrace reconstructs the fixture trace and checks the STS envelope
// shape: a session header carrying the run metadata, the request history, and
// the final assistant turn decoded from the gzip SSE response, with tool-call
// ids preserved so the viewer can stitch a result to its call.
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
	if len(lines) < 6 {
		t.Fatalf("want header + 5 messages, got %d lines: %s", len(lines), data)
	}

	// Header line.
	var hdr stsHeader
	mustJSON(t, lines[0], &hdr)
	if hdr.Type != "session" || hdr.Harness != "tomo-oi" || hdr.ID != meta.ID {
		t.Fatalf("bad header: %+v", hdr)
	}
	if hdr.Eval != "swebench-live" || hdr.Scenario != "dynaconf-1225" || !hdr.Passed {
		t.Fatalf("header lost metadata: %+v", hdr)
	}

	msgs := make([]stsMessage, 0, len(lines)-1)
	for _, l := range lines[1:] {
		var env stsEnvelope
		mustJSON(t, l, &env)
		if env.Type != "message" {
			t.Fatalf("non-message line: %s", l)
		}
		msgs = append(msgs, env.Message)
	}

	// The request history: system, user, assistant-with-call, tool-result.
	if msgs[0].Role != "system" || msgs[1].Role != "user" {
		t.Fatalf("history head wrong: %+v", msgs[:2])
	}
	// The assistant turn in history made a tool call.
	if len(msgs[2].ToolCalls) != 1 || msgs[2].ToolCalls[0].ID != "call_1" {
		t.Fatalf("history tool call lost: %+v", msgs[2])
	}
	if msgs[2].ToolCalls[0].Function.Name != "read_file" {
		t.Fatalf("tool name lost: %+v", msgs[2].ToolCalls[0])
	}
	// The tool result carries the tool_call_id linking it to the call.
	if msgs[3].Role != "tool" || msgs[3].ToolCallID != "call_1" {
		t.Fatalf("tool result link lost: %+v", msgs[3])
	}

	// The final message is the assistant reply decoded from the SSE stream.
	final := msgs[len(msgs)-1]
	if final.Role != "assistant" {
		t.Fatalf("final not assistant: %+v", final)
	}
	if final.ReasoningContent != "The bug is an off-by-one." {
		t.Fatalf("reasoning not accumulated: %q", final.ReasoningContent)
	}
	if final.Content != "I will patch foo.py." {
		t.Fatalf("content not accumulated: %q", final.Content)
	}
	if len(final.ToolCalls) != 1 || final.ToolCalls[0].ID != "call_2" {
		t.Fatalf("final tool call lost: %+v", final.ToolCalls)
	}
	if got := final.ToolCalls[0].Function.Arguments; got != `{"path":"foo.py"}` {
		t.Fatalf("streamed arguments not reassembled: %q", got)
	}
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
	if len(lines) != 2 {
		t.Fatalf("want header + 1 message, got %d", len(lines))
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
