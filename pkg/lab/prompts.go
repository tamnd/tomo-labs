package lab

import (
	"embed"
	"strings"
	"text/template"
)

// promptFS holds the task-prompt templates as their own markdown files rather than
// string builders in Go, so the wording an agent actually reads lives in one
// reviewable place and stays faithful to the upstream benchmark prompt. The
// SWE-bench and SWE-bench-Live inference prompt (create_instance.py) gives the
// issue and asks for a fix, with no instruction about tests; the test-file rules
// live only in the agent scaffolds, not the benchmark, so these templates carry
// none either. The grader owns the tests: check.sh resets any test file the agent
// touched before applying the hidden patch.
//
//go:embed prompts/*.md
var promptFS embed.FS

var taskPrompts = template.Must(template.ParseFS(promptFS, "prompts/*.md"))

// promptData is the interpolation set every task prompt shares: the repository the
// agent is working in and the issue text it must resolve.
type promptData struct {
	Repo    string
	Problem string
}

// renderPrompt executes the named template in prompts/ against the repo and issue.
func renderPrompt(name, repo, problem string) string {
	var b strings.Builder
	// The templates are compiled in at build time, so an execution error is a
	// programming mistake, not a runtime condition; surface it in the output rather
	// than swallow it so a broken template is caught the first time it renders.
	if err := taskPrompts.ExecuteTemplate(&b, name, promptData{Repo: repo, Problem: strings.TrimSpace(problem)}); err != nil {
		return "prompt render error: " + err.Error()
	}
	return b.String()
}
