package trace

import (
	"bytes"
	"encoding/json"
	"hash/fnv"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// A Responses/bridge capture records each upstream call as a request under
// <traceDir>/bridgetrace/NNNN.req.json, holding an `instructions` system prompt
// and an `input` array of typed items (message, reasoning, function_call,
// function_call_output, and the codex custom_tool_call variants), and the teed
// response stream alongside it as NNNN.resp.
//
// A single request's input is NOT the whole conversation. An agent that runs a
// straight loop (codex, pi) does send a monotonically growing history, so its
// last request holds every turn; but an agent that spawns sub-tasks (opencode's
// task tool, its title/summary side-calls) runs each on its own request thread
// that starts small again, so its calls live only in those threads' requests and
// never appear in the main thread's input. The native session log flattens every
// thread into one store, so reconstructing from any single request undercounts:
// on dynaconf-1225 opencode's richest request carried 101 of the 183 tool calls
// its native store recorded, the other 82 belonging to side threads. The union of
// every request's input, deduplicated by call id, recovers all 183.

// responsesMessages reconstructs the message list from a bridgetrace, or reports
// ok false when the directory is not a bridgetrace. It unions the input items of
// every request across every thread, keeping each distinct item on first sight in
// request order, then folds in each teed response's final output items, so a run
// that fanned out into sub-task threads reconstructs as completely as a flat one.
func responsesMessages(traceDir, model string) ([]Message, bool) {
	dir := filepath.Join(traceDir, "bridgetrace")
	reqs, _ := filepath.Glob(filepath.Join(dir, "*.req.json"))
	if len(reqs) == 0 {
		return nil, false
	}
	sort.Slice(reqs, func(i, j int) bool { return bridgeReqSeq(reqs[i]) < bridgeReqSeq(reqs[j]) })

	var msgs []Message
	seen := map[string]bool{}
	var instructions string
	add := func(it respItem) {
		key := itemKey(it)
		if key == "" || seen[key] {
			return
		}
		if m, ok := itemMessage(it); ok {
			seen[key] = true
			msgs = append(msgs, m)
		}
	}

	for _, f := range reqs {
		raw, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var req bridgeReq
		if json.Unmarshal(raw, &req) != nil {
			continue
		}
		if instructions == "" && strings.TrimSpace(req.Instructions) != "" {
			instructions = req.Instructions
		}
		for _, it := range req.Input {
			add(it)
		}
	}

	// Each teed response carries its thread's final turn, the one no later request
	// echoes back; fold every response's output items in, deduped against what the
	// requests already hold so a turn a subsequent request repeated is not doubled.
	resps, _ := filepath.Glob(filepath.Join(dir, "*.resp"))
	sort.Slice(resps, func(i, j int) bool { return bridgeSeq(resps[i]) < bridgeSeq(resps[j]) })
	for _, rf := range resps {
		body, err := os.ReadFile(rf)
		if err != nil {
			continue
		}
		for _, it := range finalOutput(body) {
			add(it)
		}
	}

	if instructions != "" {
		msgs = append([]Message{{Role: "system", Content: []Block{textBlock(instructions)}}}, msgs...)
	}
	return msgs, true
}

// itemKey is the identity of a Responses item for union deduplication across
// request threads. A tool call and its output are keyed by call id (the id a
// following output refers back to), so the same call echoed in many requests'
// histories counts once; a message or reasoning item, which carries no id, is
// keyed by role and a hash of its text. An item with no renderable content
// returns "" and is skipped.
func itemKey(it respItem) string {
	switch it.Type {
	case "function_call", "custom_tool_call":
		id := it.CallID
		if id == "" {
			id = it.ID
		}
		if id == "" {
			// No id to pair on (rare): fall back to name plus argument text so two
			// genuinely distinct calls do not collapse into one.
			args := it.Arguments
			if args == "" {
				args = it.Input
			}
			return "call:" + it.Name + ":" + hashText(args)
		}
		return "call:" + id
	case "function_call_output", "custom_tool_call_output":
		return "out:" + it.CallID
	case "message":
		return "msg:" + it.Role + ":" + hashText(joinParts(it.Content))
	case "reasoning":
		return "rsn:" + hashText(joinParts(it.Summary))
	default:
		return ""
	}
}

// hashText is a short, dependency-light fingerprint of a block of text, used only
// to give id-less items (messages, reasoning) a stable union key.
func hashText(s string) string {
	h := fnv.New64a()
	_, _ = h.Write([]byte(s))
	return strconv.FormatUint(h.Sum64(), 36)
}

// bridgeReqSeq is the leading sequence number of a NNNN.req.json capture, so the
// union walks requests in the order they were sent.
func bridgeReqSeq(path string) int {
	base := filepath.Base(path)
	if i := strings.IndexByte(base, '.'); i >= 0 {
		base = base[:i]
	}
	n, err := strconv.Atoi(base)
	if err != nil {
		return 0
	}
	return n
}

// respItem is one item of a Responses `input` or `output` array, the union of
// every shape a bridgetrace carries. Type selects which fields are meaningful.
type respItem struct {
	Type string `json:"type"`
	Role string `json:"role"`

	// message: content is an array of text parts under one of the Responses part
	// type names (input_text, output_text, text).
	Content []respPart `json:"content"`

	// reasoning: a summary array of text parts.
	Summary []respPart `json:"summary"`

	// function_call and custom_tool_call: the call's name, its arguments (a JSON
	// string) or its input (the codex custom-tool payload), and the id a following
	// output item refers back to.
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
	Input     string `json:"input"`
	CallID    string `json:"call_id"`
	ID        string `json:"id"`

	// function_call_output and custom_tool_call_output: the tool result, a string
	// or an array of parts.
	Output json.RawMessage `json:"output"`
}

// respPart is one part of a Responses content or summary array. The several part
// type names all put their text in the same field.
type respPart struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// bridgeReq is the subset of a bridgetrace request capture this decoder reads.
type bridgeReq struct {
	Instructions string     `json:"instructions"`
	Input        []respItem `json:"input"`
}

// itemMessage maps one Responses item to a Message, or reports ok false for
// items that carry no renderable turn (tool definitions, empty items). The same
// mapping serves both the request history and the final response's output items.
func itemMessage(it respItem) (Message, bool) {
	switch it.Type {
	case "message":
		text := joinParts(it.Content)
		if strings.TrimSpace(text) == "" {
			return Message{}, false
		}
		role := it.Role
		// developer is the Responses spelling of a system instruction; fold it to
		// system so the viewer renders it as the setup turn it is.
		if role == "developer" {
			role = "system"
		}
		return Message{Role: role, Content: []Block{textBlock(text)}}, true

	case "reasoning":
		text := joinParts(it.Summary)
		if strings.TrimSpace(text) == "" {
			return Message{}, false
		}
		return Message{Role: "assistant", Content: []Block{thinkingBlock(text)}}, true

	case "function_call", "custom_tool_call":
		args := it.Arguments
		if args == "" {
			args = it.Input
		}
		return Message{
			Role:    "assistant",
			Content: []Block{toolCallBlock(wireCall{ID: it.CallID, Name: it.Name, Args: args})},
		}, true

	case "function_call_output", "custom_tool_call_output":
		return Message{
			Role:    "tool",
			Content: []Block{toolResultBlock(it.CallID, rawToString(it.Output))},
		}, true

	default:
		// additional_tools, tool definitions, and any unknown item carry no turn.
		return Message{}, false
	}
}

// finalOutput pulls the output items from a Responses SSE stream. It prefers the
// per-item events: a stream emits one response.output_item.done per output item
// (the turn's reasoning, message, and function_call items), and codex's terminal
// response.completed frequently reports an EMPTY output array, so reading only
// the completed event silently drops the turn's final assistant message. The
// per-item events are collected in stream order; the completed event's output is
// used only as a fallback for a provider that populates it there instead. This
// gap was found by the native-session audit: a codex run whose 109 tool calls
// captured perfectly still lost its closing summary, because that summary arrived
// as an output_item.done while completed.output was empty.
func finalOutput(body []byte) []respItem {
	var items []respItem
	var completed []respItem
	for _, payload := range sseData(body) {
		var evt struct {
			Type     string    `json:"type"`
			Item     *respItem `json:"item"`
			Response struct {
				Output []respItem `json:"output"`
			} `json:"response"`
		}
		if json.Unmarshal([]byte(payload), &evt) != nil {
			continue
		}
		// A completed item arrives as its own done event; collect it in order.
		if evt.Type == "response.output_item.done" && evt.Item != nil {
			items = append(items, *evt.Item)
		}
		// Any event carrying a populated response.output is a completed-style
		// event; keep the last one as the fallback. This stays type-independent so
		// a stream that labels the event only in its SSE `event:` line, not in the
		// data payload, is still recognized.
		if len(evt.Response.Output) > 0 {
			completed = evt.Response.Output
		}
	}
	if len(items) > 0 {
		return items
	}
	return completed
}

// sseData returns the payloads of every `data:` line in an SSE stream, skipping
// blank lines and the terminal [DONE] sentinel.
func sseData(body []byte) []string {
	var out []string
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" || payload == "[DONE]" {
			continue
		}
		out = append(out, payload)
	}
	return out
}

func bridgeSeq(path string) int {
	base := filepath.Base(path)
	base = strings.TrimSuffix(base, ".resp")
	n, err := strconv.Atoi(base)
	if err != nil {
		return 0
	}
	return n
}

// joinParts concatenates the text of a Responses content or summary array.
func joinParts(parts []respPart) string {
	var b strings.Builder
	for _, p := range parts {
		if p.Text != "" {
			b.WriteString(p.Text)
		}
	}
	return b.String()
}

// rawToString renders a tool result output, given as a JSON string, an array of
// parts, or an object with an output/text field, into plain text.
func rawToString(raw json.RawMessage) string {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	switch raw[0] {
	case '"':
		var s string
		if json.Unmarshal(raw, &s) == nil {
			return s
		}
	case '[':
		var parts []respPart
		if json.Unmarshal(raw, &parts) == nil {
			return joinParts(parts)
		}
	case '{':
		var obj struct {
			Output string `json:"output"`
			Text   string `json:"text"`
		}
		if json.Unmarshal(raw, &obj) == nil {
			if obj.Output != "" {
				return obj.Output
			}
			if obj.Text != "" {
				return obj.Text
			}
		}
	}
	return string(raw)
}
