// Package trace encodes an agent run's captured conversation into the Hugging
// Face Hub's native agent-session schema, the JSONL shape the Data Studio viewer
// renders without a bespoke importer: a session record, a model_change record,
// then one message record per turn, each message a role plus an array of typed
// content blocks (text, thinking, toolCall, toolResult).
//
// There is one message model here, Message, and every capture format is parsed
// straight into it. A run is captured in one of two shapes: chat-completions
// (a requests.jsonl of full message arrays plus resp-N.txt response streams) or
// Responses/bridge (a bridgetrace of per-turn request captures plus teed .resp
// response streams). Both parse directly to []Message with redaction applied as
// each block is built, so no credential ever reaches a block, and the shared
// encoder serializes the same structure for either source.
package trace

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/tamnd/tomo-labs/pkg/result"
)

// Tokens is the run's token accounting, defined once in pkg/result. The session
// meta mirrors it into the published trace, so it aliases that single definition
// rather than restating it.
type Tokens = result.Tokens

// Session is the first record of a trace: the envelope the viewer keys on.
// Version 3 is the schema the Hub's native session importer expects. Meta
// carries run provenance for anyone querying the raw JSONL; the viewer ignores
// it and the reports read result.json instead.
type Session struct {
	Type      string `json:"type"` // always "session"
	Version   int    `json:"version"`
	ID        string `json:"id"`
	Timestamp string `json:"timestamp"`
	Cwd       string `json:"cwd"`
	Meta      *Meta  `json:"meta,omitempty"`
}

// Meta is the run provenance carried on the session record. It mirrors the
// fields result.json holds so the raw trace is self-describing, but nothing
// downstream depends on it.
type Meta struct {
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

// ModelChange records which model produced the turns that follow, the record
// the native importer reads to label the session's model.
type ModelChange struct {
	Type      string  `json:"type"` // always "model_change"
	ID        string  `json:"id"`
	ParentID  *string `json:"parentId"`
	Timestamp string  `json:"timestamp"`
	ModelID   string  `json:"modelId"`
}

// Record is one turn: a message with a top-level id, its parent in the record
// chain, a timestamp, and the message body. The parent chain is what lets the
// viewer thread the timeline in order.
type Record struct {
	Type      string  `json:"type"` // always "message"
	ID        string  `json:"id"`
	ParentID  string  `json:"parentId"`
	Timestamp string  `json:"timestamp"`
	Message   Message `json:"message"`
}

// Message is one conversation turn: a role and its content blocks. It is the
// single model every parser targets.
type Message struct {
	Role    string  `json:"role"`
	Content []Block `json:"content"`
}

// Block is one content block. Type selects which payload fields are meaningful:
// text carries Text, thinking carries Thinking, toolCall carries ID/Name/
// Arguments, toolResult carries ToolCallID/Output.
type Block struct {
	Type string `json:"type"`

	Text     string `json:"text,omitempty"`
	Thinking string `json:"thinking,omitempty"`

	// toolCall fields. Arguments is a parsed JSON object so the viewer renders
	// each argument as a field rather than a re-escaped string.
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Arguments json.RawMessage `json:"arguments,omitempty"`

	// toolResult fields.
	ToolCallID string `json:"toolCallId,omitempty"`
	Output     string `json:"output,omitempty"`
}

// Header is the metadata the caller supplies for a trace, taken from the run's
// Result. The publisher fills it from result.json.
type Header struct {
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

// Encode reconstructs a run's conversation from its trace directory and returns
// one JSONL file in the Hub's native session schema. A Responses/bridge capture
// is decoded when present; otherwise the chat-completions capture is. Both
// resolve to the same message list, redacted as it was built, and the same
// encoder.
func Encode(traceDir string, h Header) ([]byte, error) {
	return encodeSession(h, Reconstruct(traceDir, h.Model))
}

// Reconstruct returns a run's conversation as the message list both capture
// formats resolve to: the Responses/bridge capture when present, else the
// chat-completions capture. It is what Encode serializes, exposed on its own so
// the audit can tally our reconstruction against the agent's native session log
// without re-parsing the emitted JSONL.
func Reconstruct(traceDir, model string) []Message {
	if msgs, ok := tomoMessages(traceDir); ok {
		return msgs
	}
	if msgs, ok := responsesMessages(traceDir, model); ok {
		return msgs
	}
	return chatMessages(traceDir, model)
}

// encodeSession serializes a reconstructed message list into the Hub's native
// session schema. It is the shared tail of both capture paths, so they emit
// byte-identical structure.
func encodeSession(h Header, msgs []Message) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)

	base := baseTime(h.Timestamp)

	if err := enc.Encode(Session{
		Type:      "session",
		Version:   3,
		ID:        orUnknown(h.ID),
		Timestamp: stamp(base, 0),
		Cwd:       "/workspace",
		Meta:      headerMeta(h),
	}); err != nil {
		return nil, err
	}

	const modelID = "model-0"
	if err := enc.Encode(ModelChange{
		Type:      "model_change",
		ID:        modelID,
		ParentID:  nil,
		Timestamp: stamp(base, 0),
		ModelID:   orUnknown(h.Model),
	}); err != nil {
		return nil, err
	}

	parent := modelID
	for i, m := range msgs {
		id := fmt.Sprintf("msg-%d", i)
		if err := enc.Encode(Record{
			Type:      "message",
			ID:        id,
			ParentID:  parent,
			Timestamp: stamp(base, i+1),
			Message:   m,
		}); err != nil {
			return nil, err
		}
		parent = id
	}
	return buf.Bytes(), nil
}

