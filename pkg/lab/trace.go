package lab

import (
	"bufio"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// traceMetrics is what the harness pulls out of a single attempt's trace dir.
// It replaces the awk and jq the shell harness leaned on with real parsing, so
// a stray line or an empty file yields a zero rather than a silent skew.
type traceMetrics struct {
	MaxRSSKB     int
	ElapsedClock string
	Requests     int
	Tokens       Tokens
	Latency      Latency
}

// readTrace parses every metric file the proxy and GNU time left in a trace dir.
// Missing or malformed files degrade to zero values, never an error, because a
// tool that crashed still deserves a graded, comparable row.
func readTrace(dir string) traceMetrics {
	m := traceMetrics{}
	m.MaxRSSKB, m.ElapsedClock = readTime(filepath.Join(dir, "time.txt"))
	m.Requests = countLines(filepath.Join(dir, "requests.jsonl"))
	m.Tokens = sumTokens(filepath.Join(dir, "usage.jsonl"))
	m.Latency = latencyStats(filepath.Join(dir, "latency.jsonl"))
	return m
}

// readTime pulls the max resident set size (in kbytes) and the wall-clock
// elapsed string out of GNU time's -v report.
func readTime(path string) (rssKB int, elapsed string) {
	f, err := os.Open(path)
	if err != nil {
		return 0, ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		switch {
		case strings.HasPrefix(line, "Maximum resident set size"):
			rssKB = lastInt(line)
		case strings.HasPrefix(line, "Elapsed (wall clock) time"):
			if i := strings.LastIndex(line, ": "); i >= 0 {
				elapsed = strings.TrimSpace(line[i+2:])
			}
		}
	}
	return rssKB, elapsed
}

// countLines counts non-empty lines, which is the request count in a jsonl tap.
func countLines(path string) int {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()
	n := 0
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for sc.Scan() {
		if strings.TrimSpace(sc.Text()) != "" {
			n++
		}
	}
	return n
}

// sumTokens adds up the usage rows the proxy recorded, one per reply.
func sumTokens(path string) Tokens {
	var t Tokens
	forEachJSON(path, func(b []byte) {
		var r struct {
			Prompt     int `json:"prompt_tokens"`
			Completion int `json:"completion_tokens"`
			Total      int `json:"total_tokens"`
		}
		if json.Unmarshal(b, &r) == nil {
			t.Prompt += r.Prompt
			t.Completion += r.Completion
			t.Total += r.Total
		}
	})
	return t
}

// latencyStats averages ttfb and total over the model calls the proxy timed. It
// counts only the completions endpoint with a 200, so a readiness probe or a
// rejected request never skews the numbers.
func latencyStats(path string) Latency {
	var ttfb, total, n int
	forEachJSON(path, func(b []byte) {
		var r struct {
			Status  int    `json:"status"`
			Path    string `json:"path"`
			TTFBMS  int    `json:"ttfb_ms"`
			TotalMS int    `json:"total_ms"`
		}
		if json.Unmarshal(b, &r) != nil || r.Status != 200 {
			return
		}
		if !strings.Contains(r.Path, "chat/completions") && !strings.Contains(r.Path, "/messages") {
			return
		}
		ttfb += r.TTFBMS
		total += r.TotalMS
		n++
	})
	if n == 0 {
		return Latency{}
	}
	return Latency{AvgTTFB: ttfb / n, AvgTotal: total / n, Calls: n}
}

// forEachJSON calls fn with each non-empty line of a jsonl file, skipping a
// missing file so callers stay one-liners.
func forEachJSON(path string, fn func([]byte)) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for sc.Scan() {
		b := sc.Bytes()
		if len(strings.TrimSpace(string(b))) == 0 {
			continue
		}
		fn(b)
	}
}

// dirSizeKB is the on-disk footprint of a work tree in kbytes, summed over every
// regular file. It stands in for du -sk without shelling out.
func dirSizeKB(root string) int {
	var total int64
	_ = filepath.WalkDir(root, func(_ string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if info, err := d.Info(); err == nil {
			total += info.Size()
		}
		return nil
	})
	return int(total / 1024)
}

func lastInt(s string) int {
	fields := strings.Fields(s)
	for i := len(fields) - 1; i >= 0; i-- {
		if n, err := strconv.Atoi(strings.Trim(fields[i], ":")); err == nil {
			return n
		}
	}
	return 0
}
