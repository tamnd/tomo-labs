package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestChatRequestToResponses(t *testing.T) {
	chat := []byte(`{
	  "model": "gpt-4",
	  "messages": [
	    {"role":"system","content":"be terse"},
	    {"role":"user","content":"list files"},
	    {"role":"assistant","content":"ok","tool_calls":[{"id":"call_1","type":"function","function":{"name":"bash","arguments":"{\"cmd\":\"ls\"}"}}]},
	    {"role":"tool","tool_call_id":"call_1","content":"a.txt"}
	  ],
	  "tools": [{"type":"function","function":{"name":"bash","description":"run","parameters":{"type":"object"}}}],
	  "tool_choice": "auto"
	}`)
	req, err := chatRequestToResponses(chat, "gpt-5.6-sol", "high")
	if err != nil {
		t.Fatal(err)
	}
	if req["model"] != "gpt-5.6-sol" {
		t.Errorf("model override lost: %v", req["model"])
	}
	if req["instructions"] != "be terse" {
		t.Errorf("instructions: %q", req["instructions"])
	}
	if req["store"] != false || req["stream"] != true {
		t.Errorf("store/stream flags wrong: %v %v", req["store"], req["stream"])
	}
	reason, _ := req["reasoning"].(map[string]any)
	if reason["effort"] != "high" {
		t.Errorf("reasoning effort: %v", reason)
	}
	input := req["input"].([]any)
	if len(input) != 4 {
		t.Fatalf("want 4 input items, got %d: %+v", len(input), input)
	}
	types := []string{}
	for _, it := range input {
		types = append(types, it.(map[string]any)["type"].(string))
	}
	want := []string{"message", "message", "function_call", "function_call_output"}
	for i := range want {
		if types[i] != want[i] {
			t.Errorf("item %d type = %s, want %s", i, types[i], want[i])
		}
	}
	// user message uses input_text, assistant uses output_text
	u := input[0].(map[string]any)
	if u["role"] != "user" {
		t.Errorf("first item role %v", u["role"])
	}
	if pt := u["content"].([]any)[0].(map[string]any)["type"]; pt != "input_text" {
		t.Errorf("user part type %v", pt)
	}
	fc := input[2].(map[string]any)
	if fc["call_id"] != "call_1" || fc["name"] != "bash" {
		t.Errorf("function_call item wrong: %+v", fc)
	}
	fo := input[3].(map[string]any)
	if fo["call_id"] != "call_1" || fo["output"] != "a.txt" {
		t.Errorf("function_call_output wrong: %+v", fo)
	}
	tools := req["tools"].([]map[string]any)
	if len(tools) != 1 || tools[0]["name"] != "bash" || tools[0]["type"] != "function" {
		t.Errorf("tools flatten wrong: %+v", tools)
	}
}

// The bridge must forward prompt_cache_key to the Responses backend, not drop it
// in translation, or a tool's cache routing is invisible through the bridge and a
// benchmark run measures a stripped request instead of the real one. Absent the
// hint, the field must simply be absent.
func TestChatRequestForwardsPromptCacheKey(t *testing.T) {
	with := []byte(`{"model":"gpt-4","messages":[{"role":"user","content":"hi"}],"prompt_cache_key":"tomo-abc123"}`)
	req, err := chatRequestToResponses(with, "gpt-5.6-sol", "high")
	if err != nil {
		t.Fatal(err)
	}
	if req["prompt_cache_key"] != "tomo-abc123" {
		t.Errorf("prompt_cache_key not forwarded: %v", req["prompt_cache_key"])
	}

	without := []byte(`{"model":"gpt-4","messages":[{"role":"user","content":"hi"}]}`)
	req2, err := chatRequestToResponses(without, "gpt-5.6-sol", "high")
	if err != nil {
		t.Fatal(err)
	}
	if _, present := req2["prompt_cache_key"]; present {
		t.Errorf("prompt_cache_key should be absent when unset, got %v", req2["prompt_cache_key"])
	}
}

func TestResponsesStreamToChat(t *testing.T) {
	// A minimal but representative Responses SSE stream: some text, one function
	// call streamed in argument fragments, then completion with usage.
	sse := strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"resp_1"}}`,
		`data: {"type":"response.output_text.delta","delta":"Hello"}`,
		`data: {"type":"response.output_text.delta","delta":" world"}`,
		`data: {"type":"response.output_item.added","output_index":1,"item":{"id":"fc_1","type":"function_call","call_id":"call_9","name":"bash","arguments":""}}`,
		`data: {"type":"response.function_call_arguments.delta","item_id":"fc_1","delta":"{\"cmd\":"}`,
		`data: {"type":"response.function_call_arguments.delta","item_id":"fc_1","delta":"\"ls\"}"}`,
		`data: {"type":"response.completed","response":{"usage":{"input_tokens":100,"output_tokens":20,"total_tokens":120,"input_tokens_details":{"cached_tokens":80}}}}`,
		`data: [DONE]`,
	}, "\n") + "\n"

	var out bytes.Buffer
	responsesStreamToChat(&out, nil, strings.NewReader(sse), 1, "gpt-5.6-sol")

	// Replay the produced chat SSE the way tomo's parser would.
	var text strings.Builder
	var toolName, toolArgs, finish string
	var prompt, completion, cached int
	for _, line := range strings.Split(out.String(), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		p := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if p == "[DONE]" || p == "" {
			continue
		}
		var ch struct {
			Choices []struct {
				Delta struct {
					Content   string `json:"content"`
					ToolCalls []struct {
						Function struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						} `json:"function"`
					} `json:"tool_calls"`
				} `json:"delta"`
				FinishReason string `json:"finish_reason"`
			} `json:"choices"`
			Usage *struct {
				PromptTokens        int `json:"prompt_tokens"`
				CompletionTokens    int `json:"completion_tokens"`
				PromptTokensDetails *struct {
					CachedTokens int `json:"cached_tokens"`
				} `json:"prompt_tokens_details"`
			} `json:"usage"`
		}
		if json.Unmarshal([]byte(p), &ch) != nil {
			t.Fatalf("bad chunk: %s", p)
		}
		if ch.Usage != nil {
			prompt, completion = ch.Usage.PromptTokens, ch.Usage.CompletionTokens
			if ch.Usage.PromptTokensDetails != nil {
				cached = ch.Usage.PromptTokensDetails.CachedTokens
			}
		}
		for _, c := range ch.Choices {
			text.WriteString(c.Delta.Content)
			for _, tc := range c.Delta.ToolCalls {
				if tc.Function.Name != "" {
					toolName = tc.Function.Name
				}
				toolArgs += tc.Function.Arguments
			}
			if c.FinishReason != "" {
				finish = c.FinishReason
			}
		}
	}
	if text.String() != "Hello world" {
		t.Errorf("text = %q", text.String())
	}
	if toolName != "bash" || toolArgs != `{"cmd":"ls"}` {
		t.Errorf("tool = %s args = %q", toolName, toolArgs)
	}
	if finish != "tool_calls" {
		t.Errorf("finish = %q, want tool_calls", finish)
	}
	if prompt != 100 || completion != 20 {
		t.Errorf("usage = %d/%d", prompt, completion)
	}
	if cached != 80 {
		t.Errorf("cached prompt tokens = %d, want 80 (bridge must not drop the cache read)", cached)
	}
}
