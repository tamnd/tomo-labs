// This file lets the proxy speak the OpenAI Responses API at its edge while the
// upstream stays chat-only. Our shared free model (deepseek) is served by zen
// over /v1/chat/completions, but codex only speaks /v1/responses, so without a
// shim codex could not run against the same model every other tool uses.
//
// serveResponses accepts a /responses request, rewrites it into a chat
// completions request, forwards it to the same upstream every other tool hits,
// and rewrites the reply back into the Responses shape codex expects, streaming
// or not. Usage and latency get recorded the same way as a plain proxied call,
// so codex lands in the metrics on equal footing.
package proxy

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// isResponses reports whether a request is a Responses-API call the proxy
// should translate rather than forward verbatim. codex posts to /v1/responses;
// everything else (chat, messages, models) passes straight through.
func isResponses(r *http.Request) bool {
	return r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/responses")
}

// chatPathOf maps a /responses path onto its /chat/completions sibling, keeping
// any version prefix intact (so /v1/responses becomes /v1/chat/completions).
func chatPathOf(p string) string {
	return strings.TrimSuffix(p, "/responses") + "/chat/completions"
}

// serveResponses drives one Responses-API call end to end: read, translate to
// chat, forward upstream with the caller's key, and translate the reply back.
func (t *tap) serveResponses(w http.ResponseWriter, r *http.Request, target *url.URL) {
	seq := t.next()
	start := time.Now()
	t.markStart(seq, start)

	body, _ := io.ReadAll(r.Body)
	chatBody, stream, err := toChatRequest(body)
	if err != nil {
		http.Error(w, "responses translate: "+err.Error(), http.StatusBadRequest)
		t.recordLatency(seq, start, time.Now(), time.Time{}, chatPathOf(r.URL.Path), http.StatusBadRequest)
		return
	}
	// Force the shared decoding knobs the same way the pass-through path does, so
	// codex is judged under the identical regime as every other tool.
	chatBody = t.det.apply(chatBody)
	t.writeJSON(t.reqs, reqRecord{
		Seq:    seq,
		TS:     nowStamp(),
		Method: http.MethodPost,
		Path:   chatPathOf(r.URL.Path) + " (from responses)",
		Body:   sanitize(chatBody),
	})

	up := *target
	up.Path = singleJoin(target.Path, chatPathOf(r.URL.Path))
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, up.String(), bytes.NewReader(chatBody))
	if err != nil {
		http.Error(w, "responses upstream: "+err.Error(), http.StatusBadGateway)
		t.recordLatency(seq, start, time.Now(), time.Time{}, chatPathOf(r.URL.Path), http.StatusBadGateway)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	if a := r.Header.Get("Authorization"); a != "" {
		req.Header.Set("Authorization", a)
	}
	if stream {
		req.Header.Set("Accept", "text/event-stream")
	}

	resp, err := t.client.Do(req)
	if err != nil {
		http.Error(w, "responses upstream: "+err.Error(), http.StatusBadGateway)
		t.recordLatency(seq, start, time.Now(), time.Time{}, chatPathOf(r.URL.Path), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// An upstream error is not ours to reshape: pass status and body through so
	// codex sees the real failure, and still log a latency row for the attempt.
	if resp.StatusCode != http.StatusOK {
		w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
		w.WriteHeader(resp.StatusCode)
		b, _ := io.ReadAll(resp.Body)
		w.Write(b)
		t.recordLatency(seq, start, time.Now(), time.Now(), chatPathOf(r.URL.Path), resp.StatusCode)
		return
	}

	if stream {
		t.streamResponses(w, resp, seq, start, r.URL.Path)
		return
	}
	t.jsonResponses(w, resp, seq, start, r.URL.Path)
}

// respItem is one element of a Responses-API input array. The three shapes that
// matter for a coding agent are a message, a prior tool call (function_call),
// and a tool result (function_call_output); anything else is skipped.
type respItem struct {
	Type      string          `json:"type"`
	Role      string          `json:"role"`
	Content   json.RawMessage `json:"content"`
	CallID    string          `json:"call_id"`
	Name      string          `json:"name"`
	Arguments string          `json:"arguments"`
	Output    json.RawMessage `json:"output"`
}

// toChatRequest converts a Responses-API request body into a Chat Completions
// body and reports whether the caller asked to stream. The mapping is: the
// top-level instructions become a system message, the input array becomes the
// message list, and the flat tool shape becomes the nested chat tool shape.
func toChatRequest(body []byte) (chat []byte, stream bool, err error) {
	var rr struct {
		Model            string            `json:"model"`
		Instructions     string            `json:"instructions"`
		Input            json.RawMessage   `json:"input"`
		Tools            []json.RawMessage `json:"tools"`
		ToolChoice       json.RawMessage   `json:"tool_choice"`
		Temperature      json.RawMessage   `json:"temperature"`
		TopP             json.RawMessage   `json:"top_p"`
		MaxOutputTokens  json.RawMessage   `json:"max_output_tokens"`
		ParallelToolCall json.RawMessage   `json:"parallel_tool_calls"`
		Stream           bool              `json:"stream"`
	}
	if err = json.Unmarshal(body, &rr); err != nil {
		return nil, false, err
	}

	msgs := []map[string]any{}
	if strings.TrimSpace(rr.Instructions) != "" {
		msgs = append(msgs, map[string]any{"role": "system", "content": rr.Instructions})
	}
	items, err := parseInput(rr.Input)
	if err != nil {
		return nil, false, err
	}
	// A run of function_call items collapses into one assistant message carrying
	// all its tool_calls, which is the shape chat completions expects before the
	// matching tool results arrive.
	var pending []map[string]any
	flush := func() {
		if len(pending) > 0 {
			msgs = append(msgs, map[string]any{"role": "assistant", "content": nil, "tool_calls": pending})
			pending = nil
		}
	}
	for _, it := range items {
		typ := it.Type
		if typ == "" && it.Role != "" {
			typ = "message"
		}
		switch typ {
		case "message":
			flush()
			msgs = append(msgs, map[string]any{"role": chatRole(it.Role), "content": contentText(it.Content)})
		case "function_call":
			pending = append(pending, map[string]any{
				"id":       it.CallID,
				"type":     "function",
				"function": map[string]any{"name": it.Name, "arguments": it.Arguments},
			})
		case "function_call_output":
			flush()
			msgs = append(msgs, map[string]any{"role": "tool", "tool_call_id": it.CallID, "content": contentText(it.Output)})
		}
	}
	flush()

	chatMap := map[string]any{"model": rr.Model, "messages": msgs}
	if tools := toChatTools(rr.Tools); len(tools) > 0 {
		chatMap["tools"] = tools
	}
	if len(rr.ToolChoice) > 0 {
		chatMap["tool_choice"] = toChatToolChoice(rr.ToolChoice)
	}
	if len(rr.Temperature) > 0 {
		chatMap["temperature"] = rr.Temperature
	}
	if len(rr.TopP) > 0 {
		chatMap["top_p"] = rr.TopP
	}
	if len(rr.MaxOutputTokens) > 0 {
		chatMap["max_tokens"] = rr.MaxOutputTokens
	}
	if len(rr.ParallelToolCall) > 0 {
		chatMap["parallel_tool_calls"] = rr.ParallelToolCall
	}
	chatMap["stream"] = rr.Stream
	if rr.Stream {
		// Ask upstream to append a final usage chunk, which chat streaming omits by
		// default, so the translated response.completed carries real token counts.
		chatMap["stream_options"] = map[string]any{"include_usage": true}
	}
	out, err := json.Marshal(chatMap)
	return out, rr.Stream, err
}

// chatRole maps a Responses-API message role onto one chat completions accepts.
// codex tags its harness instructions with the developer role, which the newer
// OpenAI wire allows but the deepseek chat endpoint rejects, so it folds into
// system; an empty role defaults to user.
func chatRole(role string) string {
	switch role {
	case "", "user":
		return "user"
	case "developer", "system":
		return "system"
	default:
		return role
	}
}

// parseInput accepts the two Responses-API input shapes: a bare string (one user
// message) or an array of items.
func parseInput(raw json.RawMessage) ([]respItem, error) {
	t := bytes.TrimSpace(raw)
	if len(t) == 0 {
		return nil, nil
	}
	if t[0] == '"' {
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return nil, err
		}
		return []respItem{{Type: "message", Role: "user", Content: raw}}, nil
	}
	var items []respItem
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, err
	}
	return items, nil
}

