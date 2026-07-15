package probe

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
)

// projRecord is a trace line read for projection: unlike Record it keeps the raw
// message bytes, since an elision strategy has to weigh what each round carried by
// how much of it was stale tool output.
type projRecord struct {
	Round   int `json:"round"`
	Request struct {
		System   string            `json:"System"`
		Messages []json.RawMessage `json:"Messages"`
	} `json:"request"`
	Response struct {
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"Usage"`
	} `json:"response"`
}

// message is the one shape a trace message needs for projection: its role and its
// content, so a tool result can be told from a user or assistant turn and weighed
// by size. The provider serializes richer messages, but role and a stringifiable
// content are enough to model what a strategy would keep or drop.
type message struct {
	Role    string          `json:"Role"`
	Content json.RawMessage `json:"Content"`
	// A tool result rides as its own message in this loop; the analyzer only needs
	// its byte weight, so any extra fields are ignored.
}

// Projection is a counterfactual: what one turn's input tokens would have been
// under a transcript-shaping strategy, derived from the real per-round tokens the
// trace recorded so the estimate is grounded, not invented.
type Projection struct {
	Name       string
	PerRound   []int // projected billed input tokens per round
	TotalInput int   // sum of PerRound
	TotalOut   int   // unchanged: a strategy shapes the prompt, not the output
}

// ResendRatio mirrors Report.ResendRatio for a projection, so a strategy's ratio
// reads next to the actual one.
func (p Projection) ResendRatio() float64 {
	if p.TotalOut == 0 {
		return 0
	}
	return float64(p.TotalInput) / float64(p.TotalOut)
}

// readProjRecords reads a trace.jsonl keeping the message bytes each round carried.
func readProjRecords(r io.Reader) ([]projRecord, error) {
	var recs []projRecord
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 1024*1024), 64*1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var rec projRecord
		if err := json.Unmarshal(line, &rec); err != nil {
			return nil, fmt.Errorf("project record: %w", err)
		}
		recs = append(recs, rec)
	}
	return recs, sc.Err()
}

// ProjectFile reads a trace and returns the actual curve plus every modeled
// strategy, so one call yields the whole comparison the report prints.
func ProjectFile(path string, cacheRate float64, keepLast int) (actual Projection, strategies []Projection, err error) {
	f, err := os.Open(path)
	if err != nil {
		return Projection{}, nil, err
	}
	defer f.Close()
	recs, err := readProjRecords(f)
	if err != nil {
		return Projection{}, nil, err
	}
	return project(recs, cacheRate, keepLast)
}

func project(recs []projRecord, cacheRate float64, keepLast int) (Projection, []Projection, error) {
	actual := Projection{Name: "actual"}
	for _, r := range recs {
		actual.PerRound = append(actual.PerRound, r.Response.Usage.InputTokens)
		actual.TotalInput += r.Response.Usage.InputTokens
		actual.TotalOut += r.Response.Usage.OutputTokens
	}
	cache := projectCache(recs, cacheRate)
	elide := projectElide(recs, keepLast)
	elideCache := projectCacheOf(elide, cacheRate)
	elideCache.Name = fmt.Sprintf("elide>%d + cache", keepLast)
	return actual, []Projection{cache, elide, elideCache}, nil
}

// projectCache models prefix caching, the trick strong tools survive the quadratic
// re-send with: everything the model already saw last round is a byte-stable prefix
// this round, so it is billed at the provider's cache-read rate instead of full
// price. The loop here is append-only, so the whole prior request is that prefix.
// Grounded on the recorded per-round input tokens, no re-tokenizing.
func projectCache(recs []projRecord, cacheRate float64) Projection {
	p := Projection{Name: fmt.Sprintf("prefix cache @%.0f%%", cacheRate*100)}
	prev := 0
	for _, r := range recs {
		in := r.Response.Usage.InputTokens
		cached := min(prev, in) // the stable prefix carried over from last round
		billed := (in - cached) + int(float64(cached)*cacheRate)
		p.PerRound = append(p.PerRound, billed)
		p.TotalInput += billed
		p.TotalOut += r.Response.Usage.OutputTokens
		prev = in
	}
	return p
}

// projectCacheOf applies the same prefix-cache model to an already-shaped curve
// (e.g. after elision), so the two levers compose: shrink the transcript, then
// cache what remains.
func projectCacheOf(base Projection, cacheRate float64) Projection {
	p := Projection{Name: "cache", TotalOut: base.TotalOut}
	prev := 0
	for _, in := range base.PerRound {
		cached := min(prev, in)
		billed := (in - cached) + int(float64(cached)*cacheRate)
		p.PerRound = append(p.PerRound, billed)
		p.TotalInput += billed
		prev = in
	}
	return p
}

// projectElide models dropping stale tool results: past the newest keepLast of
// them, a tool result is replaced by a short stub, since a wide read or a long log
// from twenty rounds ago is rarely load-bearing yet re-sends every round. The
// token estimate scales the round's real input by the share of its message bytes
// the strategy keeps, so a round that was mostly stale output shrinks the most.
func projectElide(recs []projRecord, keepLast int) Projection {
	p := Projection{Name: fmt.Sprintf("elide tool results >%d rounds old", keepLast)}
	const stubBytes = 40 // "[earlier tool result elided]" and the like
	for _, r := range recs {
		msgs := decodeMessages(r.Request.Messages)
		// Index the tool-result messages so the newest keepLast survive whole.
		var toolIdx []int
		for i, m := range msgs {
			if isToolResult(m) {
				toolIdx = append(toolIdx, i)
			}
		}
		keptFrom := 0
		if len(toolIdx) > keepLast {
			keptFrom = len(toolIdx) - keepLast
		}
		elided := map[int]bool{}
		for i := 0; i < keptFrom; i++ {
			elided[toolIdx[i]] = true
		}
		total, kept := len(r.Request.System), len(r.Request.System)
		for i, m := range msgs {
			b := len(m.Content) + len(m.Role)
			total += b
			if elided[i] {
				kept += stubBytes
			} else {
				kept += b
			}
		}
		in := r.Response.Usage.InputTokens
		billed := in
		if total > 0 {
			billed = int(float64(in) * float64(kept) / float64(total))
		}
		p.PerRound = append(p.PerRound, billed)
		p.TotalInput += billed
		p.TotalOut += r.Response.Usage.OutputTokens
	}
	return p
}

func decodeMessages(raw []json.RawMessage) []message {
	out := make([]message, 0, len(raw))
	for _, r := range raw {
		var m message
		_ = json.Unmarshal(r, &m)
		out = append(out, m)
	}
	return out
}

// isToolResult reports whether a message is a tool result, the elidable kind. The
// provider marks these with the "tool" role; a read of its content bytes is all the
// strategy weighs.
func isToolResult(m message) bool {
	return m.Role == "tool" || m.Role == "tool_result"
}
