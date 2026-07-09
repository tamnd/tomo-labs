package proxy

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestToChatRequest checks the request half of the shim: instructions become a
// system message, the input items become the message list in order, the flat
// tool shape nests, and a streamed call asks upstream for a usage chunk.
func TestToChatRequest(t *testing.T) {
	body := []byte(`{
		"model": "deepseek-v4-flash-free",
		"instructions": "you are terse",
		"stream": true,
		"tools": [{"type":"function","name":"read_file","description":"read","parameters":{"type":"object"}}],
		"tool_choice": {"type":"function","name":"read_file"},
		"input": [
			{"type":"message","role":"user","content":[{"type":"input_text","text":"hi"}]},
			{"type":"function_call","call_id":"c1","name":"read_file","arguments":"{\"p\":\"a\"}"},
			{"type":"function_call_output","call_id":"c1","output":"file body"},
			{"type":"message","role":"user","content":"and now what"}
		]
	}`)
	chat, stream, err := toChatRequest(body)
	if err != nil {
		t.Fatalf("toChatRequest: %v", err)
	}
	if !stream {
		t.Fatalf("stream should be true")
	}
	var got struct {
		Model         string           `json:"model"`
		Messages      []map[string]any `json:"messages"`
		Tools         []map[string]any `json:"tools"`
		ToolChoice    map[string]any   `json:"tool_choice"`
		StreamOptions map[string]any   `json:"stream_options"`
	}
	if err := json.Unmarshal(chat, &got); err != nil {
		t.Fatalf("unmarshal chat: %v", err)
	}
	if got.Model != "deepseek-v4-flash-free" {
		t.Errorf("model = %q", got.Model)
	}
	// system, user, assistant(tool_calls), tool, user
	if len(got.Messages) != 5 {
		t.Fatalf("messages = %d, want 5: %v", len(got.Messages), got.Messages)
	}
	if got.Messages[0]["role"] != "system" || got.Messages[0]["content"] != "you are terse" {
		t.Errorf("system message wrong: %v", got.Messages[0])
	}
	if got.Messages[1]["content"] != "hi" {
		t.Errorf("first user content flattened wrong: %v", got.Messages[1])
	}
	if got.Messages[2]["role"] != "assistant" {
		t.Errorf("expected assistant tool_calls message, got %v", got.Messages[2])
	}
	if _, ok := got.Messages[2]["tool_calls"]; !ok {
		t.Errorf("assistant message missing tool_calls: %v", got.Messages[2])
	}
	if got.Messages[3]["role"] != "tool" || got.Messages[3]["tool_call_id"] != "c1" || got.Messages[3]["content"] != "file body" {
		t.Errorf("tool result message wrong: %v", got.Messages[3])
	}
	if len(got.Tools) != 1 {
		t.Fatalf("tools = %d", len(got.Tools))
	}
	fn, _ := got.Tools[0]["function"].(map[string]any)
	if fn == nil || fn["name"] != "read_file" {
		t.Errorf("tool not nested under function: %v", got.Tools[0])
	}
	tcFn, _ := got.ToolChoice["function"].(map[string]any)
	if tcFn == nil || tcFn["name"] != "read_file" {
		t.Errorf("tool_choice not nested: %v", got.ToolChoice)
	}
	if got.StreamOptions["include_usage"] != true {
		t.Errorf("stream_options.include_usage not set: %v", got.StreamOptions)
	}
}

// TestStreamResponsesText drives the streaming translator with a text-only chat
// stream and checks the Responses events codex depends on are all present and in
// order, ending in a response.completed carrying translated token counts.
func TestStreamResponsesText(t *testing.T) {
	rec := httptest.NewRecorder()
	s := &respStream{w: rec, fl: rec, id: "resp_lab_1", tools: map[int]*toolAcc{}}
	s.begin()
	s.chunk(mustChunk(`{"model":"deepseek-v4-flash-free","choices":[{"delta":{"content":"Hel"}}]}`))
	s.chunk(mustChunk(`{"choices":[{"delta":{"content":"lo"}}]}`))
	s.chunk(mustChunk(`{"choices":[{"delta":{},"finish_reason":"stop"}]}`))
	s.chunk(mustChunk(`{"choices":[],"usage":{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12,"prompt_cache_hit_tokens":4}}`))
	s.finish()

	out := rec.Body.String()
	for _, want := range []string{
		"event: response.created",
		"event: response.output_item.added",
		"event: response.output_text.delta",
		"event: response.output_text.done",
		"event: response.completed",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("stream missing %q\n%s", want, out)
		}
	}
	// the delta text should reassemble to Hello
	if !strings.Contains(out, `"delta":"Hel"`) || !strings.Contains(out, `"delta":"lo"`) {
		t.Errorf("text deltas missing: %s", out)
	}
	// response.completed must carry Responses-named usage with the cached count
	ev := lastEvent(out, "response.completed")
	if ev == nil {
		t.Fatalf("no response.completed data")
	}
	resp, _ := ev["response"].(map[string]any)
	usage, _ := resp["usage"].(map[string]any)
	if usage["input_tokens"].(float64) != 10 || usage["output_tokens"].(float64) != 2 {
		t.Errorf("usage tokens wrong: %v", usage)
	}
	det, _ := usage["input_tokens_details"].(map[string]any)
	if det["cached_tokens"].(float64) != 4 {
		t.Errorf("cached tokens not carried: %v", usage)
	}
}

// TestStreamResponsesToolCall checks that a streamed tool call becomes a
// function_call item with its arguments reassembled.
func TestStreamResponsesToolCall(t *testing.T) {
	rec := httptest.NewRecorder()
	s := &respStream{w: rec, fl: rec, id: "resp_lab_2", tools: map[int]*toolAcc{}}
	s.begin()
	s.chunk(mustChunk(`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_9","function":{"name":"run","arguments":"{\"cmd\":"}}]}}]}`))
	s.chunk(mustChunk(`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"ls\"}"}}]}}]}`))
	s.chunk(mustChunk(`{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`))
	s.finish()

	out := rec.Body.String()
	if !strings.Contains(out, "event: response.function_call_arguments.delta") {
		t.Errorf("missing argument deltas: %s", out)
	}
	ev := lastEvent(out, "response.completed")
	resp, _ := ev["response"].(map[string]any)
	output, _ := resp["output"].([]any)
	if len(output) != 1 {
		t.Fatalf("output items = %d, want 1: %v", len(output), output)
	}
	item, _ := output[0].(map[string]any)
	if item["type"] != "function_call" || item["name"] != "run" || item["call_id"] != "call_9" {
		t.Errorf("function_call item wrong: %v", item)
	}
	if item["arguments"] != `{"cmd":"ls"}` {
		t.Errorf("arguments not reassembled: %v", item["arguments"])
	}
}

func mustChunk(s string) chatChunk {
	var c chatChunk
	if err := json.Unmarshal([]byte(s), &c); err != nil {
		panic(err)
	}
	return c
}

// lastEvent returns the parsed data JSON of the last SSE event of the given type.
func lastEvent(stream, event string) map[string]any {
	var out map[string]any
	for _, block := range strings.Split(stream, "\n\n") {
		if !strings.Contains(block, "event: "+event+"\n") {
			continue
		}
		for _, line := range strings.Split(block, "\n") {
			if strings.HasPrefix(line, "data: ") {
				var m map[string]any
				if json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &m) == nil {
					out = m
				}
			}
		}
	}
	return out
}