// contentText flattens a Responses-API content value into a plain string. It
// handles a bare string, an array of typed parts (input_text/output_text/text),
// and falls back to the raw JSON so nothing is silently dropped.
func contentText(raw json.RawMessage) string {
	t := bytes.TrimSpace(raw)
	if len(t) == 0 {
		return ""
	}
	if t[0] == '"' {
		var s string
		if json.Unmarshal(raw, &s) == nil {
			return s
		}
	}
	if t[0] == '[' {
		var parts []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}
		if json.Unmarshal(raw, &parts) == nil {
			var b strings.Builder
			for _, p := range parts {
				b.WriteString(p.Text)
			}
			return b.String()
		}
	}
	return string(raw)
}

// toChatTools rewrites the flat Responses-API function tool shape into the
// nested chat shape. A tool already carrying a function object passes through.
func toChatTools(tools []json.RawMessage) []map[string]any {
	out := []map[string]any{}
	for _, raw := range tools {
		var t struct {
			Type        string          `json:"type"`
			Name        string          `json:"name"`
			Description string          `json:"description"`
			Parameters  json.RawMessage `json:"parameters"`
			Function    json.RawMessage `json:"function"`
		}
		if json.Unmarshal(raw, &t) != nil {
			continue
		}
		if t.Type != "" && t.Type != "function" {
			continue
		}
		if len(t.Function) > 0 {
			out = append(out, map[string]any{"type": "function", "function": json.RawMessage(t.Function)})
			continue
		}
		fn := map[string]any{"name": t.Name}
		if t.Description != "" {
			fn["description"] = t.Description
		}
		if len(t.Parameters) > 0 {
			fn["parameters"] = json.RawMessage(t.Parameters)
		}
		out = append(out, map[string]any{"type": "function", "function": fn})
	}
	return out
}

