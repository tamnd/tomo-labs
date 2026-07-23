package trace

import "regexp"

// The Hub repository is public, so no block may carry a credential. Redaction
// happens here, at the single point every block string passes through on its way
// into a Block (see the block constructors in trace.go), so a captured request
// that echoed a key never reaches a block with it. The publisher's pre-commit
// gate re-runs Scan over every assembled file as a second, independent defense.

// secretPattern is one known credential shape: a name for the abort message and
// a regexp that both detects it and rewrites it.
type secretPattern struct {
	name string
	re   *regexp.Regexp
	// mask is the replacement, using $1-style references to keep a label prefix
	// while dropping the secret value.
	mask string
}

// secretPatterns are the credential shapes that must never reach a public
// commit: the OPENCODE_API_KEY value in the run environment, a subscription
// OAuth bearer, a Hub token, and a generic Authorization bearer. Each is matched
// conservatively enough to catch the real shapes without masking ordinary prose.
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

// redactString rewrites every known secret shape in s to a masked form. It is
// applied by the block constructors, so every text, thinking, and tool-argument
// string is scrubbed before it becomes a block.
func redactString(s string) string {
	for _, p := range secretPatterns {
		s = p.re.ReplaceAllString(s, p.mask)
	}
	return s
}

// Scan reports the name of the first known secret shape found in data, or the
// empty string when data is clean. The publisher's commit gate calls it over
// every assembled file so a leak has to pass both this and block-level redaction
// to escape.
func Scan(data []byte) string {
	for _, p := range secretPatterns {
		if p.re.Match(data) {
			return p.name
		}
	}
	return ""
}
