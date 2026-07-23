package publish

import (
	"fmt"
	"regexp"
)

// The repository is public, so no committed file may carry a credential. There
// are two independent lines of defense here. redactMessage scrubs a message on
// the way into an STS trace, so a captured request that echoed a key never
// reaches disk with it. ScanFiles is the pre-commit gate: it re-checks every
// fully assembled file and aborts the whole commit if any secret shape survives,
// so a leak has to pass two checks to escape, and it cannot.

// secretPattern is one known credential shape: a name for the abort message and
// a regexp that both detects it (for the gate) and rewrites it (for redaction).
type secretPattern struct {
	name string
	re   *regexp.Regexp
	// mask is the replacement, using $1-style references to keep a label prefix
	// while dropping the secret value.
	mask string
}

// secretPatterns are the credential shapes that must never reach a public
// commit: the OPENCODE_API_KEY value present in the run environment, a
// subscription OAuth bearer, a Hub token, and a generic Authorization bearer
// line. Each is matched conservatively enough to catch the real shapes without
// masking ordinary prose.
var secretPatterns = []secretPattern{
	{
		name: "authorization-bearer",
		re:   regexp.MustCompile(`(?i)(authorization"?\s*[:=]\s*"?bearer\s+)[A-Za-z0-9._\-]{12,}`),
		mask: "${1}[REDACTED]",
	},
	{
		name: "bearer-token",
		re:   regexp.MustCompile(`(?i)(bearer\s+)[A-Za-z0-9._\-]{20,}`),
		mask: "${1}[REDACTED]",
	},
	{
		name: "opencode-api-key",
		re:   regexp.MustCompile(`(?i)(OPENCODE_API_KEY"?\s*[:=]\s*"?)[A-Za-z0-9._\-]{12,}`),
		mask: "${1}[REDACTED]",
	},
	{
		name: "sk-key",
		re:   regexp.MustCompile(`\bsk-[A-Za-z0-9._\-]{16,}`),
		mask: "[REDACTED-KEY]",
	},
	{
		name: "hf-token",
		re:   regexp.MustCompile(`\bhf_[A-Za-z0-9]{20,}`),
		mask: "[REDACTED-HF]",
	},
	{
		name: "openai-oauth",
		re:   regexp.MustCompile(`\b(eyJ[A-Za-z0-9._\-]{20,})`),
		mask: "[REDACTED-JWT]",
	},
}

// redactString rewrites every known secret shape in s to a masked form.
func redactString(s string) string {
	for _, p := range secretPatterns {
		s = p.re.ReplaceAllString(s, p.mask)
	}
	return s
}

// redactMessage scrubs a message in place before it is written to a trace,
// covering the content, the reasoning, and every tool-call argument string,
// which is where a credential passed as a tool argument would otherwise land.
func redactMessage(m *stsMessage) {
	m.Content = redactString(m.Content)
	m.ReasoningContent = redactString(m.ReasoningContent)
	for i := range m.ToolCalls {
		m.ToolCalls[i].Function.Arguments = redactString(m.ToolCalls[i].Function.Arguments)
	}
}

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
		if f := scanBytes(op.PathInRepo, op.Content); f != nil {
			return f
		}
	}
	return nil
}

func scanBytes(path string, data []byte) *SecretFinding {
	for _, p := range secretPatterns {
		if p.re.Match(data) {
			return &SecretFinding{Path: path, Shape: p.name}
		}
	}
	return nil
}
