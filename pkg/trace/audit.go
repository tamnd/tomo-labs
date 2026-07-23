package trace

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tamnd/tomo-labs/pkg/analyzer/claude"
)

// The audit answers one question about a capture: did we miss anything? Every
// agent writes its own complete session log natively (codex writes a rollout
// jsonl under ~/.codex/sessions/YYYY/MM/DD/), which the harness copies out under
// the run's trace/native/. That native log is ground truth: it records every
// turn, tool call, and reply the agent actually made. Tallying it and our own
// reconstructed session.jsonl by block type, then comparing, proves our capture
// dropped nothing. A reconstruction that is missing the final reply or a tool
// call shows up as a lower count against the oracle.

// Counts is a session's block-type census: how many of each renderable block it
// carries, and whether it ends on a non-empty assistant reply. It is the unit of
// comparison between a reconstruction and its oracle.
type Counts struct {
	System     int  `json:"system"`
	User       int  `json:"user"`
	Assistant  int  `json:"assistant"` // assistant text blocks
	Thinking   int  `json:"thinking"`
	ToolCall   int  `json:"toolCall"`
	ToolResult int  `json:"toolResult"`
	FinalReply bool `json:"finalReply"` // the last assistant text block is non-empty
}

// Tally counts a message list by block type. Text blocks are split by role, so a
// missing final assistant reply is visible as a lower assistant count rather than
// hiding in an aggregate text total. FinalReply tracks the last assistant text
// seen, the reply a downstream reader most wants and the original capture most
// often dropped.
func Tally(msgs []Message) Counts {
	var c Counts
	for _, m := range msgs {
		for _, b := range m.Content {
			switch b.Type {
			case "text":
				switch m.Role {
				case "system":
					c.System++
				case "user":
					c.User++
				case "assistant":
					c.Assistant++
					c.FinalReply = strings.TrimSpace(b.Text) != ""
				}
			case "thinking":
				c.Thinking++
			case "toolCall":
				c.ToolCall++
			case "toolResult":
				c.ToolResult++
			}
		}
	}
	return c
}

// rolloutLine is one line of a codex native rollout: a typed envelope whose
// payload, for a response_item, is exactly a Responses item this package already
// decodes.
type rolloutLine struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// AuditCodex reads a codex native rollout and returns its conversation as the
// same Message list a reconstruction produces, so the two tally identically. It
// maps every response_item payload through the shared itemMessage, ignoring the
// rollout's UI events (event_msg), per-turn config (turn_context), and session
// header (session_meta), none of which is a conversation turn.
func AuditCodex(rolloutPath string) ([]Message, error) {
	data, err := os.ReadFile(rolloutPath)
	if err != nil {
		return nil, err
	}
	var msgs []Message
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var env rolloutLine
		if json.Unmarshal([]byte(line), &env) != nil || env.Type != "response_item" {
			continue
		}
		var it respItem
		if json.Unmarshal(env.Payload, &it) != nil {
			continue
		}
		if m, ok := itemMessage(it); ok {
			msgs = append(msgs, m)
		}
	}
	return msgs, nil
}

// CodexRollout finds the newest rollout jsonl under a run's native capture,
// nativeDir being the trace/native the harness copied codex's ~/.codex/sessions
// into. It returns "" when no rollout is present, so a run whose native store was
// not captured audits as simply un-auditable rather than erroring.
func CodexRollout(nativeDir string) string {
	var rollouts []string
	_ = filepath.Walk(nativeDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		base := filepath.Base(path)
		if strings.HasPrefix(base, "rollout-") && strings.HasSuffix(base, ".jsonl") {
			rollouts = append(rollouts, path)
		}
		return nil
	})
	if len(rollouts) == 0 {
		return ""
	}
	// The rollout name is timestamp-lexicographic, so the last is the newest.
	sort.Strings(rollouts)
	return rollouts[len(rollouts)-1]
}

