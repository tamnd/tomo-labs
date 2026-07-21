package lab

import (
	"fmt"
	"sort"
	"strings"
)

// The cap names a Stop field carries. Rendered, "rate-limit" reads as 429
// because that is what the wire actually said.
const (
	stopTimeout   = "timeout"
	stopTurns     = "turns"
	stopRateLimit = "rate-limit"
	stopQuota     = "quota"
)

// stopReason classifies a finished attempt at write time: which cap ended it,
// if any. A pass is never capped, whatever the clock did, because the work
// passed the checker. A fail is capped when the wall clock killed the
// container, when the run burned its whole turn budget, or when the upstream
// starved it: every call rejected, or a back-off longer than the attempt is
// allowed to live, which means the run could never have continued.
func stopReason(r *Result, maxTurns, attemptSecs int) string {
	if r.Passed {
		return ""
	}
	if r.ExitCode == exitTimeout {
		return stopTimeout
	}
	if r.RateLimit != nil {
		if r.RateLimit.QuotaHits > 0 && r.Tokens.Total == 0 {
			return stopQuota
		}
		if r.Tokens.Total == 0 {
			return stopRateLimit
		}
		if attemptSecs > 0 && r.RateLimit.MaxRetryAfterS > attemptSecs {
			return stopRateLimit
		}
	}
	if maxTurns > 0 && r.Orchestration.ModelCalls >= maxTurns {
		return stopTurns
	}
	return ""
}

// stopOf is the report's view of a run's cap. New runs carry the verdict in
// result.json; historical rows predate the field, so the unambiguous halves
// are derived the same way stopReason does it, minus the turn cap, which
// cannot be derived without knowing the budget the run ran under.
func stopOf(r *Result) string {
	if r.Passed {
		return ""
	}
	if r.Stop != "" {
		return r.Stop
	}
	if r.ExitCode == exitTimeout {
		return stopTimeout
	}
	if r.RateLimit != nil && r.Tokens.Total == 0 {
		if r.RateLimit.QuotaHits > 0 {
			return stopQuota
		}
		return stopRateLimit
	}
	return ""
}

// capCell renders a row's capped-run count with the cap types spelled out, so
// an infrastructure artifact is named where it is counted: "2 (429)" when one
// kind capped both, "3 (2 429, 1 timeout)" when they mix, "-" when nothing
// was capped.
func capCell(kinds map[string]int) string {
	total := 0
	for _, n := range kinds {
		total += n
	}
	if total == 0 {
		return "-"
	}
	names := make([]string, 0, len(kinds))
	for k := range kinds {
		names = append(names, k)
	}
	sort.Strings(names)
	if len(names) == 1 {
		return fmt.Sprintf("%d (%s)", total, capLabel(names[0]))
	}
	parts := make([]string, 0, len(names))
	for _, k := range names {
		parts = append(parts, fmt.Sprintf("%d %s", kinds[k], capLabel(k)))
	}
	return fmt.Sprintf("%d (%s)", total, strings.Join(parts, ", "))
}

// capLabel is the rendered name of a cap: the rate-limit cap reads as 429
// because that is the status the upstream sent.
func capLabel(kind string) string {
	if kind == stopRateLimit {
		return "429"
	}
	return kind
}
