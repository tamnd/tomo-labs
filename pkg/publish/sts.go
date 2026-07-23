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
	"time"
)

// A trace is a JSONL file in the Hub's native agent-session schema, the same
// shape the Data Studio viewer renders for Pi and opencode sessions: a session
// record, a model_change record, then one message record per turn. Each message
// carries a top-level id, parentId, and timestamp, and its content is an array
// of typed blocks (text, thinking, toolCall, toolResult). tomo is a custom
// harness, so it emits this schema itself rather than relying on the Hub's
// built-in importers. See spec 2110/01. The result metadata the reports need
// lives in result.json, not here, so the trace stays a clean conversation
// record the viewer recognizes without bespoke fields to trip its inference.

// piSession is the first line of a trace: the session envelope the viewer keys
// on. version 3 is the schema the Hub's native session importer expects. The
// meta object carries run provenance for anyone querying the raw JSONL; the
// viewer ignores it and the reports read result.json instead.
type piSession struct {
	Type      string       `json:"type"` // always "session"
	Version   int          `json:"version"`
	ID        string       `json:"id"`
	Timestamp string       `json:"timestamp"`
	Cwd       string       `json:"cwd"`
	Meta      *sessionMeta `json:"meta,omitempty"`
}

// sessionMeta is the run provenance carried on the session record. It mirrors
// the fields result.json holds so the raw trace is self-describing, but nothing
// downstream depends on it: the reports read result.json directly.
type sessionMeta struct {
	Harness     string  `json:"harness,omitempty"`
	Eval        string  `json:"eval,omitempty"`
	Scenario    string  `json:"scenario,omitempty"`
	Model       string  `json:"model,omitempty"`
	Passed      bool    `json:"passed"`
	Ungraded    bool    `json:"ungraded,omitempty"`
	ExitCode    int     `json:"exitCode"`
	Attempts    int     `json:"attempts,omitempty"`
	WallSeconds int     `json:"wallSeconds,omitempty"`
	Tokens      *Tokens `json:"tokens,omitempty"`
	CostUSD     float64 `json:"costUsd,omitempty"`
	Stop        string  `json:"stop,omitempty"`
}

// piModelChange records which model produced the turns that follow, the same
// record the native importer reads to label the session's model.
type piModelChange struct {
	Type      string  `json:"type"` // always "model_change"
	ID        string  `json:"id"`
	ParentID  *string `json:"parentId"`
	Timestamp string  `json:"timestamp"`
	ModelID   string  `json:"modelId"`
}

// piMessageRec is one turn: a message with a top-level id, its parent in the
// record chain, a timestamp, and the message body. The parent chain is what
// lets the viewer thread the timeline in order.
type piMessageRec struct {
	Type      string    `json:"type"` // always "message"
	ID        string    `json:"id"`
	ParentID  string    `json:"parentId"`
	Timestamp string    `json:"timestamp"`
	Message   piMessage `json:"message"`
}

// piMessage is a role plus an array of typed content blocks, the shape the
// viewer renders block by block.
type piMessage struct {
	Role    string    `json:"role"`
	Content []piBlock `json:"content"`
}

// piBlock is one content block. The type field selects which of the payload
// fields is meaningful: text carries Text, thinking carries Thinking, toolCall
// carries ID/Name/Arguments, toolResult carries ToolCallID/Output.
type piBlock struct {
	Type string `json:"type"`

	Text     string `json:"text,omitempty"`
	Thinking string `json:"thinking,omitempty"`

	// toolCall fields. Arguments is a parsed JSON object so the viewer can render
	// each argument as a field rather than a re-escaped string.
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Arguments json.RawMessage `json:"arguments,omitempty"`

	// toolResult fields.
	ToolCallID string `json:"toolCallId,omitempty"`
	Output     string `json:"output,omitempty"`
}

// Tokens mirrors the labs token accounting so a session's meta can carry it
// verbatim without importing pkg/lab (which would be a cycle once lab calls
// publish).
type Tokens struct {
	Prompt     int `json:"prompt"`
	Completion int `json:"completion"`
	Total      int `json:"total"`
	Cached     int `json:"cached,omitempty"`
	CacheWrite int `json:"cache_write,omitempty"`
	Reasoning  int `json:"reasoning,omitempty"`
}

// stsMessage is the intermediate shape a captured turn parses into before it is
// redacted and emitted as a piMessageRec. It keeps tool-call arguments as the
// raw JSON string the wire carried, so redaction (which is string-based) runs
// before the string is parsed into an object.
type stsMessage struct {
	Role             string
	Content          string
	ReasoningContent string
	ToolCalls        []stsToolCall
	ToolCallID       string
	Model            string
}

type stsToolCall struct {
	ID       string
	Function stsToolFunc
}