// toChatToolChoice rewrites a tool_choice value. The string forms
// (auto/none/required) pass through; the {type:function,name} object gets the
// name nested under a function object, which is where chat expects it.
func toChatToolChoice(raw json.RawMessage) any {
	t := bytes.TrimSpace(raw)
	if len(t) > 0 && t[0] == '"' {
		return json.RawMessage(raw)
	}
	var m struct {
		Type string `json:"type"`
		Name string `json:"name"`
	}
	if json.Unmarshal(raw, &m) == nil && m.Name != "" {
		return map[string]any{"type": "function", "function": map[string]any{"name": m.Name}}
	}
	return json.RawMessage(raw)
}

// chatChunk is the slice of a chat streaming chunk the translator reads.
type chatChunk struct {
	Model   string `json:"model"`
	Choices []struct {
		Delta struct {
			Content   string `json:"content"`
			ToolCalls []struct {
				Index    int    `json:"index"`
				ID       string `json:"id"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage json.RawMessage `json:"usage"`
}

// jsonResponses translates a single (non-streamed) chat completion into a
// Responses object and records usage and latency.
func (t *tap) jsonResponses(w http.ResponseWriter, resp *http.Response, seq int, start time.Time, inPath string) {
	b, _ := io.ReadAll(resp.Body)
	first := time.Now()
	obj := chatToResponsesJSON(b, seq)
	out, _ := json.Marshal(obj)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(out)
	if u := extractUsage(b); u != nil {
		u.Seq = seq
		u.TS = nowStamp()
		u.Status = http.StatusOK
		t.writeJSON(t.usage, u)
	}
	t.recordLatency(seq, start, time.Now(), first, chatPathOf(inPath), http.StatusOK)
}

// chatToResponsesJSON assembles a completed Responses object from one chat
// completion: text becomes a message item, each tool_call a function_call item.
func chatToResponsesJSON(chat []byte, seq int) map[string]any {
	var c struct {
		Model   string `json:"model"`
		Choices []struct {
			Message struct {
				Content   string `json:"content"`
				ToolCalls []struct {
					ID       string `json:"id"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
		Usage json.RawMessage `json:"usage"`
	}
	json.Unmarshal(chat, &c)
	output := []any{}
	if len(c.Choices) > 0 {
		m := c.Choices[0].Message
		if m.Content != "" {
			output = append(output, map[string]any{
				"id": fmt.Sprintf("msg_lab_%d", seq), "type": "message", "status": "completed",
				"role": "assistant", "content": []any{textPart(m.Content)},
			})
		}
		for i, tc := range m.ToolCalls {
			output = append(output, map[string]any{
				"id": fmt.Sprintf("fc_lab_%d_%d", seq, i), "type": "function_call", "status": "completed",
				"call_id": tc.ID, "name": tc.Function.Name, "arguments": tc.Function.Arguments,
			})
		}
	}
	obj := responseEnvelope(fmt.Sprintf("resp_lab_%d", seq), c.Model, "completed", output)
	obj["usage"] = responsesUsageFrom(c.Usage)
	return obj
}

// responsesUsageFrom maps a chat usage block onto the Responses token names
// (input_tokens/output_tokens), carrying the cached-prompt count across the two
// spellings a provider might use.
func responsesUsageFrom(chatUsage json.RawMessage) map[string]any {
	var u struct {
		PromptTokens        int `json:"prompt_tokens"`
		CompletionTokens    int `json:"completion_tokens"`
		TotalTokens         int `json:"total_tokens"`
		PromptCacheHitToks  int `json:"prompt_cache_hit_tokens"`
		PromptTokensDetails *struct {
			CachedTokens int `json:"cached_tokens"`
		} `json:"prompt_tokens_details"`
	}
	if len(chatUsage) > 0 {
		json.Unmarshal(chatUsage, &u)
	}
	cached := 0
	if u.PromptTokensDetails != nil {
		cached = u.PromptTokensDetails.CachedTokens
	}
	if u.PromptCacheHitToks > 0 {
		cached = u.PromptCacheHitToks
	}
	return map[string]any{
		"input_tokens":          u.PromptTokens,
		"input_tokens_details":  map[string]any{"cached_tokens": cached},
		"output_tokens":         u.CompletionTokens,
		"output_tokens_details": map[string]any{"reasoning_tokens": 0},
		"total_tokens":          u.TotalTokens,
	}
}

// textPart is the output_text content part codex expects inside a message item.
func textPart(text string) map[string]any {
	return map[string]any{"type": "output_text", "text": text, "annotations": []any{}}
}

// responseEnvelope builds a Responses object with the nullable fields codex's
// decoder expects present, so a strict client does not choke on a lean reply.
func responseEnvelope(id, model, status string, output []any) map[string]any {
	return map[string]any{
		"id": id, "object": "response", "status": status, "model": model,
		"output": output, "created_at": 0, "error": nil, "incomplete_details": nil,
		"instructions": nil, "max_output_tokens": nil, "metadata": map[string]any{},
		"parallel_tool_calls": true, "temperature": nil, "top_p": nil,
		"tool_choice": "auto", "tools": []any{}, "reasoning": nil,
	}
}

// streamResponses reads the upstream chat SSE stream and re-emits it as the
// Responses-API event stream codex consumes, recording usage and latency once
// the stream ends.
func (t *tap) streamResponses(w http.ResponseWriter, resp *http.Response, seq int, start time.Time, inPath string) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	fl, _ := w.(http.Flusher)

	s := &respStream{w: w, fl: fl, id: fmt.Sprintf("resp_lab_%d", seq), tools: map[int]*toolAcc{}}
	s.begin()

	var firstAt time.Time
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 64*1024), 8*1024*1024)
	for sc.Scan() {
		if firstAt.IsZero() {
			firstAt = time.Now()
		}
		line := bytes.TrimSpace(sc.Bytes())
		if !bytes.HasPrefix(line, []byte("data:")) {
			continue
		}
		payload := bytes.TrimSpace(line[len("data:"):])
		if len(payload) == 0 || bytes.Equal(payload, []byte("[DONE]")) {
			continue
		}
		var c chatChunk
		if json.Unmarshal(payload, &c) != nil {
			continue
		}
		if s.model == "" && c.Model != "" {
			s.model = c.Model
		}
		s.chunk(c)
	}
	s.finish()
	done := time.Now()

	if len(s.usage) > 0 {
		synth := append(append([]byte(`{"usage":`), s.usage...), '}')
		if u := extractUsage(synth); u != nil {
			u.Seq = seq
			u.TS = nowStamp()
			u.Status = http.StatusOK
			t.writeJSON(t.usage, u)
		}
	}
	t.recordLatency(seq, start, done, firstAt, chatPathOf(inPath), http.StatusOK)
}

