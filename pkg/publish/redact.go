package publish

import (
	"fmt"

	"github.com/tamnd/tomo-labs/pkg/trace"
)

// The repository is public, so no committed file may carry a credential. There
// are two independent lines of defense. Block-level redaction in pkg/trace
// scrubs every string on its way into a trace block, so a captured request that
// echoed a key never reaches disk with it. ScanFiles is the pre-commit gate: it
// re-checks every fully assembled file against the same secret shapes and aborts
// the whole commit if any survive, so a leak has to pass both checks to escape.

// SecretFinding names a leaked secret shape and the file it was found in.
type SecretFinding struct {
	Path  string
	Shape string
}

func (f SecretFinding) Error() string {
	return fmt.Sprintf("secret gate: %q matched shape %q; commit aborted", f.Path, f.Shape)
}

// ScanFiles is the pre-commit gate. It scans every assembled file for the known
// secret shapes and returns the first finding, so the publisher can abort the
// whole commit naming the file and shape rather than leak. It returns nil when
// every file is clean.
func ScanFiles(ops []HFOp) *SecretFinding {
	for _, op := range ops {
		if op.Delete || op.Content == nil {
			continue
		}
		if shape := trace.Scan(op.Content); shape != "" {
			return &SecretFinding{Path: op.PathInRepo, Shape: shape}
		}
	}
	return nil
}
