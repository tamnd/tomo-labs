package publish

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Session Trace Simple Format (STS-Format) is the Hub's native agent-trace
// shape: a JSONL file whose first line is a session header and whose every
// following line is one message in an envelope. The viewer in Data Studio reads
// this directly and renders the timeline of prompts, assistant turns, tool
// calls, and results. tomo is a custom harness, so it emits the format itself
// rather than relying on the Hub's built-in support for Claude Code, Codex, and
// Pi. See spec 2110/01.

// stsHeader is the first line of an STS file. type and harness and id are the
// required fields; name is the optional title; the rest are extra metadata the
// viewer ignores and the reports generator reads, so one file is both a
// renderable trace and a queryable result row.
type stsHeader struct {
	Type    string `json:"type"` // always "session"
	Harness string `json:"harness"`
	ID      string `json:"id"`
	Name    string `json:"name,omitempty"`

	// Result metadata carried as extra header fields.
	Eval        string  `json:"eval,omitempty"`
	Scenario    string  `json:"scenario,omitempty"`
	Model       string  `json:"model,omitempty"`
	Passed      bool    `json:"passed"`
	Ungraded    bool    `json:"ungraded,omitempty"`
	ExitCode    int     `json:"exit_code"`
	Attempts    int     `json:"attempts,omitempty"`
	WallSeconds int     `json:"wall_seconds,omitempty"`
	Tokens      *Tokens `json:"tokens,omitempty"`
	CostUSD     float64 `json:"cost_usd,omitempty"`
	Stop        string  `json:"stop,omitempty"`
	Timestamp   string  `json:"timestamp,omitempty"`
}

// Tokens mirrors the labs token accounting so a header can carry it verbatim
// without importing pkg/lab (which would be a cycle once lab calls publish).
type Tokens struct {
	Prompt     int `json:"prompt"`
	Completion int `json:"completion"`
	Total      int `json:"total"`
	Cached     int `json:"cached,omitempty"`
	CacheWrite int `json:"cache_write,omitempty"`
	Reasoning  int `json:"reasoning,omitempty"`
}

// stsEnvelope wraps one message line.
type stsEnvelope struct {
	Type    string     `json:"type"` // always "message"
	Message stsMessage `json:"message"`
}

// stsMessage is the message shape the Hub interprets. Only these fields are
// read by the viewer; toolCalls and toolCallId are what let it stitch a tool
// result next to the call that produced it.
type stsMessage struct {
	Role             string        `json:"role"`
	Content          string        `json:"content"`
	ReasoningContent string        `json:"reasoningContent,omitempty"`
	ToolCalls        []stsToolCall `json:"toolCalls,omitempty"`
	ToolCallID       string        `json:"toolCallId,omitempty"`
	Model            string        `json:"model,omitempty"`
}

type stsToolCall struct {
	ID       string      `json:"id"`
	Function stsToolFunc `json:"function"`
}

type stsToolFunc struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // a JSON string, per the STS spec
}

// SessionMeta is the header metadata the caller supplies for a trace, taken
// from the run's Result. The publisher fills it from result.json.
type SessionMeta struct {
	Harness     string
	ID          string
	Name        string
	Eval        string
	Scenario    string
	Model       string
	Passed      bool
	Ungraded    bool
	ExitCode    int
	Attempts    int
	WallSeconds int
	Tokens      *Tokens
	CostUSD     float64
	Stop        string
	Timestamp   string
}

// EncodeTrace reconstructs a run's conversation from its trace directory and
// returns one STS-Format JSONL file. The conversation is built from the richest
// request's message history (which the agent rebuilds verbatim each turn, so it
// holds every assistant turn and tool result up to the last) plus the final
// assistant reply decoded from the last response stream, which is the one turn
// no request history carries yet. Redaction is applied to every message on the
// way out, so a public trace never carries a captured credential.
func EncodeTrace(traceDir string, meta SessionMeta) ([]byte, error) {
	history := richestHistory(filepath.Join(traceDir, "requests.jsonl"))
	msgs := make([]stsMessage, 0, len(history)+1)
	for _, m := range history {
		msgs = append(msgs, transliterate(m, meta.Model))
	}
	if final, ok := finalAssistant(traceDir, meta.Model); ok {
		msgs = append(msgs, final)
	}

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)

	hdr := stsHeader{
		Type:        "session",
		Harness:     meta.Harness,
		ID:          meta.ID,
		Name:        meta.Name,
		Eval:        meta.Eval,
		Scenario:    meta.Scenario,
		Model:       meta.Model,
		Passed:      meta.Passed,
		Ungraded:    meta.Ungraded,
		ExitCode:    meta.ExitCode,
		Attempts:    meta.Attempts,
		WallSeconds: meta.WallSeconds,
		Tokens:      meta.Tokens,
		CostUSD:     meta.CostUSD,
		Stop:        meta.Stop,
		Timestamp:   meta.Timestamp,
	}
	if err := enc.Encode(hdr); err != nil {
		return nil, err
	}
	for _, m := range msgs {
		redactMessage(&m)
		if err := enc.Encode(stsEnvelope{Type: "message", Message: m}); err != nil {
			return nil, err
		}
	}
	return buf.Bytes(), nil
}

// oaiMessage is an OpenAI chat message as it appears in a captured request body.
// Content is a raw value because the API allows a plain string or an array of
// content parts; contentString coerces both to text.
type oaiMessage struct {
	Role             string          `json:"role"`
	Content          json.RawMessage `json:"content"`
	ToolCalls        []oaiToolCall   `json:"tool_calls"`
	ToolCallID       string          `json:"tool_call_id"`
	ReasoningContent string          `json:"reasoning_content"`
	Reasoning        string          `json:"reasoning"`
}

