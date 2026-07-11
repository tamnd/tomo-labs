package lab

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"sort"
)

// Clean removes the leftovers a campaign of builds and runs piles up on the
// container machine: any lab containers still around by name, and the dangling
// <none> images every rebuild orphans. Build already prunes images on its own,
// so this is the on-demand lever for when a run was killed mid-flight or the
// machine has just drifted, exposed as `lab clean`. It leaves the tagged tool
// images in place, so the next run needs no rebuild.
func (l *Lab) Clean(ctx context.Context) {
	for _, name := range l.orphanContainers(ctx) {
		l.rt.Remove(ctx, name)
	}
	l.rt.PruneImages(ctx)
}

// orphanContainers finds every proxy, web, and tool container this harness owns,
// across all worker slots. A run at concurrency N creates a proxy and a tool
// container per slot, worker zero on the bare names and worker i on a -i suffix,
// so removing only worker zero (which the earlier cleanup did) leaves the higher
// slots behind whenever a concurrent sweep was killed mid-flight. Those orphans
// are detached containers that pin their writable layers and published ports,
// and they pile up run after run until the runtime's disk fills. Discovering the
// slots by name rather than by the current concurrency also catches a sweep that
// ran at a higher concurrency than the clean does. The match is anchored on the
// known roles so a co-resident harness under a longer prefix, say tomolab-mc
// beside tomolab, is never caught by the shorter prefix's clean.
func (l *Lab) orphanContainers(ctx context.Context) []string {
	return ownedSlots(l.cfg.NamePrefix, l.rt.Containers(ctx))
}

// ownedSlots filters container names to the proxy, web, and tool slots a harness
// with the given prefix owns, at any worker index.
func ownedSlots(prefix string, names []string) []string {
	slot := regexp.MustCompile("^" + regexp.QuoteMeta(prefix) + `-(proxy|web|run)(-\d+)?$`)
	var owned []string
	for _, name := range names {
		if slot.MatchString(name) {
			owned = append(owned, name)
		}
	}
	return owned
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
