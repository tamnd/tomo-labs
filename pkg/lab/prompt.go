package lab

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// SystemPrompt is one distinct system prompt a tool sent, recovered from the
// proxy's request tap. The proxy records every completion after translating it
// to the chat-completions shape, so a tool's system prompt lands in messages[]
// no matter which wire the tool speaks; the origin wire is kept in Wire, read
// off the tag the proxy leaves in the path (for example "(from responses)").
//
// A run usually holds more than one: the main agent prompt that carries the
// tool schema, plus smaller side prompts a tool makes for itself (opencode asks
// the model for a thread title first, some tools summarize). Requests is how
// many captured completions carried this prompt, and WithTools marks the one
// sent alongside a tool schema, which is the agent's real working prompt.
//
// Most tools splice volatile context into the prompt each run (the date, the
// working directory, a session id, git status), so the same base prompt renders
// as slightly different text every time. Those are grouped as one prompt: Text
// is a fully-rendered representative, and Variants counts how many distinct
// renderings collapsed into it. Variants of 1 means the prompt is static.
type SystemPrompt struct {
	Text      string   `json:"text"`
	Chars     int      `json:"chars"`
	Wire      string   `json:"wire"`
	Requests  int      `json:"requests"`
	Variants  int      `json:"variants"`
	WithTools bool     `json:"with_tools"`
	Tools     []string `json:"tools,omitempty"`
}

// ToolPrompts is every distinct system prompt one tool sent, unioned across the
// runs in scope so nothing is missed: a tool can send a different prompt per
// scenario (a plan-mode preamble, a summarizer, a title generator), and any one
// run only shows the prompts that scenario happened to trigger. Scenario is the
// filter that was applied ("all" when none), Runs is how many runs were scanned,
// and Newest is the newest run's timestamp, kept so the scan is traceable.
type ToolPrompts struct {
	Tool     string         `json:"tool"`
	Scenario string         `json:"scenario"`
	Runs     int            `json:"runs"`
	Newest   string         `json:"newest"`
	Prompts  []SystemPrompt `json:"prompts"`
}

// fromWireRe pulls the origin wire out of a proxy-tagged path like
// "/v1/chat/completions (from responses)". A plain chat request has no tag.
var fromWireRe = regexp.MustCompile(`\(from (\w+)\)`)

