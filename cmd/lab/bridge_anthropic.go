package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// This file gives the bridge an Anthropic Messages front, so claude-code — which
// speaks POST /v1/messages, not chat/completions — runs on the same subscription
// backend as every other tool. The request is translated into the same Responses
// request the chat front produces (so the bridgetrace stays one format and the
// existing reconstruction and the claude native-session audit both work), and the
// Responses SSE the backend streams back is re-emitted as the Anthropic Messages
// event stream claude-code reads. It is the Anthropic-wire mirror of bridge_wire.

// serveAnthropic translates one Anthropic Messages request, forwards it to the
// codex backend, and streams the answer back as Anthropic Messages events. It is
// the /v1/messages sibling of serve: same trace dump (a Responses request under
// <seq>.req.json), same teed <seq>.resp, so a claude run audits and reconstructs
// exactly as a codex or opencode run does.
func (b *bridge) serveAnthropic(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read body: "+err.Error(), http.StatusBadRequest)
		return
	}
	b.mu.Lock()
	b.seq++
	seq := b.seq
	b.mu.Unlock()

	respReq, err := anthropicRequestToResponses(body, b.o.model, b.o.effort)
	if err != nil {
		http.Error(w, "translate: "+err.Error(), http.StatusBadRequest)
		return
	}
	rb, _ := json.Marshal(respReq)
	if b.o.traceDir != "" {
		b.dump(seq, "req", rb)
	}

	resp, err := b.forward(r.Context(), rb, true)
	if err != nil {
		http.Error(w, "backend: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(resp.Body)
		if b.o.traceDir != "" {
			b.dump(seq, "err", msg)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(resp.StatusCode)
		w.Write(msg)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	flush := func() {
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}
	respBody := b.teeResponse(seq, resp.Body)
	responsesStreamToAnthropic(w, flush, respBody, seq, b.o.model)
}

// serveAnthropicCountTokens answers claude-code's POST /v1/messages/count_tokens
// probe. The codex backend exposes no token counter, so we return a cheap
// character-based estimate (~4 chars per token) of the request body. claude-code
// uses the number only to display remaining context, so an estimate keeps the run
// moving without a real tokenizer or an extra backend round-trip.
func (b *bridge) serveAnthropicCountTokens(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	var a struct {
		System   json.RawMessage `json:"system"`
		Messages []struct {
			Content json.RawMessage `json:"content"`
		} `json:"messages"`
	}
	_ = json.Unmarshal(body, &a)
	chars := len(a.System)
	for _, m := range a.Messages {
		chars += len(m.Content)
	}
	if chars == 0 {
		chars = len(body)
	}
	est := chars / 4
	if est < 1 {
		est = 1
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"input_tokens": est})
}

// anthropicBlock is one content block of an Anthropic message, the union of the
// shapes a request carries: text, an assistant tool_use (a call with a JSON
// input object), or a user tool_result (a call's output).
type anthropicBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
	ToolUseID string          `json:"tool_use_id"`
	Content   json.RawMessage `json:"content"` // tool_result output: string or parts
}

// anthropicRequestToResponses converts an Anthropic Messages request into the
// Responses request the codex backend expects, the mirror of
// chatRequestToResponses: the system prompt becomes instructions, each message's
// blocks become input items (text, function_call, function_call_output), and the
// Anthropic tool shape ({name,description,input_schema}) flattens to the
// Responses one. modelOverride and effort are pinned the same way.
func anthropicRequestToResponses(body []byte, modelOverride, effort string) (map[string]any, error) {
	var a struct {
		Model    string          `json:"model"`
		System   json.RawMessage `json:"system"`
		Messages []struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		} `json:"messages"`
		Tools      []json.RawMessage `json:"tools"`
		ToolChoice json.RawMessage   `json:"tool_choice"`
	}
	if err := json.Unmarshal(body, &a); err != nil {
		return nil, err
	}

	var instructions []string
	if s := anthropicSystemText(a.System); s != "" {
		instructions = append(instructions, s)
	}
	input := []any{}
	for _, m := range a.Messages {
		for _, bl := range anthropicBlocks(m.Content) {
			switch bl.Type {
			case "text":
				if strings.TrimSpace(bl.Text) == "" {
					continue
				}
				if m.Role == "assistant" {
					input = append(input, msgItem("assistant", "output_text", bl.Text))
				} else {
					input = append(input, msgItem("user", "input_text", bl.Text))
				}
			case "tool_use":
				args := strings.TrimSpace(string(bl.Input))
				if args == "" {
					args = "{}"
				}
				input = append(input, map[string]any{
					"type":      "function_call",
					"call_id":   bl.ID,
					"name":      bl.Name,
					"arguments": args,
				})
			case "tool_result":
				input = append(input, map[string]any{
					"type":    "function_call_output",
					"call_id": bl.ToolUseID,
					"output":  anthropicResultText(bl.Content),
				})
			}
		}
	}

	model := a.Model
	if modelOverride != "" {
		model = modelOverride
	}
	req := map[string]any{
		"model":               model,
		"instructions":        strings.Join(instructions, "\n\n"),
		"input":               input,
		"tool_choice":         anthropicToolChoice(a.ToolChoice),
		"parallel_tool_calls": false,
		"store":               false,
		"stream":              true,
	}
	if tools := anthropicToolsToResponses(a.Tools); len(tools) > 0 {
		req["tools"] = tools
	}
	if effort != "" {
		req["reasoning"] = map[string]any{"effort": effort, "summary": "auto"}
	}
	return req, nil
}

