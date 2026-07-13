package lab

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestSweStripFutureRemovesTheGoldFix builds a repo the way an instance's really
// looks: a base commit the bug is reported at, then a later commit on another
// branch that carries the fix, plus a tag on each. It clones that into a work
// tree the way setup.sh does, checks out the base, and runs the shared strip
// step. Afterward the fix commit must be gone from the work tree while the base
// and its own tag survive, which is the whole point: a tool cannot git-log the
// answer.
func TestSweStripFutureRemovesTheGoldFix(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not on PATH")
	}

	up := t.TempDir() // the upstream repo, standing in for github

	// Run every git off a clean config, so the host's global settings (signed tags,
	// hooks, aliases) cannot change what the test builds or how the strip prunes.
	hermetic := append(os.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	git := func(dir string, args ...string) string {
		t.Helper()
		c := exec.Command("git", args...)
		c.Dir = dir
		c.Env = hermetic
		out, err := c.CombinedOutput()
		if err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
		return strings.TrimSpace(string(out))
	}

	write := func(path, data string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	git(up, "init", "-q", "-b", "main")
	write(filepath.Join(up, "app.py"), "x = 1\n")
	git(up, "add", "-A")
	git(up, "commit", "-qm", "base")
	base := git(up, "rev-parse", "HEAD")
	git(up, "tag", "v1.0") // an ancestor tag, must survive so version detection works

	// The fix lands later, on its own branch, with its own release tag: exactly the
	// shape that let a real run diff base..fix and apply it.
	git(up, "checkout", "-q", "-b", "fix")
	write(filepath.Join(up, "app.py"), "x = 2\n")
	git(up, "add", "-A")
	git(up, "commit", "-qm", "fix: the gold patch")
	fix := git(up, "rev-parse", "HEAD")
	git(up, "tag", "v1.1")
	git(up, "checkout", "-q", "main")

	// Clone into the work tree the way setup.sh does, then land on the base commit.
	work := filepath.Join(t.TempDir(), "work")
	git("", "clone", "-q", "--no-hardlinks", up, work)
	git(work, "checkout", "-q", base)

	// The fix is reachable before the strip, or the test would prove nothing.
	if err := exec.Command("git", "-C", work, "cat-file", "-e", fix).Run(); err != nil {
		t.Fatalf("precondition: fix commit should be reachable before strip: %v", err)
	}

	// Run the exact snippet setup.sh runs.
	c := exec.Command("bash", "-c", sweStripFuture)
	c.Env = append(hermetic, "W="+work, "SHA="+base)
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatalf("strip: %v\n%s", err, out)
	}

	// The fix commit and its future tag are gone; the base and its tag remain.
	if err := exec.Command("git", "-C", work, "cat-file", "-e", fix).Run(); err == nil {
		t.Error("fix commit still reachable after strip: the answer leaked")
	}
	if err := exec.Command("git", "-C", work, "cat-file", "-e", base).Run(); err != nil {
		t.Errorf("base commit should survive the strip: %v", err)
	}
	tags := git(work, "tag")
	if strings.Contains(tags, "v1.1") {
		t.Errorf("future tag v1.1 should be deleted, tags = %q", tags)
	}
	if !strings.Contains(tags, "v1.0") {
		t.Errorf("ancestor tag v1.0 should survive for version detection, tags = %q", tags)
	}
	// The working tree still holds the base content, unchanged.
	got, err := os.ReadFile(filepath.Join(work, "app.py"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "x = 1\n" {
		t.Errorf("work tree content = %q, want the base version", got)
	}
}

// TestCommittedSetupMatchesTemplate guards the checked-in task setup.sh files
// against drifting from the sweSetup template. gen writes a copy per task, so a
// change to the template that is not regenerated would leave stale, leaky copies
// on disk. Every committed setup.sh that clones a repo must equal the template
// byte for byte.
func TestCommittedSetupMatchesTemplate(t *testing.T) {
	matches, err := filepath.Glob("../../evals/*/tasks/*/setup.sh")
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, path := range matches {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		// Only the swebench tiers use the clone template; other suites set up
		// differently and are not the concern here.
		if !strings.Contains(string(b), "no-hardlinks") {
			continue
		}
		checked++
		if string(b) == sweSetup {
			continue
		}
		// REGEN=1 rewrites the committed copies from the template, the golden-file
		// refresh path for when sweSetup itself changes.
		if os.Getenv("REGEN") != "" {
			if err := os.WriteFile(path, []byte(sweSetup), 0o755); err != nil {
				t.Fatal(err)
			}
			continue
		}
		t.Errorf("%s has drifted from sweSetup; rerun with REGEN=1 to refresh", path)
	}
	if checked == 0 {
		t.Skip("no committed swebench task setup.sh found")
	}
}