// respStream turns the flat chat delta stream into the item-structured Responses
// event stream. It tracks one optional assistant text message plus any number of
// function_call items, assigning each an output_index as it first appears.
type respStream struct {
	w     http.ResponseWriter
	fl    http.Flusher
	id    string
	model string
	evNo  int
	next  int // next output_index to hand out

	textStarted bool
	textIdx     int
	textID      string
	textBuf     strings.Builder

	tools map[int]*toolAcc // keyed by the chat tool_call index
	order []int            // tool indices in first-seen order

	usage json.RawMessage
}

// toolAcc accumulates one streamed function call: its ids, name, and the
// argument JSON that arrives in fragments.
type toolAcc struct {
	idx    int
	id     string
	callID string
	name   string
	args   strings.Builder
}

// emit writes one Responses SSE event, both the event: name and the data: JSON,
// stamping an incrementing sequence_number the way the real API does.
func (s *respStream) emit(typ string, obj map[string]any) {
	obj["type"] = typ
	obj["sequence_number"] = s.evNo
	s.evNo++
	b, _ := json.Marshal(obj)
	fmt.Fprintf(s.w, "event: %s\ndata: %s\n\n", typ, b)
	if s.fl != nil {
		s.fl.Flush()
	}
}

// begin announces the response before any content, which is the handshake codex
// waits for.
func (s *respStream) begin() {
	env := responseEnvelope(s.id, s.model, "in_progress", []any{})
	s.emit("response.created", map[string]any{"response": env})
	s.emit("response.in_progress", map[string]any{"response": env})
}