// headerMeta builds the session's provenance object from the caller's header.
func headerMeta(h Header) *Meta {
	return &Meta{
		Harness:     h.Harness,
		Eval:        h.Eval,
		Scenario:    h.Scenario,
		Model:       h.Model,
		Passed:      h.Passed,
		Ungraded:    h.Ungraded,
		ExitCode:    h.ExitCode,
		Attempts:    h.Attempts,
		WallSeconds: h.WallSeconds,
		Tokens:      h.Tokens,
		CostUSD:     h.CostUSD,
		Stop:        h.Stop,
	}
}

// wireCall is one tool call as the wire carried it: an id, a name, and the
// argument JSON as a string. Arguments stay a string until the block is built
// so the string-based redaction runs before they are parsed into an object.
type wireCall struct {
	ID   string
	Name string
	Args string
}

// The block constructors are the single place redaction happens: every string
// that reaches a block passes through redactString first, so a public trace can
// never carry a captured credential regardless of which parser produced it.

func textBlock(s string) Block     { return Block{Type: "text", Text: redactString(s)} }
func thinkingBlock(s string) Block { return Block{Type: "thinking", Thinking: redactString(s)} }

func toolCallBlock(c wireCall) Block {
	return Block{Type: "toolCall", ID: c.ID, Name: c.Name, Arguments: argObject(redactString(c.Args))}
}

func toolResultBlock(callID, output string) Block {
	return Block{Type: "toolResult", ToolCallID: callID, Output: redactString(output)}
}

// assistantMessage assembles an assistant turn from its reasoning, its text, and
// its tool calls: a thinking block if any, a text block if any, then one
// toolCall block per call. A turn with no content still emits one empty text
// block so the message is never a contentless record.
func assistantMessage(text, reasoning string, calls []wireCall) Message {
	var blocks []Block
	if strings.TrimSpace(reasoning) != "" {
		blocks = append(blocks, thinkingBlock(reasoning))
	}
	if text != "" {
		blocks = append(blocks, textBlock(text))
	}
	for _, c := range calls {
		blocks = append(blocks, toolCallBlock(c))
	}
	if len(blocks) == 0 {
		blocks = []Block{textBlock("")}
	}
	return Message{Role: "assistant", Content: blocks}
}

// argObject renders a tool call's argument string as a JSON object for the
// viewer. When the string is a JSON object it passes through verbatim; anything
// else (empty, malformed, a bare scalar) is wrapped as {"raw": ...} so the
// emitted value is always a valid object the viewer can render.
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

// baseTime parses the run's recorded timestamp into a base the message
// timestamps count from. It accepts the labs stamp and RFC3339, and falls back
// to the epoch so a missing stamp still yields a valid, deterministic sequence
// rather than the forbidden wall clock.
func baseTime(ts string) time.Time {
	for _, layout := range []string{"20060102T150405Z", time.RFC3339, "2006-01-02T15:04:05.000Z"} {
		if t, err := time.Parse(layout, ts); err == nil {
			return t.UTC()
		}
	}
	return time.Unix(0, 0).UTC()
}

// stamp renders the timestamp for the nth record, one second per record so the
// timeline is monotonic and ordered without inventing a per-turn clock the trace
// never captured.
func stamp(base time.Time, n int) string {
	return base.Add(time.Duration(n) * time.Second).Format("2006-01-02T15:04:05.000Z")
}

func orUnknown(s string) string {
	if strings.TrimSpace(s) == "" {
		return "unknown"
	}
	return s
}
