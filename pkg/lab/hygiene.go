package lab

import (
	"context"
	"os"
	"path/filepath"
	"sort"
)

// Clean removes the leftovers a campaign of builds and runs piles up on the
// container machine: any lab containers still around by name, and the dangling
// <none> images every rebuild orphans. Build already prunes images on its own,
// so this is the on-demand lever for when a run was killed mid-flight or the
// machine has just drifted, exposed as `lab clean`. It leaves the tagged tool
// images in place, so the next run needs no rebuild.
func (l *Lab) Clean(ctx context.Context) {
	for _, name := range []string{l.cfg.proxyName(), l.cfg.webName(), l.cfg.runName()} {
		l.rt.Remove(ctx, name)
	}
	l.rt.PruneImages(ctx)
}

// cacheDirs are directory names that hold build or dependency caches, never a
// graded artifact. The harness strips them from a work tree once grading is
// done. The list is deliberately general, not tied to any one scenario: these
// names mean "regenerable cache" in every ecosystem a tool might reach for.
var cacheDirs = map[string]bool{
	".cache":        true, // go build cache, pip cache, generic XDG cache
	"node_modules":  true, // npm, pnpm, yarn
	"__pycache__":   true, // cpython bytecode
	".venv":         true, // python virtualenv
	"venv":          true,
	".gradle":       true, // gradle
	".npm":          true, // npm cache
	".pytest_cache": true,
	".mypy_cache":   true,
	".ruff_cache":   true,
}

// stripCaches removes regenerable cache directories from a finished work tree.
// It runs after the checker has read the tree, so nothing needed for scoring is
// lost, and it keeps the persisted trace honest: what stays on disk is the work
// the agent produced, not the tool's rebuildable scratch. An agent that builds a
// Go binary or installs npm packages would otherwise leave a cache that dwarfs
// the actual result and piles up across every rerun.
func stripCaches(work string) {
	_ = filepath.WalkDir(work, func(path string, d os.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil
		}
		if cacheDirs[d.Name()] {
			os.RemoveAll(path)
			return filepath.SkipDir
		}
		return nil
	})
}

// pruneOldRuns keeps only the newest keep timestamped run directories under a
// tool/scenario dir and removes the rest. Run directories are named by a UTC
// timestamp in a sortable layout, so a lexical sort is a chronological sort. A
// keep of zero or less keeps everything. This is what stops a long campaign of
// reruns from turning into a slow disk leak: the interesting run is the latest
// one, and the history past keep is not worth the space.
func pruneOldRuns(scenarioDir string, keep int) {
	if keep <= 0 {
		return
	}
	entries, err := os.ReadDir(scenarioDir)
	if err != nil {
		return
	}
	var runs []string
	for _, e := range entries {
		if e.IsDir() {
			runs = append(runs, e.Name())
		}
	}
	if len(runs) <= keep {
		return
	}
	sort.Strings(runs)
	for _, old := range runs[:len(runs)-keep] {
		os.RemoveAll(filepath.Join(scenarioDir, old))
	}
}
