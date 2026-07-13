package claude

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSlugForCwd(t *testing.T) {
	cases := map[string]string{
		"/Users/apple/x.y":                  "-Users-apple-x-y",
		"/work/dynaconf":                    "-work-dynaconf",
		"/Users/apple/data/tomo-labs/a_b.c": "-Users-apple-data-tomo-labs-a-b-c",
	}
	for cwd, want := range cases {
		if got := SlugForCwd(cwd); got != want {
			t.Errorf("SlugForCwd(%q) = %q, want %q", cwd, got, want)
		}
	}
}

func TestFindSessionsNewestFirst(t *testing.T) {
	home := t.TempDir()
	proj := filepath.Join(ProjectsDir(home), "-work-dynaconf")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	old := filepath.Join(proj, "old.jsonl")
	newer := filepath.Join(proj, "new.jsonl")
	for _, p := range []string{old, newer, filepath.Join(proj, "notes.txt")} {
		if err := os.WriteFile(p, []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// The filename is a UUID, not a timestamp, so ordering is by mtime: make old
	// older on disk than newer.
	past := time.Now().Add(-time.Hour)
	if err := os.Chtimes(old, past, past); err != nil {
		t.Fatal(err)
	}
	got, err := FindSessions(home)
	if err != nil {
		t.Fatalf("FindSessions: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("found %d sessions, want 2 (the .txt is skipped): %v", len(got), got)
	}
	if filepath.Base(got[0]) != "new.jsonl" {
		t.Errorf("newest first failed, got[0] = %s", got[0])
	}
}

func TestFindSessionsForCwd(t *testing.T) {
	home := t.TempDir()
	proj := filepath.Join(ProjectsDir(home), SlugForCwd("/work/dynaconf"))
	other := filepath.Join(ProjectsDir(home), SlugForCwd("/work/other"))
	for _, d := range []string{proj, other} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(proj, "a.jsonl"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(other, "b.jsonl"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := FindSessionsForCwd(home, "/work/dynaconf")
	if err != nil {
		t.Fatalf("FindSessionsForCwd: %v", err)
	}
	if len(got) != 1 || filepath.Base(got[0]) != "a.jsonl" {
		t.Errorf("got %v, want just the dynaconf session", got)
	}
}

func TestFindSessionsMissingTree(t *testing.T) {
	got, err := FindSessions(t.TempDir())
	if err != nil {
		t.Fatalf("FindSessions on empty home: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("found %d sessions in empty home, want 0", len(got))
	}
}

func TestLatestSession(t *testing.T) {
	home := t.TempDir()
	if _, ok, err := LatestSession(home); err != nil || ok {
		t.Errorf("LatestSession on empty home = ok %v err %v, want false nil", ok, err)
	}
}
