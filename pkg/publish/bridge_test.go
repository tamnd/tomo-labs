package publish

import (
	"os"
	"path/filepath"
	"testing"
)

// TestEncodeTraceBridge reconstructs a session from a bridgetrace directory, the
// Responses-API request-capture format a swelive container run records instead
// of a chat requests.jsonl. The richest capture's instructions and input array
// map through the same emitter the chat path uses, so the trace carries the
// native session schema: a system message from the instructions, the reasoning
// as a thinking block, a function_call as a toolCall block with its arguments
// parsed to an object, and its output as a toolResult block linked by call id.
func TestEncodeTraceBridge(t *testing.T) {
	dir := t.TempDir()
	bt := filepath.Join(dir, "bridgetrace")
	if err := os.MkdirAll(bt, 0o755); err != nil {
		t.Fatal(err)
	}

	// A short capture and the richest capture; the decoder must pick the richest.
	writeFile(t, filepath.Join(bt, "0000.req.json"),
		`{"instructions":"you are an agent","input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"fix the bug"}]}]}`)
	writeFile(t, filepath.Join(bt, "0001.req.json"), `{
	  "instructions": "you are an agent",
	  "input": [
	    {"type":"message","role":"developer","content":[{"type":"input_text","text":"repo rules"}]},
	    {"type":"message","role":"user","content":[{"type":"input_text","text":"fix the bug"}]},
	    {"type":"reasoning","summary":[{"type":"summary_text","text":"I should read foo.py"}]},
	    {"type":"function_call","name":"read_file","arguments":"{\"path\":\"foo.py\"}","call_id":"call_a"},
	    {"type":"function_call_output","call_id":"call_a","output":"def foo(): pass"},
	    {"type":"message","role":"assistant","content":[{"type":"output_text","text":"patched foo.py"}]}
	  ]
	}`)

	data, err := EncodeTrace(dir, SessionMeta{Harness: "codex", ID: "b1", Model: "gpt-5.6-luna"})
	if err != nil {
		t.Fatalf("EncodeTrace: %v", err)
	}

	lines := splitJSONL(t, data)
	recs := make([]piRec, 0, len(lines))
	for _, l := range lines {
		var r piRec
		mustJSON(t, l, &r)
		recs = append(recs, r)
	}
	if recs[0].Type != "session" || recs[1].Type != "model_change" {
		t.Fatalf("missing session/model_change header: %+v", recs[:2])
	}

	msgs := recs[2:]
	// The developer instruction folds to system, and the standalone instructions
	// string is the first system message; both render as system turns.
	if msgs[0].Message.Role != "system" || findBlock(msgs[0], "text").Text != "you are an agent" {
		t.Fatalf("instructions not the first system turn: %+v", msgs[0])
	}
	if msgs[1].Message.Role != "system" || findBlock(msgs[1], "text").Text != "repo rules" {
		t.Fatalf("developer role not folded to system: %+v", msgs[1])
	}
	if msgs[2].Message.Role != "user" {
		t.Fatalf("user turn lost: %+v", msgs[2])
	}
	// The reasoning item becomes an assistant thinking block.
	if tb := findBlock(msgs[3], "thinking"); msgs[3].Message.Role != "assistant" || tb == nil || tb.Thinking != "I should read foo.py" {
		t.Fatalf("reasoning not a thinking block: %+v", msgs[3])
	}
	// The function_call becomes a toolCall block with parsed object arguments.
	call := findBlock(msgs[4], "toolCall")
	if call == nil || call.ID != "call_a" || call.Name != "read_file" || string(call.Arguments) != `{"path":"foo.py"}` {
		t.Fatalf("function_call lost: %+v", msgs[4].Message.Content)
	}
	// The output becomes a toolResult block linked by call id.
	tr := findBlock(msgs[5], "toolResult")
	if msgs[5].Message.Role != "tool" || tr == nil || tr.ToolCallID != "call_a" || tr.Output != "def foo(): pass" {
		t.Fatalf("function_call_output lost: %+v", msgs[5])
	}
	if last := findBlock(msgs[6], "text"); last == nil || last.Text != "patched foo.py" {
		t.Fatalf("final assistant text lost: %+v", msgs[6])
	}
}
