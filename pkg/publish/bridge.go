package publish

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// A swelive container run does not record its conversation as a
// chat-completions requests.jsonl. It records each upstream call as a
// Responses-API request capture under <traceDir>/bridgetrace/NNNN.req.json,
// where each file holds an `instructions` system prompt and an `input` array of
// typed items: message, reasoning, function_call, function_call_output, and the
// codex custom_tool_call variants. No response body is teed, so the final
// assistant turn after the last request is not captured; the richest request's
// input already holds every turn up to it, which is the faithful conversation
// minus that one trailing reply. This file reconstructs the message list from
// that richest capture so the same redaction and emitter the chat path uses
// produce the trace.

// respItem is one item of a Responses-API `input` array, decoded as the union
// of every shape a bridgetrace carries. The Type field selects which of the
// other fields is meaningful.
type respItem struct {
	Type string `json:"type"`
	Role string `json:"role"`

	// message: content is an array of parts, each a text carrier under one of the
	// Responses part types (input_text, output_text, text).
	Content []respPart `json:"content"`

	// reasoning: a summary array of text parts.
	Summary []respPart `json:"summary"`

	// function_call and custom_tool_call: the call's name, its arguments (a JSON
	// string) or its input (the codex custom-tool payload), and the id that a
	// following output item refers back to.
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
	Input     string `json:"input"`
	CallID    string `json:"call_id"`
	ID        string `json:"id"`

	// function_call_output and custom_tool_call_output: the tool result, which is
	// a string or an array of parts depending on the tool.
	Output json.RawMessage `json:"output"`
}

// respPart is one part of a Responses content or summary array. A part is a
// text carrier; the several Responses type names (input_text, output_text,
// summary_text, text) all put their text in the same field.
type respPart struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// bridgeReq is the subset of a bridgetrace request capture this decoder reads.
type bridgeReq struct {
	Instructions string     `json:"instructions"`
	Input        []respItem `json:"input"`
}

// bridgeHistory reconstructs the message list from a bridgetrace directory, or
// reports ok false when the directory is not a bridgetrace. It reads the request
// whose input array is longest, which is the fullest conversation the agent
// assembled, and maps its instructions and input items to the same stsMessage
// list the chat path produces.
func bridgeHistory(traceDir, model string) ([]stsMessage, bool) {
	dir := filepath.Join(traceDir, "bridgetrace")
	files, _ := filepath.Glob(filepath.Join(dir, "*.req.json"))
	if len(files) == 0 {
		return nil, false
	}

	var best bridgeReq
	bestLen := -1
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var req bridgeReq
		if json.Unmarshal(raw, &req) != nil {
			continue
		}
		if len(req.Input) > bestLen {
			bestLen = len(req.Input)
			best = req
		}
	}
	if bestLen < 0 {
		return nil, false
	}

	msgs := make([]stsMessage, 0, len(best.Input)+1)
	if strings.TrimSpace(best.Instructions) != "" {
		msgs = append(msgs, stsMessage{Role: "system", Content: best.Instructions, Model: model})
	}
	pending := map[string]string{} // call_id -> tool name, to label a result's origin
	for _, it := range best.Input {
		if m, ok := bridgeItem(it, model, pending); ok {
			msgs = append(msgs, m)
		}
	}
	return msgs, true
}

// bridgeItem maps one Responses input item to an stsMessage, or reports ok false
// for items that carry no renderable turn (tool definitions, empty items). The
// pending map carries a tool call's name forward to its output item so a result
// can be attributed, though the emitter keys the stitch on the call id.
func bridgeItem(it respItem, model string, pending map[string]string) (stsMessage, bool) {
	switch it.Type {
	case "message":
		text := joinParts(it.Content)
		if strings.TrimSpace(text) == "" {
			return stsMessage{}, false
		}
		role := it.Role
		// A developer role is the Responses spelling of a system instruction; fold
		// it to system so the viewer renders it as the setup turn it is.
		if role == "developer" {
			role = "system"
		}
		return stsMessage{Role: role, Content: text, Model: model}, true

	case "reasoning":
		text := joinParts(it.Summary)
		if strings.TrimSpace(text) == "" {
			return stsMessage{}, false
		}
		return stsMessage{Role: "assistant", ReasoningContent: text, Model: model}, true

	case "function_call", "custom_tool_call":
		args := it.Arguments
		if args == "" {
			args = it.Input
		}
		if it.CallID != "" {
			pending[it.CallID] = it.Name
		}
		return stsMessage{
			Role:  "assistant",
			Model: model,
			ToolCalls: []stsToolCall{{
				ID:       it.CallID,
				Function: stsToolFunc{Name: it.Name, Arguments: args},
			}},
		}, true

	case "function_call_output", "custom_tool_call_output":
		return stsMessage{
			Role:       "tool",
			Content:    rawToString(it.Output),
			ToolCallID: it.CallID,
			Model:      model,
		}, true

	default:
		// additional_tools, tool definitions, and any unknown item carry no turn.
		return stsMessage{}, false
	}
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

// rawToString renders a function_call_output `output`, which the Responses API
// gives as either a JSON string or an array of parts, into plain text.
func rawToString(raw json.RawMessage) string {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	if raw[0] == '"' {
		var s string
		if json.Unmarshal(raw, &s) == nil {
			return s
		}
	}
	if raw[0] == '[' {
		var parts []respPart
		if json.Unmarshal(raw, &parts) == nil {
			return joinParts(parts)
		}
	}
	if raw[0] == '{' {
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
