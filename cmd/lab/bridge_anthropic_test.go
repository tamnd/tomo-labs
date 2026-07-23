package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// TestAnthropicRequestToResponses proves the Anthropic Messages request the
// claude-code CLI sends flattens into the same Responses request every other tool
// produces: system to instructions, the message blocks (text, tool_use,
// tool_result) to input items, and the Anthropic tool shape (input_schema) to a
// flat Responses function tool. This is what keeps the bridgetrace one format so
// reconstruction and the claude native-session audit both work unchanged.
func TestAnthropicRequestToResponses(t *testing.T) {
	body := []byte(`{
	  "model": "claude-opus-4-8",
	  "system": [{"type":"text","text":"be terse"}],
	  "messages": [
	    {"role":"user","content":"list files"},
	    {"role":"assistant","content":[
	      {"type":"text","text":"ok"},
	      {"type":"tool_use","id":"toolu_1","name":"bash","input":{"cmd":"ls"}}
	    ]},
	    {"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_1","content":[{"type":"text","text":"a.txt"}]}]}
	  ],
	  "tools": [{"name":"bash","description":"run","input_schema":{"type":"object","properties":{"cmd":{"type":"string"}}}}],
	  "tool_choice": {"type":"auto"}
	}`)
	req, err := anthropicRequestToResponses(body, "gpt-5.6-sol", "high")
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
	// user text, assistant text, assistant tool_use, user tool_result = 4 items.
	types := []string{}
	for _, it := range input {
		types = append(types, it.(map[string]any)["type"].(string))
	}
	want := []string{"message", "message", "function_call", "function_call_output"}
	if len(types) != len(want) {
		t.Fatalf("want %d input items, got %d: %+v", len(want), len(types), types)
	}
	for i := range want {
		if types[i] != want[i] {
			t.Errorf("item %d type = %s, want %s", i, types[i], want[i])
		}
	}
	fc := input[2].(map[string]any)
	if fc["call_id"] != "toolu_1" || fc["name"] != "bash" || fc["arguments"] != `{"cmd":"ls"}` {
		t.Errorf("function_call item wrong: %+v", fc)
	}
	fo := input[3].(map[string]any)
	if fo["call_id"] != "toolu_1" || fo["output"] != "a.txt" {
		t.Errorf("function_call_output wrong: %+v", fo)
	}
	tools := req["tools"].([]map[string]any)
	if len(tools) != 1 || tools[0]["name"] != "bash" || tools[0]["type"] != "function" {
		t.Errorf("tools flatten wrong: %+v", tools)
	}
}