// volatile spans that a tool splices into its prompt fresh every run. Masking
// them yields a signature that is stable across runs of the same base prompt, so
// renderings that differ only in the date, a path, or a session id group as one.
// Order matters: timestamps before bare dates before digit runs.
var volatile = []struct {
	re   *regexp.Regexp
	mask string
}{
	{regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`), "<uuid>"},
	{regexp.MustCompile(`\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}(?:\.\d+)?Z?`), "<ts>"},
	{regexp.MustCompile(`\b\d+\.\d+\.\d+(?:[.\-]\w+)?\b`), "<ver>"},
	{regexp.MustCompile(`\d{4}-\d{2}-\d{2}`), "<date>"},
	{regexp.MustCompile(`(?:/[\w.\-]+){2,}`), "<path>"},
	{regexp.MustCompile(`\b[0-9a-f]{8,}\b`), "<id>"},
	{regexp.MustCompile(`\d+`), "<n>"},
}

// promptSig masks the volatile spans of a prompt so two renderings of the same
// base prompt share a signature. Two genuinely different prompts still differ in
// their stable text, so they never collapse together.
func promptSig(text string) string {
	for _, v := range volatile {
		text = v.re.ReplaceAllString(text, v.mask)
	}
	return text
}

// Prompts recovers the system prompts a tool actually sent, read from its
// captured traces rather than from the tool's source, so it is ground truth for
// what reached the model. It scans every run in scope, all attempts, and unions
// the distinct system prompts so a prompt that only shows up in one scenario is
// still captured; the result is ranked with the real agent prompt (the one
// carrying a tool schema) first. An empty scenario scans all of the tool's runs;
// naming a scenario narrows the scan to that scenario's runs.
func (l *Lab) Prompts(tool, scenario string) (*ToolPrompts, error) {
	base := filepath.Join(l.resultsDir(), tool)
	if _, err := os.Stat(base); err != nil {
		return nil, fmt.Errorf("no runs for %q: run `lab run %s` first", tool, tool)
	}
	scenarios := []string{scenario}
	if scenario == "" {
		names, err := subdirs(base)
		if err != nil {
			return nil, err
		}
		scenarios = names
	}

	// Gather the request tap of every run in scope. Timestamps are the compact UTC
	// form, which sorts the same lexically as chronologically, so the largest ts
	// seen is the newest run; keep it for reference while still reading them all.
	var files []string
	var runs int
	var newest string
	for _, sc := range scenarios {
		scDir := filepath.Join(base, sc)
		stamps, err := subdirs(scDir)
		if err != nil {
			continue
		}
		for _, ts := range stamps {
			reqs := traceRequestFiles(filepath.Join(scDir, ts))
			if len(reqs) == 0 {
				continue
			}
			files = append(files, reqs...)
			runs++
			if ts > newest {
				newest = ts
			}
		}
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no captured completions for %q yet", tool)
	}

	prompts := collectPrompts(files)
	if len(prompts) == 0 {
		return nil, fmt.Errorf("%q runs have no system prompt in their traces", tool)
	}
	scope := scenario
	if scope == "" {
		scope = "all"
	}
	return &ToolPrompts{Tool: tool, Scenario: scope, Runs: runs, Newest: newest, Prompts: prompts}, nil
}

// traceRequestFiles returns the requests.jsonl of every attempt under a run dir.
// A retried scenario has more than one attempt; they carry the same prompt, but
// reading them all means a run that only got a full trace on a later attempt is
// still covered.
func traceRequestFiles(runDir string) []string {
	matches, _ := filepath.Glob(filepath.Join(runDir, "attempt-*", "trace", "requests.jsonl"))
	return matches
}

// promptAcc accumulates one distinct system prompt as the requests are scanned.
// Renderings of the same base prompt share a signature and land here together:
// repr keeps the longest rendering seen (the most fully filled-in example),
// variants counts the distinct renderings, and requests counts every completion
// that carried any of them.
type promptAcc struct {
	repr     string
	wire     string
	requests int
	variants map[string]bool
	tools    map[string]bool
	order    int
}

// collectPrompts reads the request tap and returns the distinct system prompts,
// ranked so the agent's working prompt (the one sent with a tool schema, then the
// most-used, then the longest) comes first and the smaller side prompts follow.
func collectPrompts(files []string) []SystemPrompt {
	acc := map[string]*promptAcc{}
	for _, f := range files {
		forEachJSON(f, func(b []byte) {
			var rec struct {
				Method string          `json:"method"`
				Path   string          `json:"path"`
				Body   json.RawMessage `json:"body"`
			}
			if json.Unmarshal(b, &rec) != nil {
				return
			}
			if rec.Method != "POST" || !strings.Contains(rec.Path, "chat/completions") || len(rec.Body) == 0 {
				return
			}
			var body struct {
				Messages []struct {
					Role    string          `json:"role"`
					Content json.RawMessage `json:"content"`
				} `json:"messages"`
				Tools []struct {
					Function struct {
						Name string `json:"name"`
					} `json:"function"`
				} `json:"tools"`
			}
			if json.Unmarshal(rec.Body, &body) != nil {
				return
			}
			wire := parseWire(rec.Path)
			var toolNames []string
			for _, t := range body.Tools {
				if t.Function.Name != "" {
					toolNames = append(toolNames, t.Function.Name)
				}
			}
			// One request can carry more than one system message (codex sends a base
			// prompt then an environment block). Count each distinct text once per
			// request, and attribute the request's tool schema to every system text in
			// it, since they are all part of the same working prompt.
			seen := map[string]bool{}
			for _, m := range body.Messages {
				if m.Role != "system" && m.Role != "developer" {
					continue
				}
				text := contentText(m.Content)
				if text == "" || seen[text] {
					continue
				}
				seen[text] = true
				sig := promptSig(text)
				a := acc[sig]
				if a == nil {
					a = &promptAcc{repr: text, wire: wire, variants: map[string]bool{}, tools: map[string]bool{}, order: len(acc)}
					acc[sig] = a
				}
				a.requests++
				a.variants[text] = true
				if len(text) > len(a.repr) {
					a.repr = text
				}
				for _, n := range toolNames {
					a.tools[n] = true
				}
			}
		})
	}

	out := make([]SystemPrompt, 0, len(acc))
	order := map[string]int{}
	for _, a := range acc {
		tools := make([]string, 0, len(a.tools))
		for n := range a.tools {
			tools = append(tools, n)
		}
		sort.Strings(tools)
		out = append(out, SystemPrompt{
			Text:      a.repr,
			Chars:     len(a.repr),
			Wire:      a.wire,
			Requests:  a.requests,
			Variants:  len(a.variants),
			WithTools: len(tools) > 0,
			Tools:     tools,
		})
		order[a.repr] = a.order
	}
	// Keep order stable and meaningful: the working prompt (with tools) first, then
	// by how often it was sent, then by length, then by first appearance.
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].WithTools != out[j].WithTools {
			return out[i].WithTools
		}
		if out[i].Requests != out[j].Requests {
			return out[i].Requests > out[j].Requests
		}
		if out[i].Chars != out[j].Chars {
			return out[i].Chars > out[j].Chars
		}
		return order[out[i].Text] < order[out[j].Text]
	})
	return out
}

// parseWire reports the wire a request came in on, read from the proxy's path
// tag. An untagged chat-completions request is native chat.
func parseWire(path string) string {
	if m := fromWireRe.FindStringSubmatch(path); m != nil {
		return m[1]
	}
	return "chat"
}

// contentText flattens a chat message's content into plain text. Content is
// either a JSON string or an array of typed parts ({"type":"text","text":...});
// both shapes appear across the wires, so handle each and fall back to the raw
// bytes rather than dropping an unrecognized shape.
func contentText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return strings.TrimSpace(s)
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &parts) == nil {
		var b strings.Builder
		for _, p := range parts {
			if p.Text != "" {
				if b.Len() > 0 {
					b.WriteByte('\n')
				}
				b.WriteString(p.Text)
			}
		}
		if b.Len() > 0 {
			return strings.TrimSpace(b.String())
		}
	}
	return strings.TrimSpace(string(raw))
}

// subdirs returns the names of the immediate subdirectories of dir, sorted.
func subdirs(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

// WritePrompts prints a tool's recovered system prompts: a one-line header per
// prompt with its wire, size, request count, and tool schema, then the full text
// when full is set. It is the readable side of Prompts; --json gives the same
// data as a struct.
func WritePrompts(w io.Writer, tp *ToolPrompts, full bool) {
	fmt.Fprintf(w, "%s: %d system prompt(s) across %d run(s) (scope=%s, newest=%s)\n", tp.Tool, len(tp.Prompts), tp.Runs, tp.Scenario, tp.Newest)
	for i, p := range tp.Prompts {
		role := "side prompt"
		if p.WithTools {
			role = "agent prompt"
		}
		variance := ""
		if p.Variants > 1 {
			variance = fmt.Sprintf(" | %d per-run renderings", p.Variants)
		}
		fmt.Fprintf(w, "\n[%d] %s | wire=%s | %d chars | %d request(s)%s\n", i+1, role, p.Wire, p.Chars, p.Requests, variance)
		if len(p.Tools) > 0 {
			fmt.Fprintf(w, "    tools (%d): %s\n", len(p.Tools), strings.Join(p.Tools, ", "))
		}
		if full {
			fmt.Fprintf(w, "----- begin system prompt -----\n%s\n----- end system prompt -----\n", p.Text)
		}
	}
}