// anthropicSystemText flattens the system field, which is either a plain string
// or an array of {type:text,text} blocks, into one instruction string.
func anthropicSystemText(raw json.RawMessage) string {
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
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &parts) == nil {
		var b strings.Builder
		for _, p := range parts {
			b.WriteString(p.Text)
		}
		return b.String()
	}
	return ""
}

// anthropicBlocks reads a message's content, which is either a plain string (the
// opening user prompt) or an array of typed blocks. A string becomes one text
// block so no turn is dropped for its encoding.
func anthropicBlocks(raw json.RawMessage) []anthropicBlock {
	t := strings.TrimSpace(string(raw))
	if t == "" || t == "null" {
		return nil
	}
	if t[0] == '"' {
		var s string
		if json.Unmarshal(raw, &s) == nil {
			return []anthropicBlock{{Type: "text", Text: s}}
		}
	}
	var blocks []anthropicBlock
	if json.Unmarshal(raw, &blocks) == nil {
		return blocks
	}
	return nil
}

// anthropicResultText flattens a tool_result's content (a string or an array of
// {type:text,text} parts) into the plain output string the Responses output
// field carries.
func anthropicResultText(raw json.RawMessage) string {
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
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &parts) == nil {
		var b strings.Builder
		for _, p := range parts {
			b.WriteString(p.Text)
		}
		return b.String()
	}
	return string(raw)
}

// anthropicToolsToResponses flattens the Anthropic tool shape
// ({name,description,input_schema}) into the flat Responses function tool
// ({type:function,name,description,parameters}).
func anthropicToolsToResponses(tools []json.RawMessage) []map[string]any {
	out := []map[string]any{}
	for _, raw := range tools {
		var t struct {
			Name        string          `json:"name"`
			Description string          `json:"description"`
			InputSchema json.RawMessage `json:"input_schema"`
		}
		if json.Unmarshal(raw, &t) != nil || t.Name == "" {
			continue
		}
		fn := map[string]any{"type": "function", "name": t.Name}
		if t.Description != "" {
			fn["description"] = t.Description
		}
		if len(t.InputSchema) > 0 {
			fn["parameters"] = json.RawMessage(t.InputSchema)
		} else {
			fn["parameters"] = map[string]any{"type": "object", "properties": map[string]any{}}
		}
		out = append(out, fn)
	}
	return out
}

// anthropicToolChoice maps the Anthropic tool_choice ({type:auto|any|tool,name})
// onto the Responses form. auto and any pass through as their Responses spelling;
// a named tool becomes {type:function,name}.
func anthropicToolChoice(raw json.RawMessage) any {
	t := strings.TrimSpace(string(raw))
	if t == "" || t == "null" {
		return "auto"
	}
	var m struct {
		Type string `json:"type"`
		Name string `json:"name"`
	}
	if json.Unmarshal(raw, &m) != nil {
		return "auto"
	}
	switch m.Type {
	case "any":
		return "required"
	case "tool":
		if m.Name != "" {
			return map[string]any{"type": "function", "name": m.Name}
		}
	}
	return "auto"
}

