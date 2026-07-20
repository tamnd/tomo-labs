package main

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// chatRequestToResponses converts an OpenAI chat/completions request into the
// Responses request the codex backend expects. It is the mirror of tomo's
// wire.ResponsesToChat: system messages become instructions, the rest become
// input items, and the nested chat tool shape flattens to the Responses one.
//
// modelOverride, when set, replaces whatever model the tool asked for; effort
// sets the reasoning effort the ChatGPT gpt-5.x models take.
func chatRequestToResponses(chat []byte, modelOverride, effort string) (map[string]any, error) {
	var c struct {
		Model    string `json:"model"`
		Messages []struct {
			Role       string          `json:"role"`
			Content    json.RawMessage `json:"content"`
			Name       string          `json:"name"`
			ToolCallID string          `json:"tool_call_id"`
			ToolCalls  []struct {
				ID       string `json:"id"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"messages"`
		Tools          []json.RawMessage `json:"tools"`
		ToolChoice     json.RawMessage   `json:"tool_choice"`
		PromptCacheKey string            `json:"prompt_cache_key"`
	}
	if err := json.Unmarshal(chat, &c); err != nil {
		return nil, err
	}

	var instructions []string
	input := []any{}
	for _, m := range c.Messages {
		switch m.Role {
		case "system", "developer":
			instructions = append(instructions, contentString(m.Content))
		case "user":
			input = append(input, msgItem("user", "input_text", contentString(m.Content)))
		case "assistant":
			if t := contentString(m.Content); t != "" {
				input = append(input, msgItem("assistant", "output_text", t))
			}
			for _, tc := range m.ToolCalls {
				input = append(input, map[string]any{
					"type":      "function_call",
					"call_id":   tc.ID,
					"name":      tc.Function.Name,
					"arguments": tc.Function.Arguments,
				})
			}
		case "tool":
			input = append(input, map[string]any{
				"type":    "function_call_output",
				"call_id": m.ToolCallID,
				"output":  contentString(m.Content),
			})
		}
	}

	model := c.Model
	if modelOverride != "" {
		model = modelOverride
	}
	req := map[string]any{
		"model":               model,
		"instructions":        strings.Join(instructions, "\n\n"),
		"input":               input,
		"tool_choice":         toResponsesToolChoice(c.ToolChoice),
		"parallel_tool_calls": false,
		"store":               false,
		"stream":              true,
	}
	if tools := toResponsesTools(c.Tools); len(tools) > 0 {
		req["tools"] = tools
	}
	if effort != "" {
		req["reasoning"] = map[string]any{"effort": effort, "summary": "auto"}
	}
	// The Responses API keys its prompt cache on this hint, the same one tomo's
	// OpenAI client sends. Forwarding it (instead of dropping it in translation)
	// lets a benchmark run through the bridge exercise the caller's cache routing,
	// so the harness measures the tool's real behavior rather than a stripped one.
	if c.PromptCacheKey != "" {
		req["prompt_cache_key"] = c.PromptCacheKey
	}
	return req, nil
}

// msgItem builds one Responses message input item with a single text part.
func msgItem(role, partType, text string) map[string]any {
	return map[string]any{
		"type":    "message",
		"role":    role,
		"content": []any{map[string]any{"type": partType, "text": text}},
	}
}

// contentString flattens a chat content value (a string, or an array of parts)
// down to plain text, which is all the port needs to carry across.
func contentString(raw json.RawMessage) string {
	t := strings.TrimSpace(string(raw))
	if t == "" || t == "null" {
		return ""
	}
	if t[0] == '"' {
		var s string
		if json.Unmarshal(raw, &s) == nil {
			return s
		}
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &parts) == nil {
		var b strings.Builder
		for _, p := range parts {
			if p.Text != "" {
				b.WriteString(p.Text)
			}
		}
		return b.String()
	}
	return ""
}

// toResponsesTools flattens the nested chat function tool shape
// ({type,function:{name,description,parameters}}) into the flat Responses one
// ({type:"function",name,description,parameters}).
func toResponsesTools(tools []json.RawMessage) []map[string]any {
	out := []map[string]any{}
	for _, raw := range tools {
		var t struct {
			Type     string `json:"type"`
			Function struct {
				Name        string          `json:"name"`
				Description string          `json:"description"`
				Parameters  json.RawMessage `json:"parameters"`
			} `json:"function"`
		}
		if json.Unmarshal(raw, &t) != nil {
			continue
		}
		if t.Type != "" && t.Type != "function" {
			continue
		}
		fn := map[string]any{"type": "function", "name": t.Function.Name}
		if t.Function.Description != "" {
			fn["description"] = t.Function.Description
		}
		if len(t.Function.Parameters) > 0 {
			fn["parameters"] = json.RawMessage(t.Function.Parameters)
		} else {
			fn["parameters"] = map[string]any{"type": "object", "properties": map[string]any{}}
		}
		out = append(out, fn)
	}
	return out
}

// toResponsesToolChoice maps the chat tool_choice onto the Responses form. The
// string modes pass through; the {type:function,function:{name}} object loses a
// level of nesting.
func toResponsesToolChoice(raw json.RawMessage) any {
	t := strings.TrimSpace(string(raw))
	if t == "" || t == "null" {
		return "auto"
	}
	if t[0] == '"' {
		var s string
		if json.Unmarshal(raw, &s) == nil {
			return s
		}
	}
	var m struct {
		Type     string `json:"type"`
		Function struct {
			Name string `json:"name"`
		} `json:"function"`
	}
	if json.Unmarshal(raw, &m) == nil && m.Function.Name != "" {
		return map[string]any{"type": "function", "name": m.Function.Name}
	}
	return "auto"
}

// responsesStreamToChat consumes the Responses SSE stream the codex backend
// returns and re-emits it as the chat/completions chunk stream tomo reads. It
// is the mirror of tomo's wire.StreamResponses.
func responsesStreamToChat(w io.Writer, flush func(), r io.Reader, seq int, model string) {
	id := fmt.Sprintf("chatcmpl_bridge_%d", seq)
	toolIdx := map[string]int{} // Responses item_id -> chat tool_call index
	nextTool := 0
	hadTool := false
	streamedArgs := map[string]bool{}

	emit := func(delta map[string]any, finish any, usage map[string]any) {
		choice := map[string]any{"index": 0, "delta": delta}
		if finish != nil {
			choice["finish_reason"] = finish
		}
		chunk := map[string]any{
			"id": id, "object": "chat.completion.chunk", "model": model,
			"choices": []any{choice},
		}
		if usage != nil {
			chunk["usage"] = usage
		}
		b, _ := json.Marshal(chunk)
		fmt.Fprintf(w, "data: %s\n\n", b)
		if flush != nil {
			flush()
		}
	}

	readSSELines(r, func(payload []byte) {
		var ev struct {
			Type     string          `json:"type"`
			Delta    string          `json:"delta"`
			ItemID   string          `json:"item_id"`
			Item     json.RawMessage `json:"item"`
			Response json.RawMessage `json:"response"`
		}
		if json.Unmarshal(payload, &ev) != nil {
			return
		}
		switch ev.Type {
		case "response.output_text.delta":
			if ev.Delta != "" {
				emit(map[string]any{"content": ev.Delta}, nil, nil)
			}
		case "response.output_item.added":
			var it struct {
				ID        string `json:"id"`
				Type      string `json:"type"`
				CallID    string `json:"call_id"`
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			}
			if json.Unmarshal(ev.Item, &it) != nil || it.Type != "function_call" {
				return
			}
			idx := nextTool
			toolIdx[it.ID] = idx
			nextTool++
			hadTool = true
			emit(map[string]any{"tool_calls": []any{map[string]any{
				"index": idx, "id": it.CallID, "type": "function",
				"function": map[string]any{"name": it.Name, "arguments": ""},
			}}}, nil, nil)
		case "response.function_call_arguments.delta":
			idx, ok := toolIdx[ev.ItemID]
			if !ok || ev.Delta == "" {
				return
			}
			streamedArgs[ev.ItemID] = true
			emit(map[string]any{"tool_calls": []any{map[string]any{
				"index": idx, "function": map[string]any{"arguments": ev.Delta},
			}}}, nil, nil)
		case "response.output_item.done":
			// If a function call arrived whole with no argument deltas, flush its
			// arguments now so the tool sees them.
			var it struct {
				ID        string `json:"id"`
				Type      string `json:"type"`
				Arguments string `json:"arguments"`
			}
			if json.Unmarshal(ev.Item, &it) != nil || it.Type != "function_call" {
				return
			}
			if idx, ok := toolIdx[it.ID]; ok && !streamedArgs[it.ID] && it.Arguments != "" {
				emit(map[string]any{"tool_calls": []any{map[string]any{
					"index": idx, "function": map[string]any{"arguments": it.Arguments},
				}}}, nil, nil)
			}
		case "response.completed":
			finish := "stop"
			if hadTool {
				finish = "tool_calls"
			}
			emit(map[string]any{}, finish, usageFromResponse(ev.Response))
		case "response.failed", "error", "response.error":
			msg := "codex backend stream error"
			var e struct {
				Error struct {
					Message string `json:"message"`
				} `json:"error"`
				Response struct {
					Error struct {
						Message string `json:"message"`
					} `json:"error"`
				} `json:"response"`
			}
			if json.Unmarshal(payload, &e) == nil {
				if e.Error.Message != "" {
					msg = e.Error.Message
				} else if e.Response.Error.Message != "" {
					msg = e.Response.Error.Message
				}
			}
			b, _ := json.Marshal(map[string]any{"error": map[string]any{"message": msg, "type": "server_error"}})
			fmt.Fprintf(w, "data: %s\n\n", b)
			if flush != nil {
				flush()
			}
		}
	})
	fmt.Fprint(w, "data: [DONE]\n\n")
	if flush != nil {
		flush()
	}
}

// usageFromResponse pulls the token counts out of a response.completed event and
// names them the chat way, so tomo's usage accounting stays accurate.
func usageFromResponse(resp json.RawMessage) map[string]any {
	var r struct {
		Usage struct {
			InputTokens        int `json:"input_tokens"`
			OutputTokens       int `json:"output_tokens"`
			TotalTokens        int `json:"total_tokens"`
			InputTokensDetails struct {
				CachedTokens     int `json:"cached_tokens"`
				CacheWriteTokens int `json:"cache_write_tokens"`
			} `json:"input_tokens_details"`
			OutputTokensDetails struct {
				ReasoningTokens int `json:"reasoning_tokens"`
			} `json:"output_tokens_details"`
			// Some rollout events flatten the cached count to the top level; accept
			// either shape so the read rate is never silently dropped.
			CachedInputTokens int `json:"cached_input_tokens"`
		} `json:"usage"`
	}
	if json.Unmarshal(resp, &r) != nil {
		return nil
	}
	total := r.Usage.TotalTokens
	if total == 0 {
		total = r.Usage.InputTokens + r.Usage.OutputTokens
	}
	cached := r.Usage.InputTokensDetails.CachedTokens
	if cached == 0 {
		cached = r.Usage.CachedInputTokens
	}
	usage := map[string]any{
		"prompt_tokens":     r.Usage.InputTokens,
		"completion_tokens": r.Usage.OutputTokens,
		"total_tokens":      total,
	}
	// The codex subscription caches the repeated transcript prefix, so on deep
	// runs most of prompt_tokens is a cache read billed at a fraction of the
	// fresh rate. Surface it the chat way (prompt_tokens_details.cached_tokens)
	// so tomo's usage accounting can credit it instead of counting every round
	// at the full input price.
	if cached > 0 || r.Usage.InputTokensDetails.CacheWriteTokens > 0 {
		usage["prompt_tokens_details"] = map[string]any{
			"cached_tokens":      cached,
			"cache_write_tokens": r.Usage.InputTokensDetails.CacheWriteTokens,
		}
	}
	if r.Usage.OutputTokensDetails.ReasoningTokens > 0 {
		usage["completion_tokens_details"] = map[string]any{
			"reasoning_tokens": r.Usage.OutputTokensDetails.ReasoningTokens,
		}
	}
	return usage
}