// opencodeLine is one line of the opencode native-session dump the harness
// writes at capture time: a message's role joined to one of that message's
// parts. opencode 1.x persists a run to a SQLite database (a message table and a
// part table, each part's data a JSON blob), which the container dumps to JSONL
// in part-creation order, so the audit reads plain JSON here rather than linking
// a SQLite driver. A part is one of: text (a reply or a prompt, told apart by the
// message role), reasoning (assistant thinking), or tool (one call carrying both
// its input and, once the call returns, its output).
type opencodeLine struct {
	Role string `json:"role"`
	Part struct {
		Type   string `json:"type"`
		Text   string `json:"text"`
		Tool   string `json:"tool"`
		CallID string `json:"callID"`
		State  struct {
			Status string          `json:"status"`
			Input  json.RawMessage `json:"input"`
			Output string          `json:"output"`
		} `json:"state"`
	} `json:"part"`
}

// AuditOpencode reads an opencode native-session dump and returns its
// conversation as the same Message list a reconstruction produces. A text part is
// a turn in its message's role, a reasoning part is assistant thinking, and a
// tool part expands to two turns — the assistant's call and the tool's result —
// so a completed tool part tallies as one toolCall and one toolResult, matching
// how the chat-completions reconstruction pairs a call with its output.
func AuditOpencode(dumpPath string) ([]Message, error) {
	data, err := os.ReadFile(dumpPath)
	if err != nil {
		return nil, err
	}
	var msgs []Message
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var ln opencodeLine
		if json.Unmarshal([]byte(line), &ln) != nil {
			continue
		}
		switch ln.Part.Type {
		case "text":
			if strings.TrimSpace(ln.Part.Text) == "" {
				continue
			}
			role := ln.Role
			if role == "" {
				role = "assistant"
			}
			msgs = append(msgs, Message{Role: role, Content: []Block{textBlock(ln.Part.Text)}})
		case "reasoning":
			if strings.TrimSpace(ln.Part.Text) == "" {
				continue
			}
			msgs = append(msgs, Message{Role: "assistant", Content: []Block{thinkingBlock(ln.Part.Text)}})
		case "tool":
			msgs = append(msgs, Message{
				Role:    "assistant",
				Content: []Block{toolCallBlock(wireCall{ID: ln.Part.CallID, Name: ln.Part.Tool, Args: string(ln.Part.State.Input)})},
			})
			if ln.Part.State.Status == "completed" || ln.Part.State.Output != "" {
				msgs = append(msgs, Message{
					Role:    "tool",
					Content: []Block{toolResultBlock(ln.Part.CallID, ln.Part.State.Output)},
				})
			}
		}
	}
	return msgs, nil
}

// OpencodeDump returns the opencode native-session dump under a run's native
// capture, or "" when none is present. The harness writes it to
// native/opencode/session.jsonl.
func OpencodeDump(nativeDir string) string {
	p := filepath.Join(nativeDir, "opencode", "session.jsonl")
	if _, err := os.Stat(p); err == nil {
		return p
	}
	return ""
}

// AuditClaude reads a Claude Code session transcript and returns its
// conversation as the same Message list a reconstruction produces, reusing the
// analyzer's session parser. Claude records a tool call as an assistant tool_use
// block and its result as a tool_result block on the following user turn, so the
// two map to a toolCall and a toolResult exactly as the wire reconstruction pairs
// them, and a plain text block maps to a turn in its message's role.
func AuditClaude(sessionPath string) ([]Message, error) {
	sess, err := claude.ParseSessionFile(sessionPath)
	if err != nil {
		return nil, err
	}
	var msgs []Message
	for _, m := range sess.Messages {
		for _, b := range m.Blocks {
			switch b.Type {
			case "text":
				if strings.TrimSpace(b.Text) == "" {
					continue
				}
				msgs = append(msgs, Message{Role: m.Role, Content: []Block{textBlock(b.Text)}})
			case "thinking":
				if strings.TrimSpace(b.Text) == "" {
					continue
				}
				msgs = append(msgs, Message{Role: "assistant", Content: []Block{thinkingBlock(b.Text)}})
			case "tool_use":
				msgs = append(msgs, Message{
					Role:    "assistant",
					Content: []Block{toolCallBlock(wireCall{ID: b.ToolID, Name: b.Name, Args: string(b.Input)})},
				})
			case "tool_result":
				msgs = append(msgs, Message{
					Role:    "tool",
					Content: []Block{toolResultBlock(b.ToolUseID, b.Result)},
				})
			}
		}
	}
	return msgs, nil
}

