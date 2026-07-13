// Package codex reads the session rollouts the Codex CLI writes to disk and
// turns them into typed Go values the lab can study.
//
// Codex records every session as a JSONL file under
// ~/.codex/sessions/YYYY/MM/DD/rollout-<ts>-<uuid>.jsonl. Each line is one
// record with a timestamp, a type, and a payload. The record types are:
//
//   - session_meta: written once at the top, carries the session id, the
//     working directory, the CLI version, the model provider, and the base
//     instructions (the system prompt Codex ran with).
//   - turn_context: written once per turn, carries the model and reasoning
//     effort that turn ran with, the approval and sandbox policy, and the cwd.
//   - response_item: the model's own output stream: assistant and developer
//     messages, reasoning, and tool calls with their outputs.
//   - event_msg: lifecycle and accounting: task_started, the user message,
//     agent messages, token_count (full usage), patch applies, task_complete,
//     and turn_aborted.
//
// The point of parsing these is to learn from real gpt-5.x runs: what the
// strongest local tool actually does turn by turn, which model and effort it
// chose, how many tokens it spent, and how it converged, so tomo can be
// measured against that shape rather than against a guess.
package codex

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"
)

// Rollout is one Codex session, parsed from a rollout JSONL file. The typed
// slices keep the order the records appeared in, so a turn's model, its tool
// calls, and its token count can be read back in sequence.
type Rollout struct {
	Path   string         // the file this was read from, empty when read from a reader
	Meta   SessionMeta    // the one session_meta record
	Turns  []TurnContext  // one per turn_context, in order
	Items  []ResponseItem // the model output stream, in order
	Events []Event        // lifecycle and token accounting, in order
}

// SessionMeta is the session_meta payload: the identity of the run and the
// system prompt it started from.
type SessionMeta struct {
	SessionID     string    `json:"session_id"`
	Timestamp     time.Time `json:"-"`
	Cwd           string    `json:"cwd"`
	Originator    string    `json:"originator"`
	CLIVersion    string    `json:"cli_version"`
	Source        string    `json:"source"`
	ModelProvider string    `json:"model_provider"`
	Instructions  string    `json:"-"` // base_instructions.text, the system prompt
}

// TurnContext is the turn_context payload: the model and effort a single turn
// ran with, plus the policy it ran under. Model and Effort are the fields the
// lab most cares about, since they say which gpt-5.x variant produced the turn.
type TurnContext struct {
	Timestamp      time.Time `json:"-"`
	TurnID         string    `json:"turn_id"`
	Cwd            string    `json:"cwd"`
	CurrentDate    string    `json:"current_date"`
	Timezone       string    `json:"timezone"`
	ApprovalPolicy string    `json:"approval_policy"`
	Model          string    `json:"model"`
	Effort         string    `json:"effort"`
	Summary        string    `json:"summary"`
}

// ResponseItem is one record in the model output stream. It is a union: a
// message, a reasoning block, a tool call, or a tool call's output. The unused
// fields for a given Type stay zero.
type ResponseItem struct {
	Timestamp time.Time
	Type      string // message, reasoning, function_call, function_call_output, custom_tool_call, custom_tool_call_output
	TurnID    string // from the passthrough metadata, ties the item to its turn

	// message
	Role    string
	Content json.RawMessage

	// reasoning: the content is encrypted, so only note that it was present
	Reasoning bool

	// function_call and custom_tool_call
	ID        string
	CallID    string
	Name      string // the tool name, e.g. exec_command or apply_patch
	Arguments string // function_call arguments, a JSON string
	Input     string // custom_tool_call input, e.g. an apply_patch body
	Status    string

	// function_call_output and custom_tool_call_output
	Output string
}

// IsWrite reports whether the item is a tool call that changes files, so the
// lab can count edits the same way tomo's governor does. Codex writes through
// apply_patch, so a custom_tool_call by that name is a write.
func (it ResponseItem) IsWrite() bool {
	return (it.Type == "custom_tool_call" || it.Type == "function_call") && it.Name == "apply_patch"
}

// IsToolCall reports whether the item is any tool call, write or not.
func (it ResponseItem) IsToolCall() bool {
	return it.Type == "function_call" || it.Type == "custom_tool_call"
}

// Event is one event_msg record: a lifecycle marker or a token count. Like
// ResponseItem it is a union keyed by Type, and unused fields stay zero.
type Event struct {
	Timestamp time.Time
	Type      string // task_started, user_message, agent_message, token_count, patch_apply_end, task_complete, turn_aborted
	TurnID    string

	// user_message and agent_message
	Message string
	Phase   string

	// task_started and token_count
	ModelContextWindow int

	// token_count
	Tokens     *TokenInfo
	RateLimits json.RawMessage

	// patch_apply_end
	Success bool
	Changes []string // the files the patch touched

	// task_complete and turn_aborted
	DurationMs         int
	TimeToFirstTokenMs int
	Reason             string // turn_aborted reason, e.g. interrupted
	LastAgentMessage   string
}

// TokenInfo is the token_count info block: the running total for the session
// and the delta for the last turn, both broken down by kind.
type TokenInfo struct {
	Total              TokenUsage `json:"total_token_usage"`
	Last               TokenUsage `json:"last_token_usage"`
	ModelContextWindow int        `json:"model_context_window"`
}

// TokenUsage is one token accounting snapshot. Codex counts cached input and
// reasoning output separately, which the lab keeps so a cache-heavy or a
// reasoning-heavy run can be told apart from a plain one.
type TokenUsage struct {
	InputTokens           int `json:"input_tokens"`
	CachedInputTokens     int `json:"cached_input_tokens"`
	OutputTokens          int `json:"output_tokens"`
	ReasoningOutputTokens int `json:"reasoning_output_tokens"`
	TotalTokens           int `json:"total_tokens"`
}