type oaiToolCall struct {
	ID       string `json:"id"`
	Index    int    `json:"index"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// richestHistory returns the message array of the request that carried the most
// messages, which is the fullest conversation the agent assembled.
func richestHistory(reqsPath string) []oaiMessage {
	f, err := os.Open(reqsPath)
	if err != nil {
		return nil
	}
	defer f.Close()

	var best []oaiMessage
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 32*1024*1024)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var rec struct {
			Path string          `json:"path"`
			Body json.RawMessage `json:"body"`
		}
		if json.Unmarshal(line, &rec) != nil || len(rec.Body) == 0 {
			continue
		}
		var body struct {
			Messages []oaiMessage `json:"messages"`
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

// transliterate maps one OpenAI message to an STS message. The mapping is a
// field-name rename, not a translation, because the two shapes agree on
// structure: tool_calls to toolCalls, tool_call_id to toolCallId, reasoning to
// reasoningContent.
func transliterate(m oaiMessage, model string) stsMessage {
	out := stsMessage{
		Role:       m.Role,
		Content:    contentString(m.Content),
		ToolCallID: m.ToolCallID,
		Model:      model,
	}
	if m.ReasoningContent != "" {
		out.ReasoningContent = m.ReasoningContent
	} else if m.Reasoning != "" {
		out.ReasoningContent = m.Reasoning
	}
	for _, tc := range m.ToolCalls {
		out.ToolCalls = append(out.ToolCalls, stsToolCall{
			ID:       tc.ID,
			Function: stsToolFunc{Name: tc.Function.Name, Arguments: tc.Function.Arguments},
		})
	}
	return out
}

// contentString coerces an OpenAI content value (a string or an array of parts)
// to text. An array joins the text of its text parts, which is what the viewer
// shows; non-text parts (images) are noted rather than dropped silently.
func contentString(raw json.RawMessage) string {
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

// finalAssistant decodes the last response stream into the final assistant
// message. It returns ok false when the last response is not a parseable
// assistant reply (an HTML or JSON error page from the upstream), in which case
// the trace ends at the request history, which is still a faithful record of
// everything but the turn that never completed.
func finalAssistant(traceDir, model string) (stsMessage, bool) {
	path, ok := lastRespFile(traceDir)
	if !ok {
		return stsMessage{}, false
	}
	body := gunzipTolerant(path)
	content, reasoning, calls := parseSSE(body)
	if content == "" && reasoning == "" && len(calls) == 0 {
		return stsMessage{}, false
	}
	return stsMessage{
		Role:             "assistant",
		Content:          content,
		ReasoningContent: reasoning,
		ToolCalls:        calls,
		Model:            model,
	}, true
}

// lastRespFile returns the highest-numbered resp-N.txt in a trace dir.
func lastRespFile(traceDir string) (string, bool) {
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
// possibly truncated response body. The proxy tees the raw upstream body, which
// is gzip when the upstream set Content-Encoding and is often cut off at the end
// when the client died mid-stream, so a partial decode is expected and its
// prefix is kept rather than discarded.
func gunzipTolerant(path string) []byte {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	if len(raw) < 2 || raw[0] != 0x1f || raw[1] != 0x8b {
		return raw // not gzip; the proxy stored it plain
	}
	zr, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		return raw
	}
	zr.Multistream(true)
	out, _ := io.ReadAll(zr) // ReadAll on a truncated stream returns the prefix plus an error
	return out
}

// parseSSE accumulates an SSE completion stream into the final assistant reply:
// its content, its reasoning, and its tool calls. Deltas are concatenated;
// tool-call fragments are keyed by index so a name and its streamed argument
// chunks reassemble into one call.
func parseSSE(body []byte) (content, reasoning string, calls []stsToolCall) {
	var cb, rb strings.Builder
	byIndex := map[int]*stsToolCall{}
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
					Content   string        `json:"content"`
					Reasoning string        `json:"reasoning"`
					ToolCalls []oaiToolCall `json:"tool_calls"`
				} `json:"delta"`
			} `json:"choices"`
		}
		if json.Unmarshal([]byte(payload), &chunk) != nil {
			continue
		}
		for _, ch := range chunk.Choices {
			cb.WriteString(ch.Delta.Content)
			rb.WriteString(ch.Delta.Reasoning)
			for _, tc := range ch.Delta.ToolCalls {
				cur, ok := byIndex[tc.Index]
				if !ok {
					cur = &stsToolCall{}
					byIndex[tc.Index] = cur
					order = append(order, tc.Index)
				}
				if tc.ID != "" {
					cur.ID = tc.ID
				}
				if tc.Function.Name != "" {
					cur.Function.Name = tc.Function.Name
				}
				cur.Function.Arguments += tc.Function.Arguments
			}
		}
	}
	for _, idx := range order {
		calls = append(calls, *byIndex[idx])
	}
	return cb.String(), rb.String(), calls
}

// TracePath returns the repo path for a run's STS file, the coordinate layout
// data/<eval>/<scenario>/<model>/<tool>-<id>.jsonl. Slashes and spaces in any
// component are flattened so the path is always a single valid segment tree.
func TracePath(eval, scenario, model, tool, id string) string {
	return fmt.Sprintf("data/%s/%s/%s/%s-%s.jsonl",
		slug(eval), slug(scenario), slug(model), slug(tool), slug(id))
}

func slug(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "unknown"
	}
	repl := func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			return r
		case r == '-', r == '_', r == '.':
			return r
		default:
			return '-'
		}
	}
	return strings.Map(repl, s)
}
