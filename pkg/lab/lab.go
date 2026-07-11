// Package lab is the agent-eval harness as a library. It builds the tool
// images, runs one tool against one scenario in a throwaway container with its
// LLM traffic routed through the trace proxy, grades the work the tool left on
// disk, and aggregates the runs into a report.
//
// Every run captures the same axes for every tool, so the comparison is fair:
// pass or fail by artifact, token usage, request count, peak memory, model
// latency, install footprint, wall time, and disk written. The shell front end
// (cmd/lab) is a thin wrapper; embedders drive the same methods directly.
package lab

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tamnd/tomo-labs/pkg/container"
)

// Lab is a configured harness bound to one container runtime.
type Lab struct {
	cfg Config
	rt  *container.CLI
}

// New resolves the container runtime and returns a Lab. It does not build or run
// anything; it just fixes the runtime and config for the calls that follow.
func New(ctx context.Context, cfg Config) (*Lab, error) {
	rt, err := container.Detect(ctx)
	if err != nil {
		return nil, err
	}
	if cfg.Root == "" {
		cfg = DefaultConfig()
	}
	return &Lab{cfg: cfg, rt: rt}, nil
}

// Runtime is the resolved container command name, for display.
func (l *Lab) Runtime() string { return l.rt.Bin }

// tasksDir is the directory the current suite draws tasks from. The empty suite
// is the core hand-written set under scenarios/; a named suite is a separate tier
// materialized under evals/<name>/tasks/, so a public dataset never mixes into
// the core comparison. A task in either place is the same shape (prompt.txt plus
// an optional setup.sh and check.sh), so the whole run and grade path is shared.
func (l *Lab) tasksDir() string {
	if l.cfg.Suite == "" {
		return filepath.Join(l.cfg.Root, "scenarios")
	}
	return filepath.Join(l.cfg.Root, "evals", l.cfg.Suite, "tasks")
}

// resultsDir is where a suite's runs and results land. Core runs keep the bare
// data root so nothing about the existing layout moves; a named suite gets its
// own subtree, which is what keeps its report separate from the core table and
// lets a heavy public dataset be pruned or thrown away on its own. Tool image
// metadata (version, install size) is not per suite, so it stays at the data
// root and is read from there regardless of the active suite.
func (l *Lab) resultsDir() string {
	if l.cfg.Suite == "" {
		return l.cfg.Data
	}
	return filepath.Join(l.cfg.Data, "evals", l.cfg.Suite)
}

// suiteDir is the root of the active suite's tier, the parent of both its tasks/
// and the sibling dirs a generator keeps out of the agent's reach (answers/ for a
// reference solution, oracle/ for a hidden test). It is only meaningful for a
// named suite, since the core scenarios are hand-written and have no generator.
func (l *Lab) suiteDir() string {
	return filepath.Join(l.cfg.Root, "evals", l.cfg.Suite)
}

// Scenario is one task definition on disk.
type Scenario struct {
	Name   string // directory name, e.g. 06-codegen-primes
	Desc   string // first line of the desc file, if any
	dir    string
	graded bool // whether the scenario ships a check.sh to pass or fail against
}

// Tools lists the wired tools: every directory under tools/ that is not the
// shared base and carries both a Dockerfile and an adapter.
func (l *Lab) Tools() ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(l.cfg.Root, "tools"))
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() || e.Name() == "base" {
			continue
		}
		dir := filepath.Join(l.cfg.Root, "tools", e.Name())
		if exists(filepath.Join(dir, "Dockerfile")) && exists(filepath.Join(dir, "adapter.sh")) {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out, nil
}

// Scenarios lists every scenario directory that has a prompt, drawn from the
// active suite's task dir.
func (l *Lab) Scenarios() ([]Scenario, error) {
	entries, err := os.ReadDir(l.tasksDir())
	if err != nil {
		return nil, err
	}
	var out []Scenario
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(l.tasksDir(), e.Name())
		if !exists(filepath.Join(dir, "prompt.txt")) {
			continue
		}
		out = append(out, Scenario{
			Name: e.Name(), Desc: firstLine(filepath.Join(dir, "desc")), dir: dir,
			graded: exists(filepath.Join(dir, "check.sh")),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// scenario looks up one scenario by name within the active suite.
func (l *Lab) scenario(name string) (Scenario, error) {
	dir := filepath.Join(l.tasksDir(), name)
	if !exists(filepath.Join(dir, "prompt.txt")) {
		return Scenario{}, fmt.Errorf("unknown scenario: %s", name)
	}
	return Scenario{
		Name: name, Desc: firstLine(filepath.Join(dir, "desc")), dir: dir,
		graded: exists(filepath.Join(dir, "check.sh")),
	}, nil
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func firstLine(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	if i := strings.IndexByte(string(b), '\n'); i >= 0 {
		return strings.TrimSpace(string(b[:i]))
	}
	return strings.TrimSpace(string(b))
}
