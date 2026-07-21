package lab

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tamnd/tomo-labs/pkg/lab/inspect"
	// Registers every tool's reader (lexicon + notes) at init, so inspect.Analyze
	// can find the built-in profile and per-tool notes by name. The inspect package
	// deliberately depends on nothing here; this side owns the trace files and glues
	// them onto the inspect types.
	_ "github.com/tamnd/tomo-labs/pkg/lab/inspect/tools"
)

// A result table says a run passed and cost N tokens, but not what the agent
// actually did to get there: which files it read, what it ran, where it went in
// circles. That story lives in the request tap, but reading raw requests.jsonl by
// hand is tedious. Inspect reconstructs the run and hands it to the inspect
// package, which reads it: classifies every move and writes the plain-language
// summary and walkthrough of how the run went.
//
// This file is the glue: it owns the filesystem (finding the newest run and its
// trace files) and the run's result.json (verdict and cost), and maps both onto
// the inspect package's types. The reading itself lives in package inspect, which
// depends on nothing here, so a run's analysis is testable on its own.

// Inspect finds the newest run for a tool (optionally narrowed to one scenario)
// and returns its transcript, summary, and verdict. Scenario is empty to take the
// newest run the tool has across all scenarios.
func (l *Lab) Inspect(tool, scenario string) (*inspect.Transcript, error) {
	if tool == "" {
		return nil, fmt.Errorf("usage: lab inspect <tool> [scenario] [--full] [--json]")
	}
	base := filepath.Join(l.resultsDir(), tool)
	if _, err := os.Stat(base); err != nil {
		return nil, fmt.Errorf("no runs for %q: run `lab run %s` first", tool, tool)
	}
	scenarios := []string{scenario}
	if scenario == "" {
		names, err := subdirs(base)
		if err != nil {
			return nil, err
		}
		scenarios = names
	}

	// Timestamps sort the same lexically as chronologically, so the largest ts
	// with a real request trace is the newest run worth showing.
	var bestReqs, bestScenario, bestTS string
	for _, sc := range scenarios {
		stamps, err := subdirs(filepath.Join(base, sc))
		if err != nil {
			continue
		}
		for _, ts := range stamps {
			reqs := traceRequestFiles(filepath.Join(base, sc, ts))
			if len(reqs) == 0 {
				continue
			}
			if ts > bestTS {
				bestTS, bestScenario, bestReqs = ts, sc, reqs[len(reqs)-1]
			}
		}
	}
	if bestReqs == "" {
		return nil, fmt.Errorf("no captured requests for %q yet", tool)
	}

	steps, err := transcribe(bestReqs)
	if err != nil {
		return nil, err
	}
	prof := l.loadProfile(tool)
	t := &inspect.Transcript{
		Tool:     tool,
		Scenario: bestScenario,
		Run:      bestTS,
		Steps:    steps,
		Summary:  inspect.Analyze(tool, prof, steps),
	}
	readVerdict(t, filepath.Join(base, bestScenario, bestTS, "result.json"))
	return t, nil
}

// readVerdict folds the run's own result.json (verdict, cost, checker note) onto
// the transcript so the summary can lead with what the run actually cost and
// whether it passed. A missing or malformed result leaves the fields at zero: the
// transcript still stands on its own. It also carries the run's rate-limit summary
// across as an inspect.Throttle so a run the upstream cut short reads as a floor,
// not a plain agent failure.
func readVerdict(t *inspect.Transcript, path string) {
	b, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var r Result
	if json.Unmarshal(b, &r) != nil {
		return
	}
	passed := r.Passed
	t.Passed = &passed
	t.Requests = r.Requests
	t.Tokens = r.Tokens.Total
	t.Wall = r.WallSeconds
	t.Check = r.Check
	if r.RateLimit != nil {
		t.Throttle = &inspect.Throttle{Hits: r.RateLimit.Hits, QuotaHits: r.RateLimit.QuotaHits, MaxRetryAfterS: r.RateLimit.MaxRetryAfterS}
	}
}

// transcribe reads one request tap and walks the fullest conversation it holds
// into an ordered list of steps. The fullest request is the one with the most
// messages: an agent resends its growing history every call, so the last full
// call subsumes all the earlier ones. It stays on this side of the boundary
// because it needs contentText from the lab package's prompt reader.
func transcribe(reqFile string) ([]inspect.Step, error) {
	data, err := os.ReadFile(reqFile)
	if err != nil {
		return nil, err
	}
	type message struct {
		Role      string          `json:"role"`
		Content   json.RawMessage `json:"content"`
		ToolCalls []struct {
			Function struct {
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			} `json:"function"`
		} `json:"tool_calls"`
	}
	var fullest []message
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var rec struct {
			Body struct {
				Messages []message `json:"messages"`
			} `json:"body"`
		}
		if json.Unmarshal([]byte(line), &rec) != nil {
			continue
		}
		if len(rec.Body.Messages) > len(fullest) {
			fullest = rec.Body.Messages
		}
	}
	if len(fullest) == 0 {
		return nil, fmt.Errorf("no messages captured in %s", reqFile)
	}

	var steps []inspect.Step
	for _, m := range fullest {
		switch m.Role {
		case "system":
			steps = append(steps, inspect.Step{Kind: "system", Text: contentText(m.Content)})
		case "user":
			steps = append(steps, inspect.Step{Kind: "user", Text: contentText(m.Content)})
		case "assistant":
			if t := contentText(m.Content); t != "" {
				steps = append(steps, inspect.Step{Kind: "assistant", Text: t})
			}
			for _, tc := range m.ToolCalls {
				steps = append(steps, inspect.Step{Kind: "call", Name: tc.Function.Name, Text: strings.TrimSpace(tc.Function.Arguments)})
			}
		case "tool":
			steps = append(steps, inspect.Step{Kind: "result", Text: contentText(m.Content)})
		}
	}
	return steps, nil
}

// loadProfile reads a tool's inspect.json profile if it ships one, else falls
// back to the built-in profile the tool registered in the tools sub-package. The
// file lives beside the tool's Dockerfile and adapter, so a tool's behavioral
// vocabulary is tuned where the tool is defined.
func (l *Lab) loadProfile(tool string) inspect.ToolProfile {
	path := filepath.Join(l.cfg.Root, "tools", tool, "inspect.json")
	if b, err := os.ReadFile(path); err == nil {
		var p inspect.ToolProfile
		if json.Unmarshal(b, &p) == nil && len(p.Lexicon) > 0 {
			return p
		}
	}
	return inspect.BuiltinProfile(tool)
}
