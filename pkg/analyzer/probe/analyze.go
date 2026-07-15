// Package probe (analyzer) reads the raw trace a lab probe run drops
// (trace.jsonl: one record per model call, with the full request and response)
// and derives the numbers that explain where a turn spent its tokens. Its
// headline is the re-send story: an agent turn re-sends the whole growing
// transcript on every round, so the input tokens climb round over round while
// each round's new output stays small. The analyzer makes that curve and its cost
// concrete, so a run can be read at a glance and one engine or prompt change can
// be told from another. It has no tomo dependency: it reads traces only.
package probe

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
)

// Record is one line of trace.jsonl: a single model call with its full request and
// response. Only the fields the analyzer reads are named; the rest of the request
// and response ride along in the raw JSON and are ignored.
type Record struct {
	Round     int   `json:"round"`
	LatencyMs int64 `json:"latency_ms"`
	Request   struct {
		System   string            `json:"System"`
		Messages []json.RawMessage `json:"Messages"`
		Tools    []json.RawMessage `json:"Tools"`
	} `json:"request"`
	Response struct {
		Blocks []struct {
			Type string `json:"Type"`
			Name string `json:"Name"`
		} `json:"Blocks"`
		StopReason string `json:"StopReason"`
		Usage      struct {
			InputTokens       int `json:"input_tokens"`
			CachedInputTokens int `json:"cached_input_tokens"`
			OutputTokens      int `json:"output_tokens"`
		} `json:"Usage"`
	} `json:"response"`
	Error string `json:"error,omitempty"`
}

// Round is the per-call view the report prints: how big the request was, how much
// it cost, and how much of it was the history the model had already seen.
type Round struct {
	N          int
	Messages   int   // conversation length sent this round (grows by ~2 each round)
	InputTok   int   // prompt tokens billed this round
	CachedTok  int   // prompt tokens served from cache, when the provider reports it
	OutputTok  int   // completion tokens produced this round
	InputDelta int   // InputTok minus the prior round's, the new prompt this round carried
	ToolCalls  int   // tool_use blocks the model emitted
	LatencyMs  int64 //
	StopReason string
}

// Report is the whole derived picture of a run: totals, the re-send ratio, the
// biggest per-round input jumps (a fat tool result that then re-sends every later
// round), and the round-by-round curve.
type Report struct {
	Rounds     []Round
	TotalInput int
	TotalCache int
	TotalOut   int
	FirstInput int // round one's prompt: the fixed cost of the task before any history
	LastInput  int // the final round's prompt: how big the transcript grew to
	TotalTools int
	WallMs     int64
}

// Analyze reads a trace.jsonl file and derives its Report.
func Analyze(path string) (Report, error) {
	f, err := os.Open(path)
	if err != nil {
		return Report{}, err
	}
	defer f.Close()
	return analyze(f)
}

func analyze(r io.Reader) (Report, error) {
	var rep Report
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 1024*1024), 64*1024*1024)
	prevInput := 0
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var rec Record
		if err := json.Unmarshal(line, &rec); err != nil {
			return Report{}, fmt.Errorf("round record: %w", err)
		}
		tools := 0
		for _, b := range rec.Response.Blocks {
			if b.Type == "tool_use" {
				tools++
			}
		}
		rd := Round{
			N:          rec.Round,
			Messages:   len(rec.Request.Messages),
			InputTok:   rec.Response.Usage.InputTokens,
			CachedTok:  rec.Response.Usage.CachedInputTokens,
			OutputTok:  rec.Response.Usage.OutputTokens,
			InputDelta: rec.Response.Usage.InputTokens - prevInput,
			ToolCalls:  tools,
			LatencyMs:  rec.LatencyMs,
			StopReason: rec.Response.StopReason,
		}
		prevInput = rec.Response.Usage.InputTokens
		rep.Rounds = append(rep.Rounds, rd)
		rep.TotalInput += rd.InputTok
		rep.TotalCache += rd.CachedTok
		rep.TotalOut += rd.OutputTok
		rep.TotalTools += rd.ToolCalls
		rep.WallMs += rd.LatencyMs
	}
	if err := sc.Err(); err != nil {
		return Report{}, err
	}
	if len(rep.Rounds) > 0 {
		rep.FirstInput = rep.Rounds[0].InputTok
		rep.LastInput = rep.Rounds[len(rep.Rounds)-1].InputTok
	}
	return rep, nil
}

// ResendRatio is total input over total output: how many prompt tokens the turn
// paid for each token it generated. A high ratio is the quadratic-history tell:
// the same transcript re-sent round after round dwarfs the new work.
func (r Report) ResendRatio() float64 {
	if r.TotalOut == 0 {
		return 0
	}
	return float64(r.TotalInput) / float64(r.TotalOut)
}

// CacheHitRate is the share of prompt tokens served from cache across the run. It
// is zero when the provider does not report cached tokens, which is itself the
// finding: without caching the whole re-sent transcript is billed at full price.
func (r Report) CacheHitRate() float64 {
	if r.TotalInput == 0 {
		return 0
	}
	return float64(r.TotalCache) / float64(r.TotalInput)
}

// TopJumps returns the rounds whose input grew most over the prior round, most
// first. A big jump is a fat tool result (a wide read, a long test log) entering
// the transcript, where it then re-sends on every later round; those are the
// rounds worth eliding or capping.
func (r Report) TopJumps(n int) []Round {
	rounds := append([]Round(nil), r.Rounds...)
	sort.Slice(rounds, func(i, j int) bool { return rounds[i].InputDelta > rounds[j].InputDelta })
	if len(rounds) > n {
		rounds = rounds[:n]
	}
	return rounds
}

// LoadSummary reads the sibling summary.json a sim run writes, so the report can
// print the task, engine, and grade next to the token curve. A missing or broken
// summary is not fatal: the trace stands on its own.
func LoadSummary(traceDir string) map[string]any {
	b, err := os.ReadFile(filepath.Join(traceDir, "summary.json"))
	if err != nil {
		return nil
	}
	var m map[string]any
	if json.Unmarshal(b, &m) != nil {
		return nil
	}
	return m
}
