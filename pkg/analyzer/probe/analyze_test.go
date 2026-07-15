package probe

import (
	"strings"
	"testing"
)

// A two-round trace: the second round re-sends the first round's prompt plus a new
// tool result, so input climbs while output stays small. The analyzer should see
// the growth, the delta, and the re-send ratio.
const twoRounds = `{"round":1,"latency_ms":100,"request":{"System":"sys","Messages":[{"Role":"user"}]},"response":{"Blocks":[{"Type":"tool_use","Name":"read"}],"StopReason":"tool_use","Usage":{"input_tokens":1000,"output_tokens":50}}}
{"round":2,"latency_ms":200,"request":{"System":"sys","Messages":[{"Role":"user"},{"Role":"assistant"},{"Role":"tool"}]},"response":{"Blocks":[{"Type":"text"}],"StopReason":"end_turn","Usage":{"input_tokens":3000,"output_tokens":50}}}`

func TestAnalyzeCurve(t *testing.T) {
	rep, err := analyze(strings.NewReader(twoRounds))
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Rounds) != 2 {
		t.Fatalf("rounds = %d, want 2", len(rep.Rounds))
	}
	if rep.TotalInput != 4000 || rep.TotalOut != 100 {
		t.Errorf("totals in=%d out=%d, want 4000/100", rep.TotalInput, rep.TotalOut)
	}
	if rep.FirstInput != 1000 || rep.LastInput != 3000 {
		t.Errorf("first=%d last=%d, want 1000/3000", rep.FirstInput, rep.LastInput)
	}
	if got := rep.Rounds[1].InputDelta; got != 2000 {
		t.Errorf("round 2 delta = %d, want 2000", got)
	}
	if got := rep.ResendRatio(); got != 40 {
		t.Errorf("resend ratio = %.0f, want 40", got)
	}
	if rep.TotalTools != 1 {
		t.Errorf("tool calls = %d, want 1", rep.TotalTools)
	}
	// The biggest jump is round two, carrying +2000 into the transcript.
	if top := rep.TopJumps(1); len(top) != 1 || top[0].N != 2 {
		t.Errorf("top jump = %+v, want round 2", top)
	}
}

// projectCache models the prefix cache strong tools survive the re-send with: the
// prior round's prompt is a byte-stable prefix this round, billed at the cache
// rate, so the total input must fall well below the actual re-sent total.
func TestProjectCacheCutsResend(t *testing.T) {
	recs, err := readProjRecords(strings.NewReader(twoRounds))
	if err != nil {
		t.Fatal(err)
	}
	actual, strategies, err := project(recs, 0.1, 8)
	if err != nil {
		t.Fatal(err)
	}
	if actual.TotalInput != 4000 {
		t.Fatalf("actual input = %d, want 4000", actual.TotalInput)
	}
	// round 1: 1000 uncached. round 2: 2000 new + 0.1*1000 cached = 2100. total 3100.
	var cache Projection
	for _, s := range strategies {
		if strings.HasPrefix(s.Name, "prefix cache") {
			cache = s
		}
	}
	if cache.TotalInput != 3100 {
		t.Errorf("cache projection = %d, want 3100", cache.TotalInput)
	}
	if cache.TotalInput >= actual.TotalInput {
		t.Errorf("cache %d should be below actual %d", cache.TotalInput, actual.TotalInput)
	}
}
