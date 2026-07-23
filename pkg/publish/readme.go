package publish

import (
	"bytes"
	_ "embed"
	"fmt"
	"strings"
	"text/template"
)

//go:embed readme.tmpl
var readmeTmpl string

// readmeData holds the template variables for the dataset card. Every field is
// derived from the aggregate, so the card is a pure function of the result set.
type readmeData struct {
	Traces     int
	NEvals     int
	NScenarios int
	NTools     int
	NModels    int

	ToolsList  string
	ModelsList string

	Board        string
	CheapestNote string
	CoverageRows string

	GeneratedAt string
}

// GenerateREADME builds the dataset card from the aggregate. generatedAt is
// passed in rather than read from the clock so the output is deterministic and
// golden-testable; the publisher supplies the current UTC minute.
func GenerateREADME(ag Aggregate, generatedAt string) string {
	tmpl, err := template.New("readme").Parse(readmeTmpl)
	if err != nil {
		// The template is an embedded constant; a parse error is a programmer bug.
		panic(err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, buildReadmeData(ag, generatedAt)); err != nil {
		panic(err)
	}
	return buf.String()
}

func buildReadmeData(ag Aggregate, generatedAt string) readmeData {
	return readmeData{
		Traces:       ag.Traces,
		NEvals:       len(ag.Evals),
		NScenarios:   len(ag.Scenarios),
		NTools:       len(ag.Tools),
		NModels:      len(ag.Models),
		ToolsList:    orNone(strings.Join(ag.Tools, ", ")),
		ModelsList:   orNone(strings.Join(ag.Models, ", ")),
		Board:        renderBoard(ag),
		CheapestNote: cheapestNote(ag),
		CoverageRows: coverageRows(ag),
		GeneratedAt:  generatedAt,
	}
}

func orNone(s string) string {
	if strings.TrimSpace(s) == "" {
		return "none yet"
	}
	return s
}

// renderBoard renders the solve-rate and cost board as a markdown table, the
// same board reports/board.md carries, so the front page leads with the finding.
func renderBoard(ag Aggregate) string {
	if len(ag.Cells) == 0 {
		return "_No graded runs yet._"
	}
	var b strings.Builder
	b.WriteString("| Eval | Tool | Model | Solved | Tokens | Cost | Wall |\n")
	b.WriteString("|------|------|-------|-------:|-------:|-----:|-----:|\n")
	for _, c := range ag.Cells {
		b.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s | %s | %s |\n",
			c.Eval, c.Tool, orDash(c.Model),
			solvedCell(c), fmtTokens(c.Tokens), fmtCost(c.CostUSD, c.CostKnown), fmtWall(c.WallSec)))
	}
	return strings.TrimRight(b.String(), "\n")
}

// cheapestNote names, per eval, the tool that solved at least one task for the
// fewest tokens, which is the campaign's lead: cheapest win, not just any win.
func cheapestNote(ag Aggregate) string {
	type best struct {
		tool   string
		tokens int
	}
	byEval := map[string]best{}
	for _, c := range ag.Cells {
		if c.Passed == 0 {
			continue
		}
		cur, ok := byEval[c.Eval]
		if !ok || c.Tokens < cur.tokens {
			byEval[c.Eval] = best{tool: c.Tool, tokens: c.Tokens}
		}
	}
	if len(byEval) == 0 {
		return "_No eval has a solver yet._"
	}
	var lines []string
	for _, ev := range ag.Evals {
		if bst, ok := byEval[ev]; ok {
			lines = append(lines, fmt.Sprintf("- **%s**: cheapest solver is `%s` at %s.",
				ev, bst.tool, fmtTokens(bst.tokens)))
		}
	}
	return strings.Join(lines, "\n")
}

func coverageRows(ag Aggregate) string {
	// Count distinct scenarios, tools, and models per eval.
	type cov struct{ scen, tools, models map[string]bool }
	byEval := map[string]*cov{}
	traces := map[string]int{}
	for _, r := range ag.Runs {
		c := byEval[r.Eval]
		if c == nil {
			c = &cov{scen: map[string]bool{}, tools: map[string]bool{}, models: map[string]bool{}}
			byEval[r.Eval] = c
		}
		c.scen[r.Result.Scenario] = true
		c.tools[r.Result.Tool] = true
		if r.Result.Model != "" {
			c.models[r.Result.Model] = true
		}
		traces[r.Eval]++
	}
	if len(byEval) == 0 {
		return "| _none yet_ | 0 | 0 | 0 | 0 |"
	}
	var b strings.Builder
	for _, ev := range ag.Evals {
		c := byEval[ev]
		b.WriteString(fmt.Sprintf("| %s | %d | %d | %d | %d |\n",
			ev, len(c.scen), len(c.tools), len(c.models), traces[ev]))
	}
	return strings.TrimRight(b.String(), "\n")
}
