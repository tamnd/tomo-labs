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

// Scenarios lists every scenario directory that has a prompt.
func (l *Lab) Scenarios() ([]Scenario, error) {
	entries, err := os.ReadDir(filepath.Join(l.cfg.Root, "scenarios"))
	if err != nil {
		return nil, err
	}
	var out []Scenario
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(l.cfg.Root, "scenarios", e.Name())
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

// scenario looks up one scenario by name.
func (l *Lab) scenario(name string) (Scenario, error) {
	dir := filepath.Join(l.cfg.Root, "scenarios", name)
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