type stsToolFunc struct {
	Name      string
	Arguments string // a JSON string as the wire carried it
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
// returns one JSONL file in the Hub's native session schema. The conversation
// is built from the richest request's message history (which the agent rebuilds
// verbatim each turn, so it holds every assistant turn and tool result up to the
// last) plus the final assistant reply decoded from the last response stream,
// which is the one turn no request history carries yet. Redaction is applied to
// every message before it is emitted, so a public trace never carries a
// captured credential; tool-call arguments are redacted as strings and only
// then parsed into objects.
func EncodeTrace(traceDir string, meta SessionMeta) ([]byte, error) {
	// A swelive container run records its conversation in a bridgetrace directory
	// of Responses-API request captures, not a chat-completions requests.jsonl, so
	// decode that shape when it is present and fall through to the chat shape
	// otherwise. Both paths converge on the same message list and emitter.
	if msgs, ok := bridgeHistory(traceDir, meta.Model); ok {
		return encodeSession(meta, msgs)
	}

	history := richestHistory(filepath.Join(traceDir, "requests.jsonl"))
	msgs := make([]stsMessage, 0, len(history)+1)
	for _, m := range history {
		msgs = append(msgs, transliterate(m, meta.Model))
	}
	if final, ok := finalAssistant(traceDir, meta.Model); ok {
		msgs = append(msgs, final)
	}
	return encodeSession(meta, msgs)
}

// encodeSession redacts the reconstructed messages and serializes them into the
// Hub's native session schema: a session record, a model_change record, then one
// message record per turn. It is the shared tail of every trace encoder, so the
// chat-completions and bridgetrace paths emit byte-identical structure.
func encodeSession(meta SessionMeta, msgs []stsMessage) ([]byte, error) {
	for i := range msgs {
		redactMessage(&msgs[i])
	}

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)

	base := baseTime(meta.Timestamp)
	sessionID := orUnknown(meta.ID)

	if err := enc.Encode(piSession{
		Type:      "session",
		Version:   3,
		ID:        sessionID,
		Timestamp: stamp(base, 0),
		Cwd:       "/workspace",
		Meta:      metaBlock(meta),
	}); err != nil {
		return nil, err
	}

	const modelID = "model-0"
	if err := enc.Encode(piModelChange{
		Type:      "model_change",
		ID:        modelID,
		ParentID:  nil,
		Timestamp: stamp(base, 0),
		ModelID:   orUnknown(meta.Model),
	}); err != nil {
		return nil, err
	}

	parent := modelID
	for i, m := range msgs {
		id := fmt.Sprintf("msg-%d", i)
		if err := enc.Encode(piMessageRec{
			Type:      "message",
			ID:        id,
			ParentID:  parent,
			Timestamp: stamp(base, i+1),
			Message:   piMessage{Role: m.Role, Content: blocksFor(m)},
		}); err != nil {
			return nil, err
		}
		parent = id
	}
	return buf.Bytes(), nil
}

// blocksFor turns one intermediate message into the viewer's content-block
// array. A tool result is a single toolResult block. An assistant or user turn
// is its reasoning (if any) as a thinking block, its text as a text block, then
// one toolCall block per call. A turn with no content at all still emits one
// empty text block so the message is never a contentless record.
func blocksFor(m stsMessage) []piBlock {
	if m.Role == "tool" {
		return []piBlock{{Type: "toolResult", ToolCallID: m.ToolCallID, Output: m.Content}}
	}
	var blocks []piBlock
	if m.ReasoningContent != "" {
		blocks = append(blocks, piBlock{Type: "thinking", Thinking: m.ReasoningContent})
	}
	if m.Content != "" {
		blocks = append(blocks, piBlock{Type: "text", Text: m.Content})
	}
	for _, tc := range m.ToolCalls {
		blocks = append(blocks, piBlock{
			Type:      "toolCall",
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Arguments: argObject(tc.Function.Arguments),
		})
	}
	if len(blocks) == 0 {
		blocks = append(blocks, piBlock{Type: "text", Text: ""})
	}
	return blocks
}

// argObject renders a tool call's argument string as a JSON object for the
// viewer. The wire carries arguments as a JSON string; when that string is a
// JSON object it is passed through verbatim, and when it is anything else (an
// empty string, malformed JSON, a bare scalar) it is wrapped as {"raw": ...} so
// the emitted value is always a valid JSON object the viewer can render.
func argObject(s string) json.RawMessage {
	s = strings.TrimSpace(s)
	if s == "" {
		return json.RawMessage(`{}`)
	}
	if json.Valid([]byte(s)) {
		var v any
		if json.Unmarshal([]byte(s), &v) == nil {
			if _, ok := v.(map[string]any); ok {
				return json.RawMessage(s)
			}
		}
	}
	b, _ := json.Marshal(map[string]string{"raw": s})
	return json.RawMessage(b)
}

// metaBlock builds the session's provenance object, or nil when the run carried
// nothing worth recording.
func metaBlock(meta SessionMeta) *sessionMeta {
	sm := sessionMeta{
		Harness:     meta.Harness,
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
	}
	return &sm
}

// baseTime parses the run's recorded timestamp into a base the message
// timestamps count from. It accepts the labs stamp (20060102T150405Z) and
// RFC3339, and falls back to the epoch so a missing stamp still yields a valid,
// deterministic sequence rather than the forbidden wall clock.
func baseTime(ts string) time.Time {
	for _, layout := range []string{"20060102T150405Z", time.RFC3339, "2006-01-02T15:04:05.000Z"} {
		if t, err := time.Parse(layout, ts); err == nil {
			return t.UTC()
		}
	}
	return time.Unix(0, 0).UTC()
}

// stamp renders the timestamp for the nth record, one second per record so the
// timeline is monotonic and ordered without inventing a real per-turn clock the
// trace never captured.
func stamp(base time.Time, n int) string {
	return base.Add(time.Duration(n) * time.Second).Format("2006-01-02T15:04:05.000Z")
}

func orUnknown(s string) string {
	if strings.TrimSpace(s) == "" {
		return "unknown"
	}
	return s
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
