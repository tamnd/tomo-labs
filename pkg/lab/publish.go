package lab

import (
	"context"
	"fmt"

	"github.com/tamnd/tomo-labs/pkg/publish"
)

// publishRun mirrors a just-finished run to the Hugging Face dataset, as the
// last step of the run loop after the result is written locally. It is
// best-effort by contract: publishing speaks to the network, the least reliable
// thing a run does, and the run is already graded and recorded on disk by the
// time this runs, so a publish failure never sinks the run. It is a no-op when
// publishing is disabled or no HF token is present, and it says so once.
func (l *Lab) publishRun(ctx context.Context, runDir string) {
	if !publish.Enabled() {
		return
	}
	p := publish.NewPublisher(publish.Token(), l.cfg.Data, func(format string, args ...any) {
		fmt.Printf("  "+format+"\n", args...)
	})
	// The publisher already swallows its own errors into the log and returns nil
	// for the per-run path, so the run continues regardless.
	_ = p.PublishRun(ctx, runDir)
}

// IngestSwelive folds a finished swelive container run into the labs data layout
// and publishes it, the durable replacement for the manual backfill that lost the
// early luna runs. A swelive attempt records a bridgetrace and grades offline but
// never writes the result.json the publisher indexes on, so its trace is invisible
// until it is ingested; calling this as the last step of the container wrapper
// means every run self-publishes the moment it finishes. Ingestion is required (a
// bad run dir is an error the caller should see), but the publish that follows is
// best-effort, exactly like the normal run loop's, so a network hiccup never
// undoes the on-disk ingest.
func (l *Lab) IngestSwelive(ctx context.Context, r publish.SweliveRun) error {
	runDir, err := publish.IngestSwelive(l.cfg.Data, r)
	if err != nil {
		return err
	}
	fmt.Printf("ingested %s/%s -> %s\n", r.Tool, r.Scenario, runDir)
	l.publishRun(ctx, runDir)
	return nil
}

// Backfill reconstructs and commits every trace in the local result history in
// one commit, plus the regenerated README and reports, and does the first real
// publish of the dataset. It is the on-demand path behind `lab publish
// --backfill`, so a cold start populates the dataset in a single pass.
func (l *Lab) Backfill(ctx context.Context) error {
	token := publish.Token()
	if token == "" {
		return fmt.Errorf("HF_TOKEN is not set; cannot publish")
	}
	p := publish.NewPublisher(token, l.cfg.Data, func(format string, args ...any) {
		fmt.Printf("  "+format+"\n", args...)
	})
	return p.Backfill(ctx)
}

// PublishDryRun assembles the full backfill commit and runs the secret gate but
// uploads nothing, printing what it would commit and a sample reconstructed
// trace. It is the offline pre-flight for the first real publish.
func (l *Lab) PublishDryRun(_ context.Context) error {
	p := publish.NewPublisher(publish.Token(), l.cfg.Data, nil)
	rep, err := p.DryRun()
	if err != nil {
		return err
	}
	fmt.Printf("dry-run: %d traces, %d files total\n", rep.Traces, rep.Files)
	fmt.Printf("dry-run: commit summary would be %q\n", rep.Summary)
	if rep.Finding != nil {
		return fmt.Errorf("secret gate would ABORT: %v", rep.Finding)
	}
	fmt.Println("dry-run: secret gate clean, no credential in any assembled file")
	if rep.SamplePath != "" {
		fmt.Printf("dry-run: sample trace %s (%d bytes):\n", rep.SamplePath, len(rep.SampleBytes))
		head := rep.SampleBytes
		if len(head) > 1200 {
			head = head[:1200]
		}
		fmt.Println(string(head))
	}
	return nil
}

// PublishAll regenerates the README and reports from the full result set and
// commits them without adding a specific run's trace, the on-demand path behind
// a plain `lab publish`. It shares the backfill core but is named for its intent:
// refresh the dataset's front matter to match what is already on disk.
func (l *Lab) PublishAll(ctx context.Context) error {
	return l.Backfill(ctx)
}
