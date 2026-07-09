package lab

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStripCaches(t *testing.T) {
	work := t.TempDir()
	// The artifact the agent produced, which must survive.
	mkfile(t, filepath.Join(work, "main.go"), "package main")
	mkfile(t, filepath.Join(work, "bin", "prog"), "ELF")
	// Regenerable caches, which must not.
	mkfile(t, filepath.Join(work, ".cache", "go-build", "ab", "obj"), "junk")
	mkfile(t, filepath.Join(work, "node_modules", "left-pad", "index.js"), "junk")
	mkfile(t, filepath.Join(work, "pkg", "__pycache__", "m.pyc"), "junk")

	stripCaches(work)

	for _, keep := range []string{"main.go", filepath.Join("bin", "prog")} {
		if !exists(filepath.Join(work, keep)) {
			t.Errorf("stripCaches removed the artifact %s", keep)
		}
	}
	for _, gone := range []string{".cache", "node_modules", filepath.Join("pkg", "__pycache__")} {
		if exists(filepath.Join(work, gone)) {
			t.Errorf("stripCaches left the cache %s behind", gone)
		}
	}
}

func TestPruneOldRuns(t *testing.T) {
	dir := t.TempDir()
	// Timestamps sort chronologically, oldest first.
	stamps := []string{
		"20260101T000000Z", "20260102T000000Z", "20260103T000000Z",
		"20260104T000000Z", "20260105T000000Z",
	}
	for _, s := range stamps {
		mkfile(t, filepath.Join(dir, s, "result.json"), "{}")
	}

	pruneOldRuns(dir, 2)

	for _, gone := range stamps[:3] {
		if exists(filepath.Join(dir, gone)) {
			t.Errorf("pruneOldRuns kept %s, wanted it dropped", gone)
		}
	}
	for _, keep := range stamps[3:] {
		if !exists(filepath.Join(dir, keep)) {
			t.Errorf("pruneOldRuns dropped %s, wanted it kept", keep)
		}
	}
}

func TestPruneOldRunsKeepZeroKeepsAll(t *testing.T) {
	dir := t.TempDir()
	for _, s := range []string{"20260101T000000Z", "20260102T000000Z"} {
		mkfile(t, filepath.Join(dir, s, "result.json"), "{}")
	}
	pruneOldRuns(dir, 0)
	if got := len(mustReadDir(t, dir)); got != 2 {
		t.Errorf("keep 0 pruned to %d dirs, wanted 2", got)
	}
}

func mkfile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustReadDir(t *testing.T, dir string) []os.DirEntry {
	t.Helper()
	e, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	return e
}