// record is the outer envelope every line shares: a timestamp, a type, and a
// type-specific payload left raw until the type is known.
type record struct {
	Timestamp string          `json:"timestamp"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
}

// passthrough is the metadata Codex threads through response items to tie each
// one back to the turn that produced it.
type passthrough struct {
	Meta struct {
		TurnID string `json:"turn_id"`
	} `json:"internal_chat_message_metadata_passthrough"`
}

// ParseRolloutFile reads and parses a rollout JSONL file at path.
func ParseRolloutFile(path string) (*Rollout, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r, err := ParseRollout(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	r.Path = path
	return r, nil
}

// ParseRollout parses a rollout JSONL stream. It skips blank lines and returns
// the first malformed line as an error, so a truncated trace is reported rather
// than silently half-read.
func ParseRollout(r io.Reader) (*Rollout, error) {
	out := &Rollout{}
	sc := bufio.NewScanner(r)
	// A single apply_patch body or tool output can be large, so give the
	// scanner room well past the default 64KiB line cap.
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	line := 0
	for sc.Scan() {
		line++
		b := sc.Bytes()
		if len(trimSpace(b)) == 0 {
			continue
		}
		var rec record
		if err := json.Unmarshal(b, &rec); err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
		ts := parseTime(rec.Timestamp)
		switch rec.Type {
		case "session_meta":
			out.Meta = parseMeta(rec.Payload, ts)
		case "turn_context":
			out.Turns = append(out.Turns, parseTurn(rec.Payload, ts))
		case "response_item":
			out.Items = append(out.Items, parseItem(rec.Payload, ts))
		case "event_msg":
			out.Events = append(out.Events, parseEvent(rec.Payload, ts))
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func parseMeta(payload json.RawMessage, ts time.Time) SessionMeta {
	var m SessionMeta
	_ = json.Unmarshal(payload, &m)
	m.Timestamp = ts
	var wrap struct {
		Instructions struct {
			Text string `json:"text"`
		} `json:"base_instructions"`
	}
	if json.Unmarshal(payload, &wrap) == nil {
		m.Instructions = wrap.Instructions.Text
	}
	return m
}

func parseTurn(payload json.RawMessage, ts time.Time) TurnContext {
	var t TurnContext
	_ = json.Unmarshal(payload, &t)
	t.Timestamp = ts
	return t
}

func parseItem(payload json.RawMessage, ts time.Time) ResponseItem {
	var raw struct {
		Type      string          `json:"type"`
		Role      string          `json:"role"`
		Content   json.RawMessage `json:"content"`
		ID        string          `json:"id"`
		CallID    string          `json:"call_id"`
		Name      string          `json:"name"`
		Arguments string          `json:"arguments"`
		Input     string          `json:"input"`
		Output    string          `json:"output"`
		Status    string          `json:"status"`
		Encrypted string          `json:"encrypted_content"`
	}
	_ = json.Unmarshal(payload, &raw)
	var pt passthrough
	_ = json.Unmarshal(payload, &pt)
	return ResponseItem{
		Timestamp: ts,
		Type:      raw.Type,
		TurnID:    pt.Meta.TurnID,
		Role:      raw.Role,
		Content:   raw.Content,
		Reasoning: raw.Type == "reasoning",
		ID:        raw.ID,
		CallID:    raw.CallID,
		Name:      raw.Name,
		Arguments: raw.Arguments,
		Input:     raw.Input,
		Status:    raw.Status,
		Output:    raw.Output,
	}
}

func parseEvent(payload json.RawMessage, ts time.Time) Event {
	var raw struct {
		Type               string                     `json:"type"`
		TurnID             string                     `json:"turn_id"`
		Message            string                     `json:"message"`
		Phase              string                     `json:"phase"`
		ModelContextWindow int                        `json:"model_context_window"`
		Info               *TokenInfo                 `json:"info"`
		RateLimits         json.RawMessage            `json:"rate_limits"`
		Success            bool                       `json:"success"`
		Changes            map[string]json.RawMessage `json:"changes"`
		DurationMs         int                        `json:"duration_ms"`
		TimeToFirstToken   int                        `json:"time_to_first_token_ms"`
		Reason             string                     `json:"reason"`
		LastAgentMessage   string                     `json:"last_agent_message"`
	}
	_ = json.Unmarshal(payload, &raw)
	e := Event{
		Timestamp:          ts,
		Type:               raw.Type,
		TurnID:             raw.TurnID,
		Message:            raw.Message,
		Phase:              raw.Phase,
		ModelContextWindow: raw.ModelContextWindow,
		Tokens:             raw.Info,
		RateLimits:         raw.RateLimits,
		Success:            raw.Success,
		DurationMs:         raw.DurationMs,
		TimeToFirstTokenMs: raw.TimeToFirstToken,
		Reason:             raw.Reason,
		LastAgentMessage:   raw.LastAgentMessage,
	}
	for path := range raw.Changes {
		e.Changes = append(e.Changes, path)
	}
	return e
}

func parseTime(s string) time.Time {
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t
	}
	return time.Time{}
}

func trimSpace(b []byte) []byte {
	i, j := 0, len(b)
	for i < j && (b[i] == ' ' || b[i] == '\t' || b[i] == '\r' || b[i] == '\n') {
		i++
	}
	for j > i && (b[j-1] == ' ' || b[j-1] == '\t' || b[j-1] == '\r' || b[j-1] == '\n') {
		j--
	}
	return b[i:j]
}
