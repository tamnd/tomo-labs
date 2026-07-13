// Package claude reads the session transcripts the Claude Code CLI writes to disk
// and turns them into typed Go values the lab can study, the same way the codex
// package reads Codex rollouts.
//
// Claude Code records every session as a JSONL file under
// ~/.claude/projects/<cwd-slug>/<session-id>.jsonl. Each line is one record with
// a type. The types the lab reads are:
//
//   - system: the session header, carrying the cwd, the git branch, the CLI
//     version, and the session id.
//   - user: a user turn. Its message content is either a plain string (the first
//     prompt) or an array of blocks, where a tool_result block carries the output
//     of a tool the assistant called.
//   - assistant: a model turn. Its message carries the model that produced it, the
//     stop reason, the per-call token usage, and a content array of thinking, text,
//     and tool_use blocks.
//
// Other line types (mode, permission-mode, file-history-snapshot, attachment,
// ai-title, last-prompt) are session bookkeeping the lab does not need, so they
// are skipped.
//
// The point of parsing these is the same as for Codex: learn from real runs of a
// strong model. What a Claude model actually did turn by turn, which model tier
// ran, how many tokens across the cache tiers it spent, and, importantly, whether
// it reached outside the sandbox to fetch an answer, so a claimed pass can be told
// from a real one.
package claude

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"
)

// Session is one Claude Code transcript, parsed from a session JSONL file. The
// Messages slice keeps the order the turns appeared in, so a run can be replayed
// from the first prompt to the last tool call.
type Session struct {
	Path      string // the file this was read from, empty when read from a reader
	SessionID string
	Cwd       string
	GitBranch string
	Version   string // the CLI version that wrote the session
	Messages  []Message
}

// Message is one user or assistant turn. The assistant-only fields (Model,
// StopReason, Usage) stay zero on a user turn.
type Message struct {
	Timestamp  time.Time
	Role       string // user or assistant
	Model      string // assistant only, e.g. claude-opus-4-8
	StopReason string // assistant only, e.g. tool_use, end_turn
	Usage      Usage  // assistant only, the token cost of producing this turn
	Blocks     []Block
}

// Block is one content block in a message: a thought, a text reply, a tool call,
// or a tool result. Fields not used by a block's Type stay zero.
type Block struct {
	Type string // thinking, text, tool_use, tool_result

	// text and thinking
	Text string

	// tool_use: a call the assistant made
	ToolID string
	Name   string
	Input  json.RawMessage

	// tool_result: the output of a call, carried on the next user turn
	ToolUseID string
	IsError   bool
	Result    string
}

// Usage is the token accounting for one assistant turn. Unlike Codex, which
// reports a running session total, Claude reports the cost of each call, so a
// session total is the sum across turns. The three input kinds are disjoint: fresh
// input, tokens read from the prompt cache, and tokens written into it, each billed
// at its own rate.
type Usage struct {
	InputTokens         int    // fresh, cache-miss input
	CacheCreationTokens int    // input written into the cache (a cache write)
	CacheReadTokens     int    // input served from the cache (a cache read)
	OutputTokens        int    // output, including any thinking tokens
	Ephemeral5mTokens   int    // of the cache writes, those in the 5-minute tier
	Ephemeral1hTokens   int    // of the cache writes, those in the 1-hour tier
	ServiceTier         string // standard, batch, priority
}

// IsToolCall reports whether the block is a tool call.
func (b Block) IsToolCall() bool { return b.Type == "tool_use" }

// writeTools are the builtin Claude Code tools that change files on disk. A
// tool_use by one of these names is a write, the same idea as an apply_patch in a
// Codex rollout.
var writeTools = map[string]bool{
	"Write":        true,
	"Edit":         true,
	"MultiEdit":    true,
	"NotebookEdit": true,
}

// IsWrite reports whether the block is a tool call that changes files, so the lab
// can count edits the way tomo's governor and the codex analyzer both do. A model
// that edits through Bash (a heredoc or git apply) does not show up here, which is
// why LeakFetch reads Bash separately.
func (b Block) IsWrite() bool { return b.Type == "tool_use" && writeTools[b.Name] }

// WrittenPath pulls the target file out of a write tool's input. Claude's Write,
// Edit, and MultiEdit tools all name it "file_path"; a NotebookEdit names its
// target "notebook_path". Anything else yields "", which the caller does not count.
func (b Block) WrittenPath() string {
	if !b.IsWrite() {
		return ""
	}
	var v struct {
		FilePath     string `json:"file_path"`
		NotebookPath string `json:"notebook_path"`
	}
	if json.Unmarshal(b.Input, &v) != nil {
		return ""
	}
	if v.FilePath != "" {
		return v.FilePath
	}
	return v.NotebookPath
}

// BashCommand returns the shell command of a Bash tool call, or "" when the block
// is not a Bash call. It is what the leak detector reads to catch a run that
// shelled out to fetch an answer.
func (b Block) BashCommand() string {
	if b.Type != "tool_use" || b.Name != "Bash" {
		return ""
	}
	var v struct {
		Command string `json:"command"`
	}
	if json.Unmarshal(b.Input, &v) != nil {
		return ""
	}
	return v.Command
}

