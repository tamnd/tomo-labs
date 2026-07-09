package proxy

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestAnthToChatRequest checks the request half of the Anthropic shim: system
// becomes a system message, a tool_use assistant turn becomes tool_calls, the
// following tool_result user turn becomes a tool message, and tools nest.
func TestAnthToChatRequest(t *testing.T) {
	body := []byte(`{
		"model": "deepseek-v4-flash-free",
		"max_tokens": 1024,
		"stream": true,
		"system": "be terse",
		"tools": [{"name":"read","description":"read a file","input_schema":{"type":"object"}}],
		"tool_choice": {"type":"any"},
		"messages": [
			{"role":"user","content":"read a.txt"},
			{"role":"assistant","content":[{"type":"text","text":"ok"},{"type":"tool_use","id":"tu1","name":"read","input":{"p":"a.txt"}}]},
			{"role":"user","content":[{"type":"tool_result","tool_use_id":"tu1","content":"hello"}]}
		]
	}`)
	chat, stream, err := anthToChatRequest(body)
	if err != nil {
		t.Fatalf("anthToChatRequest: %v", err)
	}
	if !stream {
		t.Fatalf("stream should be true")
	}
	var got struct {
		Messages   []map[string]any `json:"messages"`
		Tools      []map[string]any `json:"tools"`
		ToolChoice any              `json:"tool_choice"`
		MaxTokens  int              `json:"max_tokens"`
	}
	if err := json.Unmarshal(chat, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// system, user, assistant(text+tool_calls), tool
	if len(got.Messages) != 4 {
		t.Fatalf("messages = %d, want 4: %v", len(got.Messages), got.Messages)
	}
	if got.Messages[0]["role"] != "system" || got.Messages[0]["content"] != "be terse" {
		t.Errorf("system wrong: %v", got.Messages[0])
	}
	if got.Messages[2]["role"] != "assistant" || got.Messages[2]["content"] != "ok" {
		t.Errorf("assistant text wrong: %v", got.Messages[2])
	}
	if _, ok := got.Messages[2]["tool_calls"]; !ok {
		t.Errorf("assistant missing tool_calls: %v", got.Messages[2])
	}
	if got.Messages[3]["role"] != "tool" || got.Messages[3]["tool_call_id"] != "tu1" || got.Messages[3]["content"] != "hello" {
		t.Errorf("tool result wrong: %v", got.Messages[3])
	}
	if got.MaxTokens != 1024 {
		t.Errorf("max_tokens = %d", got.MaxTokens)
	}
	if got.ToolChoice != "required" {
		t.Errorf("tool_choice any should map to required, got %v", got.ToolChoice)
	}
	fn, _ := got.Tools[0]["function"].(map[string]any)
	if fn == nil || fn["name"] != "read" {
		t.Errorf("tool not nested: %v", got.Tools[0])
	}
}

// TestStreamMessagesText drives the streaming translator with a text-only chat
// stream and checks the Messages events Claude Code depends on are present, in
// order, ending in message_delta with the mapped stop reason and message_stop.
func TestStreamMessagesText(t *testing.T) {
	rec := httptest.NewRecorder()
	s := &anthStream{w: rec, fl: rec, id: "msg_lab_1", tools: map[int]*anthTool{}}
	s.begin()
	s.chunk(mustChunk(`{"model":"deepseek-v4-flash-free","choices":[{"delta":{"content":"Hel"}}]}`))
	s.chunk(mustChunk(`{"choices":[{"delta":{"content":"lo"}}]}`))
	s.chunk(mustChunk(`{"choices":[{"delta":{},"finish_reason":"stop"}]}`))
	s.chunk(mustChunk(`{"choices":[],"usage":{"prompt_tokens":9,"completion_tokens":2,"total_tokens":11}}`))
	s.finish()

	out := rec.Body.String()
	for _, want := range []string{
		"event: message_start",
		"event: content_block_start",
		"event: content_block_delta",
		"event: content_block_stop",
		"event: message_delta",
		"event: message_stop",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("stream missing %q\n%s", want, out)
		}
	}
	if !strings.Contains(out, `"text_delta"`) || !strings.Contains(out, `"text":"Hel"`) {
		t.Errorf("text deltas missing: %s", out)
	}
	ev := lastEvent(out, "message_delta")
	if ev == nil {
		t.Fatalf("no message_delta")
	}
	delta, _ := ev["delta"].(map[string]any)
	if delta["stop_reason"] != "end_turn" {
		t.Errorf("stop_reason = %v, want end_turn", delta["stop_reason"])
	}
	usage, _ := ev["usage"].(map[string]any)
	if usage["output_tokens"].(float64) != 2 {
		t.Errorf("output_tokens wrong: %v", usage)
	}
}

// TestStreamMessagesToolCall checks a streamed tool call becomes a tool_use
// block with input_json_delta fragments and a tool_use stop reason.
func TestStreamMessagesToolCall(t *testing.T) {
	rec := httptest.NewRecorder()
	s := &anthStream{w: rec, fl: rec, id: "msg_lab_2", tools: map[int]*anthTool{}}
	s.begin()
	s.chunk(mustChunk(`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_7","function":{"name":"run","arguments":"{\"cmd\":"}}]}}]}`))
	s.chunk(mustChunk(`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"ls\"}"}}]}}]}`))
	s.chunk(mustChunk(`{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`))
	s.finish()

	out := rec.Body.String()
	if !strings.Contains(out, `"type":"tool_use"`) || !strings.Contains(out, `"id":"call_7"`) || !strings.Contains(out, `"name":"run"`) {
		t.Errorf("tool_use block missing: %s", out)
	}
	if !strings.Contains(out, `"input_json_delta"`) || !strings.Contains(out, `"partial_json":"{\"cmd\":"`) {
		t.Errorf("input_json_delta missing: %s", out)
	}
	ev := lastEvent(out, "message_delta")
	delta, _ := ev["delta"].(map[string]any)
	if delta["stop_reason"] != "tool_use" {
		t.Errorf("stop_reason = %v, want tool_use", delta["stop_reason"])
	}
}
