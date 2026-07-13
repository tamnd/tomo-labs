package codex

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Home returns the Codex home directory: $CODEX_HOME if set, else ~/.codex.
// This is where the CLI keeps sessions/, models_cache.json, and config.
func Home() string {
	if h := os.Getenv("CODEX_HOME"); h != "" {
		return h
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".codex"
	}
	return filepath.Join(home, ".codex")
}

// CatalogPath is the models cache path under a Codex home.
func CatalogPath(home string) string {
	return filepath.Join(home, "models_cache.json")
}

// FindRollouts returns every rollout JSONL file under home's sessions tree,
// newest first by filename. The filenames start rollout-<timestamp>, so a
// reverse lexical sort is newest first without opening any file.
func FindRollouts(home string) ([]string, error) {
	root := filepath.Join(home, "sessions")
	var paths []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// A missing sessions tree is not fatal: return no rollouts, not an
			// error, so a fresh Codex install reads as empty.
			if os.IsNotExist(err) {
				return filepath.SkipDir
			}
			return err
		}
		if d.IsDir() {
			return nil
		}
		name := d.Name()
		if strings.HasPrefix(name, "rollout-") && strings.HasSuffix(name, ".jsonl") {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	sort.Sort(sort.Reverse(sort.StringSlice(paths)))
	return paths, nil
}

// LatestRollout returns the newest rollout under home, or ok=false when the
// sessions tree holds none.
func LatestRollout(home string) (path string, ok bool, err error) {
	paths, err := FindRollouts(home)
	if err != nil {
		return "", false, err
	}
	if len(paths) == 0 {
		return "", false, nil
	}
	return paths[0], true, nil
}
