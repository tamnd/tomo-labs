package claude

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Home returns the Claude Code config directory: $CLAUDE_CONFIG_DIR if set, else
// ~/.claude. This is where the CLI keeps projects/<cwd-slug>/<session>.jsonl.
func Home() string {
	if h := os.Getenv("CLAUDE_CONFIG_DIR"); h != "" {
		return h
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".claude"
	}
	return filepath.Join(home, ".claude")
}

// ProjectsDir is the projects tree under a Claude home, one subdirectory per
// working directory the CLI has run in.
func ProjectsDir(home string) string {
	return filepath.Join(home, "projects")
}

// SlugForCwd turns a working directory into the directory name Claude Code uses
// for it under projects/: the absolute path with every separator and dot replaced
// by a dash, so /Users/apple/x.y becomes -Users-apple-x-y. It lets a caller find
// the sessions for a known run directory without scanning them all.
func SlugForCwd(cwd string) string {
	r := strings.NewReplacer(string(filepath.Separator), "-", "/", "-", ".", "-", "_", "-")
	return r.Replace(cwd)
}

// FindSessions returns every session JSONL file under home's projects tree, newest
// first by modification time. A missing tree is not an error: a fresh install
// reads as empty.
func FindSessions(home string) ([]string, error) {
	return findUnder(ProjectsDir(home))
}

// FindSessionsForCwd returns the session files for one working directory, newest
// first, by resolving its slug under the projects tree.
func FindSessionsForCwd(home, cwd string) ([]string, error) {
	return findUnder(filepath.Join(ProjectsDir(home), SlugForCwd(cwd)))
}

func findUnder(root string) ([]string, error) {
	type entry struct {
		path string
		mod  int64
	}
	var found []entry
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return filepath.SkipDir
			}
			return err
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".jsonl") {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		found = append(found, entry{path, info.ModTime().UnixNano()})
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	// Newest first by modification time; the filename is a UUID, not a timestamp,
	// so a name sort would be meaningless here (unlike Codex rollouts).
	sort.Slice(found, func(i, j int) bool { return found[i].mod > found[j].mod })
	paths := make([]string, len(found))
	for i, e := range found {
		paths[i] = e.path
	}
	return paths, nil
}

// LatestSession returns the newest session under home, or ok=false when the
// projects tree holds none.
func LatestSession(home string) (path string, ok bool, err error) {
	paths, err := FindSessions(home)
	if err != nil {
		return "", false, err
	}
	if len(paths) == 0 {
		return "", false, nil
	}
	return paths[0], true, nil
}
