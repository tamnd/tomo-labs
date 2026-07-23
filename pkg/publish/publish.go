package publish

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// The Publisher assembles a run's commit (the trace plus the regenerated README
// and reports), runs the secret gate, and commits it to the Hub. It is a pure
// function of the results on disk: it does not care whether a result was just
// produced or produced an hour ago, so an unpublished run is never lost, only
// deferred to the next publish that reads it back.

// Repo is the dataset this package publishes to.
const Repo = "open-index/tomo-traces"

// Publisher owns an HF client and the data root it reads results from.
type Publisher struct {
	client *HFClient
	root   string
	logf   func(string, ...any)
	now    func() time.Time
}

// NewPublisher builds a publisher for the data root, authenticated with token.
// logf receives progress and outcome lines; pass nil to discard them.
func NewPublisher(token, root string, logf func(string, ...any)) *Publisher {
	if logf == nil {
		logf = func(string, ...any) {}
	}
	return &Publisher{
		client: NewHFClient(token, Repo),
		root:   root,
		logf:   logf,
		now:    time.Now,
	}
}

// Token reports whether an HF token is present in the environment.
func Token() string { return os.Getenv("HF_TOKEN") }

// Enabled reports whether a run should publish: a token is present and the
// disable switch is not set. Publishing is on by default when HF_TOKEN is set,
// so every run publishes without anyone remembering to ask, and off when
// TOMO_LABS_PUBLISH=0 or when no token is present.
func Enabled() bool {
	if os.Getenv("TOMO_LABS_PUBLISH") == "0" {
		return false
	}
	return Token() != ""
}

// PublishRun mirrors one just-finished run to the Hub in a single commit: the
// run's STS trace, plus the README and reports regenerated from the full result
// set under the data root. It is best-effort by contract: every error is logged
// with its classification and returned as nil so the run still exits success,
// because the run is already graded and recorded locally by the time this runs.
func (p *Publisher) PublishRun(ctx context.Context, runDir string) error {
	runs, err := LoadRuns(p.root)
	if err != nil {
		p.logf("publish: load results failed, skipping: %v", err)
		return nil
	}
	target, ok := findRun(runs, runDir)
	if !ok {
		p.logf("publish: run %s not found under %s, skipping", runDir, p.root)
		return nil
	}
	traceOp, ok := p.traceOp(target)
	if !ok {
		p.logf("publish: run %s has no reconstructable trace, skipping", target.RunID)
		return nil
	}
	ops := append([]HFOp{traceOp}, p.frontmatter(runs)...)
	summary := runHeadline(target)
	return p.commit(ctx, ops, summary)
}

// Backfill reconstructs and commits every trace in the local result history in
// one pass, plus the regenerated front matter, so a cold start populates the
// dataset in a single commit. Unlike PublishRun it is not best-effort: the
// caller invoked it deliberately, so an error is returned.
func (p *Publisher) Backfill(ctx context.Context) error {
	runs, err := LoadRuns(p.root)
	if err != nil {
		return fmt.Errorf("load results: %w", err)
	}
	if len(runs) == 0 {
		return fmt.Errorf("no results under %s", p.root)
	}
	var ops []HFOp
	traced := 0
	for _, r := range runs {
		if op, ok := p.traceOp(r); ok {
			ops = append(ops, op)
			traced++
		}
	}
	ops = append(ops, p.frontmatter(runs)...)
	p.logf("publish: backfilling %d traces from %d runs", traced, len(runs))
	summary := fmt.Sprintf("backfill: %d traces across %d evals", traced, distinctEvals(runs))
	return p.commit(ctx, ops, summary)
}

// DryRunReport is what a dry-run publish assembles without touching the network:
// the ops it would commit, the gate finding if any, and a sample trace for eyes.
type DryRunReport struct {
	Traces      int
	Files       int
	Finding     *SecretFinding
	SamplePath  string
	SampleBytes []byte
	Summary     string
}

// DryRun assembles the full backfill commit and runs the secret gate over it,
// but uploads nothing. It is the safe pre-flight for the first real publish to a
// public repo: it proves the traces reconstruct, the front matter generates, and
// no assembled file trips the gate, all offline.
func (p *Publisher) DryRun() (DryRunReport, error) {
	runs, err := LoadRuns(p.root)
	if err != nil {
		return DryRunReport{}, fmt.Errorf("load results: %w", err)
	}
	var ops []HFOp
	traced := 0
	var sample HFOp
	for _, r := range runs {
		if op, ok := p.traceOp(r); ok {
			ops = append(ops, op)
			if traced == 0 {
				sample = op
			}
			traced++
		}
	}
	ops = append(ops, p.frontmatter(runs)...)
	rep := DryRunReport{
		Traces:      traced,
		Files:       len(ops),
		Finding:     ScanFiles(ops),
		SamplePath:  sample.PathInRepo,
		SampleBytes: sample.Content,
		Summary:     fmt.Sprintf("backfill: %d traces across %d evals", traced, distinctEvals(runs)),
	}
	return rep, nil
}

