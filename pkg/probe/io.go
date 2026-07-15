package probe

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/tamnd/tomo/pkg/provider"
)

// countingProvider wraps a provider to count rounds and tokens and, when a trace
// file is set, write one self-contained JSON record per model call: the full
// request (model, system, messages, tool defs) and the full response (blocks,
// stop reason, usage), plus the wall-clock latency of the call. The record is
// everything the call saw and returned, so a trace replays the whole turn and can
// be reloaded as history to resume or fork.
type countingProvider struct {
	inner     provider.Provider
	trace     *os.File
	rounds    int
	inTokens  int
	outTokens int
	lastStop  string
	latencies []int64 // per-round wall-clock, ms
}

func (c *countingProvider) Stream(ctx context.Context, req provider.Request, emit func(provider.Event)) (*provider.Response, error) {
	c.rounds++
	round := c.rounds
	start := time.Now()
	resp, err := c.inner.Stream(ctx, req, emit)
	elapsed := time.Since(start).Milliseconds()
	c.latencies = append(c.latencies, elapsed)
	if resp != nil {
		c.inTokens += resp.Usage.InputTokens
		c.outTokens += resp.Usage.OutputTokens
		c.lastStop = resp.StopReason
	}
	if c.trace != nil {
		rec := traceRecord{Round: round, LatencyMs: elapsed, Request: req, Response: resp}
		if err != nil {
			rec.Error = err.Error()
		}
		if b, mErr := json.Marshal(rec); mErr == nil {
			c.trace.Write(append(b, '\n'))
		}
	}
	return resp, err
}

type traceRecord struct {
	Round     int                `json:"round"`
	LatencyMs int64              `json:"latency_ms"`
	Request   provider.Request   `json:"request"`
	Response  *provider.Response `json:"response,omitempty"`
	Error     string             `json:"error,omitempty"`
}

// event is one thing the turn did: a chunk of assistant text, a tool call with its
// full input, or a tool result with its full output. Every event is timestamped
// relative to the turn start, so the transcript shows the real order and pacing of
// the run: what the model said, what it called, what came back.
type event struct {
	Seq       int             `json:"seq"`
	ElapsedMs int64           `json:"elapsed_ms"`
	Kind      string          `json:"kind"` // "text" | "tool_start" | "tool_end"
	Name      string          `json:"name,omitempty"`
	Text      string          `json:"text,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	Result    string          `json:"result,omitempty"`
	IsErr     bool            `json:"is_err,omitempty"`
}

// metricsSink implements agent.Sink and logs everything the turn does. It tallies
// tool activity for the summary and, in full, records every assistant text, every
// tool call with its input, and every tool result with its output, so nothing the
// engine did is dropped from the record. The trajectory is one entry per tool
// call, in order, so it shows the real shape of the run rather than the inflated
// counts a transcript scan gives, where every tool definition is re-sent each round.
type metricsSink struct {
	calls      map[string]int
	trajectory []string
	events     []event
	start      time.Time
	w          *os.File // events.jsonl, optional
}

func newMetricsSink(w *os.File) *metricsSink {
	return &metricsSink{calls: map[string]int{}, start: time.Now(), w: w}
}

func (m *metricsSink) record(e event) {
	e.Seq = len(m.events) + 1
	e.ElapsedMs = time.Since(m.start).Milliseconds()
	m.events = append(m.events, e)
	if m.w != nil {
		if b, err := json.Marshal(e); err == nil {
			m.w.Write(append(b, '\n'))
		}
	}
}

func (m *metricsSink) Text(s string) {
	if strings.TrimSpace(s) == "" {
		return
	}
	m.record(event{Kind: "text", Text: s})
}

func (m *metricsSink) ToolStart(name string, input json.RawMessage) {
	m.calls[name]++
	m.trajectory = append(m.trajectory, name)
	m.record(event{Kind: "tool_start", Name: name, Input: input})
}

func (m *metricsSink) ToolEnd(name, result string, isErr bool) {
	m.record(event{Kind: "tool_end", Name: name, Result: result, IsErr: isErr})
}

// writeTranscript renders the event stream as a readable markdown log: the model's
// text, each tool call with its input, and each result, in order, with timing. It
// is the human-facing view of "log full everything", next to the machine-readable
// events.jsonl and trace.jsonl.
func writeTranscript(path string, r Result, events []event) error {
	var b strings.Builder
	fmt.Fprintf(&b, "# probe transcript: %s | %s | --engine %s\n\n", r.Task, r.Model, r.Engine)
	fmt.Fprintf(&b, "rounds %d, tool calls %d, tokens in %d out %d, %.1fs%s\n\n",
		r.Rounds, r.ToolCallsN, r.InputTokens, r.OutputTokens, r.ElapsedSecs, boolTag(r.TimedOut, " (timed out)"))
	for _, e := range events {
		ts := fmt.Sprintf("%6.1fs", float64(e.ElapsedMs)/1000)
		switch e.Kind {
		case "text":
			fmt.Fprintf(&b, "[%s] assistant:\n%s\n\n", ts, strings.TrimRight(e.Text, "\n"))
		case "tool_start":
			fmt.Fprintf(&b, "[%s] -> %s %s\n", ts, e.Name, oneLine(string(e.Input), 400))
		case "tool_end":
			tag := ""
			if e.IsErr {
				tag = " ERROR"
			}
			fmt.Fprintf(&b, "[%s]    %s%s: %s\n\n", ts, e.Name, tag, oneLine(e.Result, 600))
		}
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

func oneLine(s string, n int) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " ⏎ "))
	if len(s) > n {
		return s[:n] + fmt.Sprintf(" …(+%d chars)", len(s)-n)
	}
	return s
}

func boolTag(b bool, tag string) string {
	if b {
		return tag
	}
	return ""
}
