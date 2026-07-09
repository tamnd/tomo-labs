package lab

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"text/tabwriter"
)

// writeResult writes a Result as pretty JSON. The file is the unit the report
// aggregates, so it is written whole and atomically enough for a local run.
func writeResult(path string, r *Result) error {
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}

// RunAll runs a set of tools over a set of scenarios. Empty tools means every
// wired tool with a built image; empty scenarios means all of them. It keeps
// going on a per-run error so one broken pair does not abort the sweep, and
// returns the results it did capture.
func (l *Lab) RunAll(ctx context.Context, tools, scenarios []string) ([]*Result, error) {
	if len(tools) == 0 {
		all, err := l.Tools()
		if err != nil {
			return nil, err
		}
		tools = all
	}
	if len(scenarios) == 0 {
		all, err := l.Scenarios()
		if err != nil {
			return nil, err
		}
		for _, s := range all {
			scenarios = append(scenarios, s.Name)
		}
	}
	var out []*Result
	for _, t := range tools {
		if !l.rt.ImageExists(ctx, toolPrefix+t) {
			fmt.Fprintf(os.Stderr, "skip %s: image missing, run: lab build %s\n", t, t)
			continue
		}
		for _, s := range scenarios {
			res, err := l.RunOne(ctx, t, s)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error %s/%s: %v\n", t, s, err)
				continue
			}
			out = append(out, res)
		}
	}
	return out, nil
}

// ToolSummary is one tool's aggregate across every captured run, the row the
// comparison table is built from.
type ToolSummary struct {
	Tool        string  `json:"tool"`
	Runs        int     `json:"runs"`
	Passed      int     `json:"passed"`
	FirstTry    int     `json:"first_try"`
	Retried     int     `json:"retried"`
	AvgAttempts float64 `json:"avg_attempts"`
	InstallMB   int     `json:"install_mb"`
	ImageMB     int     `json:"image_mb"`
	TotalTokens int     `json:"total_tokens"`
	AvgTokens   int     `json:"avg_tokens"`
	AvgRSSMB    int     `json:"avg_rss_mb"`
	AvgTTFBMS   int     `json:"avg_ttfb_ms"`
	AvgWallS    int     `json:"avg_wall_s"`
}

// Report reads every result.json under the data dir and aggregates it per tool.
// A single full sweep writes one result.json per tool and scenario, so the
// summary is over exactly that sweep; repeated sweeps accumulate.
func (l *Lab) Report(_ context.Context) ([]ToolSummary, error) {
	var results []*Result
	err := filepath.WalkDir(l.cfg.Data, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || d.Name() != "result.json" {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		var r Result
		if json.Unmarshal(b, &r) == nil && r.Tool != "" {
			results = append(results, &r)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return summarize(results), nil
}

func summarize(results []*Result) []ToolSummary {
	byTool := map[string][]*Result{}
	for _, r := range results {
		byTool[r.Tool] = append(byTool[r.Tool], r)
	}
	var out []ToolSummary
	for tool, rs := range byTool {
		s := ToolSummary{Tool: tool, Runs: len(rs)}
		var tokens, rss, ttfb, wall, attempts, timed int
		for _, r := range rs {
			if r.Passed {
				s.Passed++
			}
			a := max(r.Attempts, 1)
			attempts += a
			if a == 1 && r.Passed {
				s.FirstTry++
			}
			if a > 1 {
				s.Retried++
			}
			tokens += r.Tokens.Total
			rss += r.MaxRSSKB
			wall += r.WallSeconds
			if r.Latency.Calls > 0 {
				ttfb += r.Latency.AvgTTFB
				timed++
			}
			// Install and image footprint is a property of the tool, not the run,
			// so the last one seen wins; they are all the same.
			s.InstallMB = r.InstallKB / 1024
			s.ImageMB = r.ImageKB / 1024
		}
		n := len(rs)
		s.TotalTokens = tokens
		s.AvgTokens = tokens / n
		s.AvgRSSMB = rss / n / 1024
		s.AvgWallS = wall / n
		s.AvgAttempts = float64(attempts) / float64(n)
		if timed > 0 {
			s.AvgTTFBMS = ttfb / timed
		}
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Tool < out[j].Tool })
	return out
}

// WriteTable renders the summaries as an aligned text table, the human view of a
// comparison.
func WriteTable(w io.Writer, sums []ToolSummary) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "TOOL\tRUNS\tPASS\t1ST-TRY\tRETRIED\tAVG-TRIES\tTOKENS\tAVG-TOK\tRSS-MB\tTTFB-MS\tWALL-S\tINSTALL-MB\tIMAGE-MB")
	for _, s := range sums {
		fmt.Fprintf(tw, "%s\t%d\t%d\t%d\t%d\t%.2f\t%d\t%d\t%d\t%d\t%d\t%d\t%d\n",
			s.Tool, s.Runs, s.Passed, s.FirstTry, s.Retried, s.AvgAttempts,
			s.TotalTokens, s.AvgTokens, s.AvgRSSMB, s.AvgTTFBMS, s.AvgWallS, s.InstallMB, s.ImageMB)
	}
	tw.Flush()
}