// traceOp reconstructs a run's STS trace and returns it as a commit op. It
// returns ok false when the run has no trace directory to reconstruct from.
func (p *Publisher) traceOp(r Run) (HFOp, bool) {
	if r.TraceDir == "" {
		return HFOp{}, false
	}
	data, err := EncodeTrace(r.TraceDir, r.meta())
	if err != nil || len(data) == 0 {
		return HFOp{}, false
	}
	res := r.Result
	path := TracePath(r.Eval, res.Scenario, res.Model, res.Tool, r.RunID)
	return HFOp{PathInRepo: path, Content: data}, true
}

// frontmatter builds the ops for the regenerated README and every report, from
// the full run set. generatedAt is pinned to the minute so a publish that
// changes no result is a byte-identical no-op.
func (p *Publisher) frontmatter(runs []Run) []HFOp {
	ag := Fold(runs)
	generatedAt := p.now().UTC().Format("2006-01-02 15:04 UTC")
	ops := []HFOp{
		{PathInRepo: "README.md", Content: []byte(GenerateREADME(ag, generatedAt))},
	}
	reports := GenerateReports(ag, generatedAt)
	paths := make([]string, 0, len(reports))
	for path := range reports {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		ops = append(ops, HFOp{PathInRepo: path, Content: reports[path]})
	}
	return ops
}

// commit runs the secret gate over the assembled files, then creates the repo
// (idempotent) and uploads. A gate finding aborts the whole commit before any
// byte reaches the Hub. Network and Hub errors are logged with classification;
// PublishRun swallows them, Backfill returns them.
func (p *Publisher) commit(ctx context.Context, ops []HFOp, summary string) error {
	if f := ScanFiles(ops); f != nil {
		err := fmt.Errorf("%w", f)
		p.logf("publish: ABORT, %v", err)
		return err
	}
	if err := p.client.CreateDatasetRepo(ctx, false); err != nil {
		p.logf("publish: create repo failed (%s): %v", classOf(err), err)
		return err
	}
	p.client.Message = summary
	if err := p.client.UploadFiles(ctx, ops); err != nil {
		p.logf("publish: upload failed (%s): %v", classOf(err), err)
		return err
	}
	p.logf("publish: committed %d files to %s: %s", len(ops), Repo, summary)
	return nil
}

func classOf(err error) string {
	switch {
	case IsRateLimit(err):
		return "ratelimit"
	case IsFatal(err):
		return "fatal"
	default:
		return "transient"
	}
}

// findRun locates the loaded run whose directory matches runDir, tolerating a
// trailing separator or a relative-vs-absolute mismatch by comparing cleaned
// paths and falling back to the run-id basename.
func findRun(runs []Run, runDir string) (Run, bool) {
	want := filepath.Clean(runDir)
	base := filepath.Base(want)
	for _, r := range runs {
		if filepath.Clean(r.Dir) == want {
			return r, true
		}
	}
	for _, r := range runs {
		if r.RunID == base {
			return r, true
		}
	}
	return Run{}, false
}

func distinctEvals(runs []Run) int {
	seen := map[string]bool{}
	for _, r := range runs {
		seen[r.Eval] = true
	}
	return len(seen)
}

// runHeadline is the commit summary for a single run, so the dataset's commit
// log reads as a run log, for example
// "run: tomo-oi on dynaconf-1225 (gpt-5.6-luna) pass".
func runHeadline(r Run) string {
	res := r.Result
	var b strings.Builder
	b.WriteString("run: ")
	b.WriteString(res.Tool)
	b.WriteString(" on ")
	b.WriteString(res.Scenario)
	if res.Model != "" {
		b.WriteString(" (")
		b.WriteString(res.Model)
		b.WriteString(")")
	}
	b.WriteString(" ")
	b.WriteString(outcome(res))
	return b.String()
}

// commitMessage is the generic commit summary the HF client falls back to when
// the publisher does not set an explicit headline. It reports how many traces,
// README, and report files the commit carries, so even an unlabeled commit reads
// meaningfully in the dataset history.
func commitMessage(files []preparedFile) (summary, description string) {
	var traces, reports int
	var readme bool
	for _, f := range files {
		switch {
		case f.op.Delete:
			// deletions do not shape the headline
		case strings.HasPrefix(f.op.PathInRepo, "data/"):
			traces++
		case f.op.PathInRepo == "README.md":
			readme = true
		case strings.HasPrefix(f.op.PathInRepo, "reports/"):
			reports++
		}
	}
	parts := []string{}
	if traces > 0 {
		parts = append(parts, fmt.Sprintf("%d trace(s)", traces))
	}
	if readme {
		parts = append(parts, "README")
	}
	if reports > 0 {
		parts = append(parts, fmt.Sprintf("%d report(s)", reports))
	}
	if len(parts) == 0 {
		return "publish: update dataset", ""
	}
	return "publish: " + strings.Join(parts, ", "), "Regenerated from the tomo-labs result set."
}
