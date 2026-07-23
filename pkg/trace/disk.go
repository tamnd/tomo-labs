package trace

import (
	"os"
	"path/filepath"
	"strings"
)

// The canonical on-disk artifact for a run's conversation is a single
// append-schema JSONL under a date tree, the same shape ~/.codex/sessions and
// ~/.claude use: one self-describing session file per run, its leading session
// record carrying the run's provenance (harness, eval, scenario, model, result),
// so any downstream consumer reads one file, top to bottom, with no directory of
// ad-hoc captures to reassemble. The raw wire captures a run also keeps are
// debug artifacts; this file is the source of truth.

// DiskPath is where a run's session file lives under root:
// sessions/<YYYY>/<MM>/<DD>/<tool>-<scenario>-<id>.jsonl, the day taken from the
// run's timestamp so the tree orders by when the run happened, exactly like a
// codex rollout path. Every path component is slugged to a single safe segment.
func DiskPath(root string, h Header) string {
	base := baseTime(h.Timestamp)
	name := Slug(orUnknown(h.Harness)) + "-" + Slug(orUnknown(h.Scenario)) + "-" + Slug(orUnknown(h.ID)) + ".jsonl"
	return filepath.Join(root, "sessions",
		base.Format("2006"), base.Format("01"), base.Format("02"), name)
}

// WriteSession encodes a run's conversation and writes it to its DiskPath under
// root, creating the day directory, and returns the path written. It is the
// finish step a run calls once its trace is captured, so the standard session
// file exists on disk the moment the run ends and no downstream step has to
// reconstruct it.
func WriteSession(root, traceDir string, h Header) (string, error) {
	data, err := Encode(traceDir, h)
	if err != nil {
		return "", err
	}
	path := DiskPath(root, h)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// Slug flattens a path component to a single safe segment: letters, digits, and
// the punctuation a filename tolerates are kept, everything else becomes a dash,
// and an empty component becomes "unknown" so a path never has an empty segment.
func Slug(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "unknown"
	}
	repl := func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			return r
		case r == '-', r == '_', r == '.':
			return r
		default:
			return '-'
		}
	}
	return strings.Map(repl, s)
}