// TestResponsesStreamToAnthropic proves the Responses SSE the codex backend
// returns re-emits as the Anthropic Messages event stream claude-code reads:
// a message_start, a text content block, a tool_use content block whose arguments
// arrive as input_json_delta fragments, and a closing message_delta carrying the
// tool_use stop reason and output token count. Reassembling the events the way
// claude-code's parser does must recover the text, the tool name, and the tool
// input verbatim.
func TestResponsesStreamToAnthropic(t *testing.T) {
	sse := strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"resp_1"}}`,
		`data: {"type":"response.output_text.delta","delta":"Hello"}`,
		`data: {"type":"response.output_text.delta","delta":" world"}`,
		`data: {"type":"response.output_item.added","output_index":1,"item":{"id":"fc_1","type":"function_call","call_id":"call_9","name":"bash","arguments":""}}`,
		`data: {"type":"response.function_call_arguments.delta","item_id":"fc_1","delta":"{\"cmd\":"}`,
		`data: {"type":"response.function_call_arguments.delta","item_id":"fc_1","delta":"\"ls\"}"}`,
		`data: {"type":"response.completed","response":{"usage":{"input_tokens":100,"output_tokens":20,"input_tokens_details":{"cached_tokens":70}}}}`,
		`data: [DONE]`,
	}, "\n") + "\n"

	var out bytes.Buffer
	responsesStreamToAnthropic(&out, nil, strings.NewReader(sse), 1, "gpt-5.6-sol")

	// Replay the produced Anthropic SSE the way claude-code's parser would: track
	// content blocks by index, accumulating text and tool input.
	type blk struct {
		kind, text, toolName, toolInput string
	}
	blocks := map[int]*blk{}
	var stop string
	var inTok, outTok, cached int
	for _, line := range strings.Split(out.String(), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		p := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if p == "" {
			continue
		}
		var ev struct {
			Type         string `json:"type"`
			Index        int    `json:"index"`
			ContentBlock struct {
				Type string `json:"type"`
				Name string `json:"name"`
			} `json:"content_block"`
			Delta struct {
				Type        string `json:"type"`
				Text        string `json:"text"`
				PartialJSON string `json:"partial_json"`
				StopReason  string `json:"stop_reason"`
			} `json:"delta"`
			Usage struct {
				InputTokens          int `json:"input_tokens"`
				OutputTokens         int `json:"output_tokens"`
				CacheReadInputTokens int `json:"cache_read_input_tokens"`
			} `json:"usage"`
		}
		if json.Unmarshal([]byte(p), &ev) != nil {
			t.Fatalf("bad event: %s", p)
		}
		switch ev.Type {
		case "content_block_start":
			blocks[ev.Index] = &blk{kind: ev.ContentBlock.Type, toolName: ev.ContentBlock.Name}
		case "content_block_delta":
			b := blocks[ev.Index]
			if b == nil {
				t.Fatalf("delta for unopened block %d", ev.Index)
			}
			b.text += ev.Delta.Text
			b.toolInput += ev.Delta.PartialJSON
		case "message_delta":
			stop = ev.Delta.StopReason
			outTok = ev.Usage.OutputTokens
			inTok = ev.Usage.InputTokens
			cached = ev.Usage.CacheReadInputTokens
		}
	}
	if len(blocks) != 2 {
		t.Fatalf("want 2 content blocks, got %d: %+v", len(blocks), blocks)
	}
	if blocks[0].kind != "text" || blocks[0].text != "Hello world" {
		t.Errorf("text block = %+v", blocks[0])
	}
	if blocks[1].kind != "tool_use" || blocks[1].toolName != "bash" || blocks[1].toolInput != `{"cmd":"ls"}` {
		t.Errorf("tool block = %+v", blocks[1])
	}
	if stop != "tool_use" {
		t.Errorf("stop_reason = %q, want tool_use", stop)
	}
	if inTok != 100 || outTok != 20 || cached != 70 {
		t.Errorf("usage = in %d out %d cached %d", inTok, outTok, cached)
	}
}

// TestResponsesStreamToAnthropicNoDeltaArgs proves a function call whose whole
// arguments arrive only on output_item.done (no streamed argument deltas) still
// yields the call's input, so a backend that batches arguments does not produce
// an empty tool_use for claude-code.
func TestResponsesStreamToAnthropicNoDeltaArgs(t *testing.T) {
	sse := strings.Join([]string{
		`data: {"type":"response.output_item.added","item":{"id":"fc_1","type":"function_call","call_id":"call_9","name":"bash","arguments":""}}`,
		`data: {"type":"response.output_item.done","item":{"id":"fc_1","type":"function_call","call_id":"call_9","name":"bash","arguments":"{\"cmd\":\"ls\"}"}}`,
		`data: {"type":"response.completed","response":{"usage":{"input_tokens":1,"output_tokens":1}}}`,
	}, "\n") + "\n"

	var out bytes.Buffer
	responsesStreamToAnthropic(&out, nil, strings.NewReader(sse), 1, "gpt-5.6-sol")

	var input string
	for _, line := range strings.Split(out.String(), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		var ev struct {
			Type  string `json:"type"`
			Delta struct {
				PartialJSON string `json:"partial_json"`
			} `json:"delta"`
		}
		_ = json.Unmarshal([]byte(strings.TrimPrefix(line, "data:")), &ev)
		if ev.Type == "content_block_delta" {
			input += ev.Delta.PartialJSON
		}
	}
	if input != `{"cmd":"ls"}` {
		t.Errorf("batched tool input = %q, want {\"cmd\":\"ls\"}", input)
	}
}