// ClaudeSession returns the richest Claude Code transcript under a run's native
// capture — the one with the most turns when a project dir holds several, since
// a session id is a UUID and carries no time order — or "" when none is present.
// The harness copies ~/.claude/projects into native/claude.
func ClaudeSession(nativeDir string) string {
	var best string
	bestN := 0
	_ = filepath.Walk(filepath.Join(nativeDir, "claude"), func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".jsonl") {
			return nil
		}
		if sess, err := claude.ParseSessionFile(path); err == nil && len(sess.Messages) > bestN {
			bestN = len(sess.Messages)
			best = path
		}
		return nil
	})
	return best
}

// NativeOracle locates and reads whichever agent's native session log a run
// captured under trace/native, returning the tool name and the oracle message
// list. An empty tool name means no recognized native store was present, so the
// run is un-auditable rather than failed. It probes each tool the audit
// understands — codex (a Responses rollout), claude (a Claude Code transcript),
// and opencode (a dumped session) — so one audit path serves every tool without
// the caller knowing which one ran.
func NativeOracle(nativeDir string) (tool string, msgs []Message, err error) {
	if p := CodexRollout(nativeDir); p != "" {
		m, e := AuditCodex(p)
		return "codex", m, e
	}
	if p := ClaudeSession(nativeDir); p != "" {
		m, e := AuditClaude(p)
		return "claude", m, e
	}
	if p := OpencodeDump(nativeDir); p != "" {
		m, e := AuditOpencode(p)
		return "opencode", m, e
	}
	return "", nil, nil
}

// AuditReport is the outcome of comparing a reconstruction against its oracle:
// both censuses and the specific ways the reconstruction fell short, if any.
type AuditReport struct {
	Oracle  Counts   `json:"oracle"`
	Ours    Counts   `json:"ours"`
	Missing []string `json:"missing,omitempty"` // human-readable shortfalls
}

// OK reports whether the reconstruction lost nothing: it carries at least as many
// of every block type as the oracle and, when the oracle ended on a reply, so
// does the reconstruction. A reconstruction may legitimately carry MORE (it also
// captures the system instructions the oracle files under its session header), so
// the test is a floor, not equality.
func (r AuditReport) OK() bool { return len(r.Missing) == 0 }

// Compare tallies both sides and lists every block type where the reconstruction
// fell below the oracle, plus a dropped final reply. It is the audit's verdict:
// an empty Missing means the capture is complete.
func Compare(oracle, ours []Message) AuditReport {
	o, u := Tally(oracle), Tally(ours)
	var missing []string
	shortfall := func(name string, want, got int) {
		if got < want {
			missing = append(missing, name+": oracle "+itoa(want)+", ours "+itoa(got))
		}
	}
	shortfall("user messages", o.User, u.User)
	shortfall("assistant replies", o.Assistant, u.Assistant)
	shortfall("reasoning blocks", o.Thinking, u.Thinking)
	shortfall("tool calls", o.ToolCall, u.ToolCall)
	shortfall("tool results", o.ToolResult, u.ToolResult)
	if o.FinalReply && !u.FinalReply {
		missing = append(missing, "final assistant reply is missing")
	}
	return AuditReport{Oracle: o, Ours: u, Missing: missing}
}

// itoa is a dependency-free int-to-string for the audit's messages.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