// record is the outer envelope shared by every line: the type, the timestamp, and
// the fields the lab reads off the header and the message.
type record struct {
	Type      string          `json:"type"`
	Timestamp string          `json:"timestamp"`
	SessionID string          `json:"sessionId"`
	Cwd       string          `json:"cwd"`
	GitBranch string          `json:"gitBranch"`
	Version   string          `json:"version"`
	Message   json.RawMessage `json:"message"`
}

// rawMessage is the message payload on a user or assistant line. Content is left
// raw because a user message carries it as a string while an assistant message
// carries it as an array of blocks.
type rawMessage struct {
	Role       string          `json:"role"`
	Model      string          `json:"model"`
	StopReason string          `json:"stop_reason"`
	Content    json.RawMessage `json:"content"`
	Usage      *rawUsage       `json:"usage"`
}

type rawUsage struct {
	InputTokens         int    `json:"input_tokens"`
	CacheCreationTokens int    `json:"cache_creation_input_tokens"`
	CacheReadTokens     int    `json:"cache_read_input_tokens"`
	OutputTokens        int    `json:"output_tokens"`
	ServiceTier         string `json:"service_tier"`
	CacheCreation       struct {
		Ephemeral5m int `json:"ephemeral_5m_input_tokens"`
		Ephemeral1h int `json:"ephemeral_1h_input_tokens"`
	} `json:"cache_creation"`
}

type rawBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	Thinking  string          `json:"thinking"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
	ToolUseID string          `json:"tool_use_id"`
	IsError   bool            `json:"is_error"`
	Content   json.RawMessage `json:"content"`
}

// ParseSessionFile reads and parses a session JSONL file at path.
func ParseSessionFile(path string) (*Session, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	s, err := ParseSession(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	s.Path = path
	return s, nil
}

// ParseSession parses a session JSONL stream. Blank lines are skipped and a
// malformed line is reported with its number, so a truncated transcript is caught
// rather than half-read.
func ParseSession(r io.Reader) (*Session, error) {
	out := &Session{}
	sc := bufio.NewScanner(r)
	// A single tool result (a big file read, a long test log) can be large, so give
	// the scanner room well past the default 64KiB line cap.
	sc.Buffer(make([]byte, 0, 64*1024), 32*1024*1024)
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
		// The header carries the session identity; take it from the first line that
		// has it so a session read from any starting point still names itself.
		if rec.SessionID != "" && out.SessionID == "" {
			out.SessionID = rec.SessionID
		}
		if rec.Cwd != "" && out.Cwd == "" {
			out.Cwd = rec.Cwd
		}
		if rec.GitBranch != "" && out.GitBranch == "" {
			out.GitBranch = rec.GitBranch
		}
		if rec.Version != "" && out.Version == "" {
			out.Version = rec.Version
		}
		switch rec.Type {
		case "user", "assistant":
			if m, ok := parseMessage(rec); ok {
				out.Messages = append(out.Messages, m)
			}
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func parseMessage(rec record) (Message, bool) {
	var rm rawMessage
	if json.Unmarshal(rec.Message, &rm) != nil {
		return Message{}, false
	}
	m := Message{
		Timestamp:  parseTime(rec.Timestamp),
		Role:       rec.Type, // the line type is the role: user or assistant
		Model:      rm.Model,
		StopReason: rm.StopReason,
	}
	if rm.Usage != nil {
		m.Usage = Usage{
			InputTokens:         rm.Usage.InputTokens,
			CacheCreationTokens: rm.Usage.CacheCreationTokens,
			CacheReadTokens:     rm.Usage.CacheReadTokens,
			OutputTokens:        rm.Usage.OutputTokens,
			Ephemeral5mTokens:   rm.Usage.CacheCreation.Ephemeral5m,
			Ephemeral1hTokens:   rm.Usage.CacheCreation.Ephemeral1h,
			ServiceTier:         rm.Usage.ServiceTier,
		}
	}
	m.Blocks = parseBlocks(rm.Content)
	return m, true
}

// parseBlocks reads a message's content. An assistant message carries an array of
// blocks; a user message carries either a plain string (the first prompt) or an
// array that can hold tool_result blocks. Both shapes are handled so no turn is
// dropped for its encoding.
func parseBlocks(content json.RawMessage) []Block {
	if len(content) == 0 {
		return nil
	}
	// A plain-string content (the opening user prompt) becomes one text block.
	var s string
	if json.Unmarshal(content, &s) == nil {
		return []Block{{Type: "text", Text: s}}
	}
	var raws []rawBlock
	if json.Unmarshal(content, &raws) != nil {
		return nil
	}
	var out []Block
	for _, rb := range raws {
		b := Block{Type: rb.Type, ToolID: rb.ID, Name: rb.Name, Input: rb.Input, ToolUseID: rb.ToolUseID, IsError: rb.IsError}
		switch rb.Type {
		case "text":
			b.Text = rb.Text
		case "thinking":
			b.Text = rb.Thinking
		case "tool_result":
			b.Result = toolResultText(rb.Content)
		}
		out = append(out, b)
	}
	return out
}

// toolResultText flattens a tool_result's content into a string. The content is
// either a plain string or an array of {type:text, text:...} parts, so both are
// joined into one readable result.
func toolResultText(content json.RawMessage) string {
	if len(content) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(content, &s) == nil {
		return s
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(content, &parts) != nil {
		return ""
	}
	var out string
	for _, p := range parts {
		out += p.Text
	}
	return out
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
