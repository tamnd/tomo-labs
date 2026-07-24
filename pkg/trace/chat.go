package trace

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// The chat-completions capture is a requests.jsonl, one line per upstream call
// carrying the full messages array the agent assembled, plus a resp-N.txt per
// call holding that call's SSE response stream. The richest request (the longest
// messages array) is the fullest conversation minus the final reply, which the
// last response stream carries and no request echoes yet.

// chatMessages reconstructs the message list from a chat-completions capture:
// the richest request's history followed by the final assistant reply decoded
// from the last response stream.
func chatMessages(traceDir, model string) []Message {
	history := richestChat(filepath.Join(traceDir, "requests.jsonl"))
	msgs := make([]Message, 0, len(history)+1)
	for _, m := range history {
		msgs = append(msgs, chatMessage(m))
	}
	if final, ok := chatFinal(traceDir); ok {
		msgs = append(msgs, final)
	}
	return msgs
}

// chatRequest is one OpenAI chat message from a captured request body. Content
// is raw because the API allows a plain string or an array of parts.
type chatRequest struct {
	Role             string          `json:"role"`
	Content          json.RawMessage `json:"content"`
	ToolCalls        []chatToolCall  `json:"tool_calls"`
	ToolCallID       string          `json:"tool_call_id"`
	ReasoningContent string          `json:"reasoning_content"`
	Reasoning        string          `json:"reasoning"`
}

type chatToolCall struct {
	ID       string `json:"id"`
	Index    int    `json:"index"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// chatMessage maps one captured chat message straight to a Message. A tool
// message is a single toolResult block; any other role is its reasoning, its
// text, and its tool calls.
func chatMessage(m chatRequest) Message {
	if m.Role == "tool" {
		return Message{Role: "tool", Content: []Block{toolResultBlock(m.ToolCallID, chatContent(m.Content))}}
	}
	reasoning := m.ReasoningContent
	if reasoning == "" {
		reasoning = m.Reasoning
	}
	msg := assistantMessage(chatContent(m.Content), reasoning, chatCalls(m.ToolCalls))
	msg.Role = m.Role
	return msg
}

func chatCalls(tcs []chatToolCall) []wireCall {
	out := make([]wireCall, 0, len(tcs))
	for _, tc := range tcs {
		out = append(out, wireCall{ID: tc.ID, Name: tc.Function.Name, Args: tc.Function.Arguments})
	}
	return out
}

// richestChat returns the messages array of the request that carried the most
// messages, the fullest conversation the agent assembled.
func richestChat(reqsPath string) []chatRequest {
	f, err := os.Open(reqsPath)
	if err != nil {
		return nil
	}
	defer f.Close()

	var best []chatRequest
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 32*1024*1024)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var rec struct {
			Body json.RawMessage `json:"body"`
		}
		if json.Unmarshal(line, &rec) != nil || len(rec.Body) == 0 {
			continue
		}
		var body struct {
			Messages []chatRequest `json:"messages"`
		}
		if json.Unmarshal(rec.Body, &body) != nil {
			continue
		}
		if len(body.Messages) > len(best) {
			best = body.Messages
		}
	}
	return best
}

// chatContent coerces an OpenAI content value (a string or an array of parts) to
// text. An array joins its text parts; a non-text part (an image) is noted
// rather than dropped silently.
func chatContent(raw json.RawMessage) string {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	if raw[0] == '"' {
		var s string
		if json.Unmarshal(raw, &s) == nil {
			return s
		}
		return ""
	}
	if raw[0] == '[' {
		var parts []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}
		if json.Unmarshal(raw, &parts) == nil {
			var b strings.Builder
			for _, p := range parts {
				switch {
				case p.Text != "":
					b.WriteString(p.Text)
				case p.Type != "" && p.Type != "text":
					b.WriteString("[" + p.Type + "]")
				}
			}
			return b.String()
		}
	}
	return string(raw)
}

// chatFinal decodes the last response stream into the final assistant message,
// reporting ok false when the last response is not a parseable reply (an HTML or
// JSON error page), in which case the trace ends at the request history.
func chatFinal(traceDir string) (Message, bool) {
	path, ok := lastResp(traceDir)
	if !ok {
		return Message{}, false
	}
	text, reasoning, calls := parseChatSSE(gunzipTolerant(path))
	if text == "" && reasoning == "" && len(calls) == 0 {
		return Message{}, false
	}
	return assistantMessage(text, reasoning, calls), true
}

// lastResp returns the highest-numbered resp-N.txt in a trace dir.
func lastResp(traceDir string) (string, bool) {
	matches, _ := filepath.Glob(filepath.Join(traceDir, "resp-*.txt"))
	if len(matches) == 0 {
		return "", false
	}
	sort.Slice(matches, func(i, j int) bool { return respN(matches[i]) < respN(matches[j]) })
	return matches[len(matches)-1], true
}

func respN(path string) int {
	base := filepath.Base(path)
	base = strings.TrimSuffix(strings.TrimPrefix(base, "resp-"), ".txt")
	n, err := strconv.Atoi(base)
	if err != nil {
		return 0
	}
	return n
}

// gunzipTolerant returns the decoded bytes of a possibly gzip-compressed,
// possibly truncated response body: the proxy tees the raw upstream body, which
// is gzip when the upstream set Content-Encoding and is often cut off when the
// client died mid-stream, so a partial decode keeps its prefix.
func gunzipTolerant(path string) []byte {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	if len(raw) < 2 || raw[0] != 0x1f || raw[1] != 0x8b {
		return raw // not gzip; stored plain
	}
	zr, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		return raw
	}
	zr.Multistream(true)
	out, _ := io.ReadAll(zr) // a truncated stream returns the prefix plus an error
	return out
}

// parseChatSSE accumulates a chat-completions SSE stream into the final reply:
// its text, its reasoning, and its tool calls. Deltas concatenate; tool-call
// fragments key by index so a name and its streamed argument chunks reassemble.
func parseChatSSE(body []byte) (text, reasoning string, calls []wireCall) {
	var tb, rb strings.Builder
	byIndex := map[int]*wireCall{}
	var order []int

	sc := bufio.NewScanner(bytes.NewReader(body))
	sc.Buffer(make([]byte, 1024*1024), 32*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" || payload == "[DONE]" {
			continue
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content   string         `json:"content"`
					Reasoning string         `json:"reasoning"`
					ToolCalls []chatToolCall `json:"tool_calls"`
				} `json:"delta"`
			} `json:"choices"`
		}
		if json.Unmarshal([]byte(payload), &chunk) != nil {
			continue
		}
		for _, ch := range chunk.Choices {
			tb.WriteString(ch.Delta.Content)
			rb.WriteString(ch.Delta.Reasoning)
			for _, tc := range ch.Delta.ToolCalls {
				cur, ok := byIndex[tc.Index]
				if !ok {
					cur = &wireCall{}
					byIndex[tc.Index] = cur
					order = append(order, tc.Index)
				}
				if tc.ID != "" {
					cur.ID = tc.ID
				}
				if tc.Function.Name != "" {
					cur.Name = tc.Function.Name
				}
				cur.Args += tc.Function.Arguments
			}
		}
	}
	for _, idx := range order {
		calls = append(calls, *byIndex[idx])
	}
	return tb.String(), rb.String(), calls
}
