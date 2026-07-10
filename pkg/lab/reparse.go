package lab

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// Reparse recomputes the trace-derived metrics of already-captured runs from the
// raw traces they still hold, without re-running any tool. It exists because the
// traces are the ground truth and the parsing is not: when the orchestration or
// token accounting improves, an old result.json can be brought up to the current
// metric for free instead of paying for another sweep. It walks every
// result.json under the data dir, re-reads the winning attempt's trace, and
// rewrites the run in place. It returns how many runs it refreshed.
func (l *Lab) Reparse(_ context.Context) (int, error) {
	n := 0
	err := filepath.WalkDir(l.cfg.Data, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || d.Name() != "result.json" {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		var r Result
		if json.Unmarshal(b, &r) != nil || r.Tool == "" {
			return nil
		}
		// readTrace ran over the winning-or-last attempt, which Attempts names.
		trace := filepath.Join(filepath.Dir(path), fmt.Sprintf("attempt-%d", max(r.Attempts, 1)), "trace")
		if _, statErr := os.Stat(trace); statErr != nil {
			return nil
		}
		m := readTrace(trace)
		r.Requests = m.Requests
		r.MaxRSSKB = m.MaxRSSKB
		r.ElapsedClock = m.ElapsedClock
		r.Tokens = m.Tokens
		r.Latency = m.Latency
		r.CostUSD = m.CostUSD
		r.Orchestration = m.Orch
		if err := writeResult(path, &r); err != nil {
			return err
		}
		n++
		return nil
	})
	return n, err
}
