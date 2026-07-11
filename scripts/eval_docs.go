//go:build ignore

// eval_docs runs one or more eval suites across every wired tool and writes the
// results table into each suite's docs page, so the numbers on the site are the
// numbers a rerun produced rather than a figure typed by hand.
//
// For each named suite it does three things:
//
//  1. runs the suite over every tool with a built image (lab run --suite <name>),
//     unless -report-only is passed, in which case it reuses the runs on disk;
//  2. reads the suite's aggregate (lab report --suite <name> --json), the same
//     latest-run-per-task summary the CLI prints;
//  3. replaces the block between the results markers in
//     docs/content/evals/<name>.md with a fresh table and a snapshot line.
//
// The page owns everything outside the markers, so the prose stays hand-written
// and only the table is machine-owned. Rerunning is idempotent: same runs on
// disk, same table.
//
// The model is whatever LAB_MODEL points at, so the snapshot line names it and a
// reader can tell a deepseek sweep from a fallback-model one. Source the API key
// first (the runs hit the shared upstream), then:
//
//	go build -o bin/lab ./cmd/lab
//	go run scripts/eval_docs.go livecodebench
//	go run scripts/eval_docs.go aider evalplus
//	go run scripts/eval_docs.go -report-only livecodebench   # just redraw the table
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	startMarker = "<!-- eval-results:start -->"
	endMarker   = "<!-- eval-results:end -->"
	labBin      = "bin/lab"
)

// summary mirrors the fields of lab.ToolSummary that the table shows. It is a
// subset, decoded from `lab report --json`, so the script never re-derives any
// number the harness already computed.
type summary struct {
	Tool         string  `json:"tool"`
	Version      string  `json:"version"`
	Runs         int     `json:"runs"`
	Passed       int     `json:"passed"`
	FirstTry     int     `json:"first_try"`
	TotalTokens  int     `json:"total_tokens"`
	AvgTokens    int     `json:"avg_tokens"`
	TotalCostUSD float64 `json:"total_cost_usd"`
	AvgRSSMB     int     `json:"avg_rss_mb"`
	AvgWallS     int     `json:"avg_wall_s"`
	InstallMB    int     `json:"install_mb"`
}

func main() {
	args := os.Args[1:]
	reportOnly := false
	var suites []string
	for _, a := range args {
		switch a {
		case "-report-only", "--report-only":
			reportOnly = true
		default:
			suites = append(suites, a)
		}
	}
	if len(suites) == 0 {
		fatalf("usage: go run scripts/eval_docs.go [-report-only] <suite>...")
	}
	if _, err := os.Stat(labBin); err != nil {
		fatalf("%s not found: build it first with `go build -o %s ./cmd/lab`", labBin, labBin)
	}

	model := envOr("LAB_MODEL", "deepseek-v4-flash-free")
	for _, suite := range suites {
		if !reportOnly {
			fmt.Printf("== running %s over every tool ==\n", suite)
			if err := runSuite(suite); err != nil {
				fatalf("run %s: %v", suite, err)
			}
		}
		sums, err := report(suite)
		if err != nil {
			fatalf("report %s: %v", suite, err)
		}
		if len(sums) == 0 {
			fatalf("report %s: no runs captured yet", suite)
		}
		if err := writeDoc(suite, model, sums); err != nil {
			fatalf("write %s doc: %v", suite, err)
		}
		fmt.Printf("== wrote %d rows into docs/content/evals/%s.md ==\n", len(sums), suite)
	}
}

// runSuite runs every tool with a built image over the whole suite. An empty
// tool and scenario is the harness's "all tools, all tasks" form, and the run
// keeps going past a per-pair error, so the command's own streaming output is
// the progress log and a non-zero exit is a hard failure worth stopping on.
func runSuite(suite string) error {
	cmd := exec.Command(labBin, "run", "--suite", suite)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()
	return cmd.Run()
}

// report reads the suite's aggregate as JSON, the same summary `lab report`
// prints as a table, so the doc is built from the harness's own numbers.
func report(suite string) ([]summary, error) {
	cmd := exec.Command(labBin, "report", "--suite", suite, "--json")
	cmd.Env = os.Environ()
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("%v: %s", err, strings.TrimSpace(errb.String()))
	}
	// No runs yet prints a notice to stderr and nothing to stdout.
	if strings.TrimSpace(out.String()) == "" {
		return nil, nil
	}
	var sums []summary
	if err := json.Unmarshal(out.Bytes(), &sums); err != nil {
		return nil, err
	}
	return sums, nil
}

// writeDoc replaces the block between the results markers in the suite's docs
// page. The rest of the page, the hand-written methodology, is left untouched,
// so this only ever owns the table and its snapshot line.
func writeDoc(suite, model string, sums []summary) error {
	path := filepath.Join("docs", "content", "evals", suite+".md")
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	src := string(b)
	i := strings.Index(src, startMarker)
	j := strings.Index(src, endMarker)
	if i < 0 || j < 0 || j < i {
		return fmt.Errorf("markers %q / %q not found in %s; add them where the table should go", startMarker, endMarker, path)
	}
	block := startMarker + "\n" + renderTable(suite, model, sums) + endMarker
	next := src[:i] + block + src[j+len(endMarker):]
	return os.WriteFile(path, []byte(next), 0o644)
}

// renderTable builds the results section: a snapshot line naming the model and
// task count, then one row per tool ordered by total tokens, cheapest first, the
// way the results page reads. pass is N of the tasks the tool ran, so a tool that
// skipped tasks reads honestly as passed-of-attempted.
func renderTable(suite, model string, sums []summary) string {
	sort.Slice(sums, func(i, j int) bool {
		if sums[i].TotalTokens != sums[j].TotalTokens {
			return sums[i].TotalTokens < sums[j].TotalTokens
		}
		return sums[i].Tool < sums[j].Tool
	})
	var b strings.Builder
	fmt.Fprintf(&b, "Snapshot taken %s on the `%s` model, every tool over the same tasks through the same trace proxy.\n",
		time.Now().UTC().Format("2006-01-02"), model)
	fmt.Fprintf(&b, "Rows are ordered by total tokens, cheapest first, and `pass` is how many of the %d tasks the tool got a passing grade on.\n\n",
		maxRuns(sums))
	b.WriteString("| tool | version | pass | 1st | tokens | avg | cost | rss | wall | install |\n")
	b.WriteString("| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |\n")
	for _, s := range sums {
		fmt.Fprintf(&b, "| %s | %s | %d/%d | %d | %s | %s | %s | %dMB | %ds | %dMB |\n",
			s.Tool, dash(s.Version), s.Passed, s.Runs, s.FirstTry,
			comma(s.TotalTokens), comma(s.AvgTokens), cost(s.TotalCostUSD),
			s.AvgRSSMB, s.AvgWallS, s.InstallMB)
	}
	b.WriteString("\n")
	return b.String()
}

// maxRuns is the largest task count any tool ran, the denominator the pass column
// is read against. A tool that ran fewer still shows its own N in the row.
func maxRuns(sums []summary) int {
	n := 0
	for _, s := range sums {
		if s.Runs > n {
			n = s.Runs
		}
	}
	return n
}

func comma(n int) string {
	s := fmt.Sprintf("%d", n)
	if n < 0 {
		return "-" + comma(-n)
	}
	var out []byte
	for i, c := range []byte(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, c)
	}
	return string(out)
}

func cost(c float64) string {
	if c == 0 {
		return "-"
	}
	return fmt.Sprintf("$%.4f", c)
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func fatalf(format string, a ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", a...)
	os.Exit(1)
}