// chunk folds one chat streaming chunk into the running item state, emitting the
// matching Responses delta events as text and tool arguments arrive.
func (s *respStream) chunk(c chatChunk) {
	for _, ch := range c.Choices {
		if ch.Delta.Content != "" {
			if !s.textStarted {
				s.textStarted = true
				s.textIdx = s.next
				s.next++
				s.textID = strings.Replace(s.id, "resp_", "msg_", 1)
				s.emit("response.output_item.added", map[string]any{
					"output_index": s.textIdx,
					"item": map[string]any{"id": s.textID, "type": "message", "status": "in_progress",
						"role": "assistant", "content": []any{}},
				})
				s.emit("response.content_part.added", map[string]any{
					"item_id": s.textID, "output_index": s.textIdx, "content_index": 0, "part": textPart(""),
				})
			}
			s.textBuf.WriteString(ch.Delta.Content)
			s.emit("response.output_text.delta", map[string]any{
				"item_id": s.textID, "output_index": s.textIdx, "content_index": 0, "delta": ch.Delta.Content,
			})
		}
		for _, tc := range ch.Delta.ToolCalls {
			acc := s.tools[tc.Index]
			if acc == nil {
				acc = &toolAcc{idx: s.next, id: fmt.Sprintf("%s_fc%d", strings.Replace(s.id, "resp_", "fc_", 1), tc.Index),
					callID: tc.ID, name: tc.Function.Name}
				s.next++
				s.tools[tc.Index] = acc
				s.order = append(s.order, tc.Index)
				s.emit("response.output_item.added", map[string]any{
					"output_index": acc.idx,
					"item": map[string]any{"id": acc.id, "type": "function_call", "status": "in_progress",
						"call_id": acc.callID, "name": acc.name, "arguments": ""},
				})
			}
			if tc.ID != "" && acc.callID == "" {
				acc.callID = tc.ID
			}
			if tc.Function.Name != "" && acc.name == "" {
				acc.name = tc.Function.Name
			}
			if tc.Function.Arguments != "" {
				acc.args.WriteString(tc.Function.Arguments)
				s.emit("response.function_call_arguments.delta", map[string]any{
					"item_id": acc.id, "output_index": acc.idx, "delta": tc.Function.Arguments,
				})
			}
		}
	}
	if len(c.Usage) > 0 {
		s.usage = c.Usage
	}
}

// finish closes every open item and emits response.completed with the assembled
// output and the translated usage.
func (s *respStream) finish() {
	if s.textStarted {
		s.emit("response.output_text.done", map[string]any{
			"item_id": s.textID, "output_index": s.textIdx, "content_index": 0, "text": s.textBuf.String(),
		})
		s.emit("response.content_part.done", map[string]any{
			"item_id": s.textID, "output_index": s.textIdx, "content_index": 0, "part": textPart(s.textBuf.String()),
		})
		s.emit("response.output_item.done", map[string]any{"output_index": s.textIdx, "item": s.textItem()})
	}
	for _, idx := range s.order {
		acc := s.tools[idx]
		s.emit("response.function_call_arguments.done", map[string]any{
			"item_id": acc.id, "output_index": acc.idx, "arguments": acc.args.String(),
		})
		s.emit("response.output_item.done", map[string]any{"output_index": acc.idx, "item": s.toolItem(acc)})
	}
	env := responseEnvelope(s.id, s.model, "completed", s.outputItems())
	env["usage"] = responsesUsageFrom(s.usage)
	s.emit("response.completed", map[string]any{"response": env})
}

// outputItems rebuilds the final output array in output_index order.
func (s *respStream) outputItems() []any {
	arr := make([]any, s.next)
	if s.textStarted {
		arr[s.textIdx] = s.textItem()
	}
	for _, idx := range s.order {
		acc := s.tools[idx]
		arr[acc.idx] = s.toolItem(acc)
	}
	out := []any{}
	for _, v := range arr {
		if v != nil {
			out = append(out, v)
		}
	}
	return out
}

func (s *respStream) textItem() map[string]any {
	return map[string]any{"id": s.textID, "type": "message", "status": "completed",
		"role": "assistant", "content": []any{textPart(s.textBuf.String())}}
}

func (s *respStream) toolItem(acc *toolAcc) map[string]any {
	return map[string]any{"id": acc.id, "type": "function_call", "status": "completed",
		"call_id": acc.callID, "name": acc.name, "arguments": acc.args.String()}
}