// responsesStreamToAnthropic consumes the Responses SSE stream the codex backend
// returns and re-emits it as the Anthropic Messages event stream claude-code
// reads: a message_start, then one content block per text run or tool call
// (text_delta for text, input_json_delta for a call's arguments), each framed by
// content_block_start/stop, closed by a message_delta carrying the stop reason
// and output token count and a final message_stop. It is the mirror of
// responsesStreamToChat. Only one content block is open at a time, so the block
// indices stay sequential the way claude-code's parser expects.
func responsesStreamToAnthropic(w io.Writer, flush func(), r io.Reader, seq int, model string) {
	send := func(event string, obj map[string]any) {
		b, _ := json.Marshal(obj)
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, b)
		if flush != nil {
			flush()
		}
	}

	send("message_start", map[string]any{
		"type": "message_start",
		"message": map[string]any{
			"id": fmt.Sprintf("msg_bridge_%d", seq), "type": "message", "role": "assistant",
			"model": model, "content": []any{}, "stop_reason": nil, "stop_sequence": nil,
			"usage": map[string]any{"input_tokens": 0, "output_tokens": 0},
		},
	})

	const (
		blockNone = iota
		blockText
		blockTool
	)
	openKind := blockNone
	openIndex := -1
	nextIndex := 0
	hadTool := false
	toolIndex := map[string]int{} // Responses item_id -> content block index
	streamedArgs := map[string]bool{}

	closeBlock := func() {
		if openKind != blockNone {
			send("content_block_stop", map[string]any{"type": "content_block_stop", "index": openIndex})
			openKind = blockNone
			openIndex = -1
		}
	}
	openText := func() {
		if openKind == blockText {
			return
		}
		closeBlock()
		openIndex = nextIndex
		nextIndex++
		openKind = blockText
		send("content_block_start", map[string]any{
			"type": "content_block_start", "index": openIndex,
			"content_block": map[string]any{"type": "text", "text": ""},
		})
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
			if ev.Delta == "" {
				return
			}
			openText()
			send("content_block_delta", map[string]any{
				"type": "content_block_delta", "index": openIndex,
				"delta": map[string]any{"type": "text_delta", "text": ev.Delta},
			})
		case "response.output_item.added":
			var it struct {
				ID     string `json:"id"`
				Type   string `json:"type"`
				CallID string `json:"call_id"`
				Name   string `json:"name"`
			}
			if json.Unmarshal(ev.Item, &it) != nil || it.Type != "function_call" {
				return
			}
			closeBlock()
			openIndex = nextIndex
			nextIndex++
			openKind = blockTool
			hadTool = true
			toolIndex[it.ID] = openIndex
			send("content_block_start", map[string]any{
				"type": "content_block_start", "index": openIndex,
				"content_block": map[string]any{"type": "tool_use", "id": it.CallID, "name": it.Name, "input": map[string]any{}},
			})
		case "response.function_call_arguments.delta":
			idx, ok := toolIndex[ev.ItemID]
			if !ok || ev.Delta == "" {
				return
			}
			streamedArgs[ev.ItemID] = true
			send("content_block_delta", map[string]any{
				"type": "content_block_delta", "index": idx,
				"delta": map[string]any{"type": "input_json_delta", "partial_json": ev.Delta},
			})
		case "response.output_item.done":
			// A whole function call with no argument deltas: emit its arguments as one
			// input_json_delta so claude-code sees the call's input.
			var it struct {
				ID        string `json:"id"`
				Type      string `json:"type"`
				Arguments string `json:"arguments"`
			}
			if json.Unmarshal(ev.Item, &it) != nil || it.Type != "function_call" {
				return
			}
			if idx, ok := toolIndex[it.ID]; ok && !streamedArgs[it.ID] && it.Arguments != "" {
				send("content_block_delta", map[string]any{
					"type": "content_block_delta", "index": idx,
					"delta": map[string]any{"type": "input_json_delta", "partial_json": it.Arguments},
				})
			}
		case "response.completed":
			closeBlock()
			stop := "end_turn"
			if hadTool {
				stop = "tool_use"
			}
			send("message_delta", map[string]any{
				"type":  "message_delta",
				"delta": map[string]any{"stop_reason": stop, "stop_sequence": nil},
				"usage": anthropicUsage(ev.Response),
			})
			send("message_stop", map[string]any{"type": "message_stop"})
		case "response.failed", "error", "response.error":
			closeBlock()
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
			send("error", map[string]any{"type": "error", "error": map[string]any{"type": "api_error", "message": msg}})
		}
	})
}

// anthropicUsage pulls the token counts out of a response.completed event and
// names them the Anthropic way for the message_delta, so claude-code's usage
// accounting stays accurate.
func anthropicUsage(resp json.RawMessage) map[string]any {
	var r struct {
		Usage struct {
			InputTokens        int `json:"input_tokens"`
			OutputTokens       int `json:"output_tokens"`
			InputTokensDetails struct {
				CachedTokens int `json:"cached_tokens"`
			} `json:"input_tokens_details"`
			CachedInputTokens int `json:"cached_input_tokens"`
		} `json:"usage"`
	}
	if json.Unmarshal(resp, &r) != nil {
		return map[string]any{"output_tokens": 0}
	}
	cached := r.Usage.InputTokensDetails.CachedTokens
	if cached == 0 {
		cached = r.Usage.CachedInputTokens
	}
	u := map[string]any{
		"input_tokens":  r.Usage.InputTokens,
		"output_tokens": r.Usage.OutputTokens,
	}
	if cached > 0 {
		u["cache_read_input_tokens"] = cached
	}
	return u
}
