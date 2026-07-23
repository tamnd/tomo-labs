package publish

import (
	"fmt"

	"github.com/tamnd/tomo-labs/pkg/result"
	"github.com/tamnd/tomo-labs/pkg/trace"
)

// Tokens is the token accounting a result carries. It is defined once in
// pkg/result, the shared run-outcome model, and aliased here so the results,
// reports, and README keep referring to publish.Tokens without a second
// definition to drift from it.
type Tokens = result.Tokens

// TracePath returns the repo path for a run's published trace file, the
// coordinate layout data/<eval>/<scenario>/<model>/<tool>-<id>.jsonl. It is the
// Hub-side layout, keyed for browsing by eval and model, distinct from the
// local codex-style date tree the run writes (see trace.DiskPath). Every
// component is slugged so the path is always a single valid segment tree.
func TracePath(eval, scenario, model, tool, id string) string {
	return fmt.Sprintf("data/%s/%s/%s/%s-%s.jsonl",
		trace.Slug(eval), trace.Slug(scenario), trace.Slug(model), trace.Slug(tool), trace.Slug(id))
}
