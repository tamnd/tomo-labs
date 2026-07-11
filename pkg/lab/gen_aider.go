package lab

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// The Aider polyglot benchmark is a set of Exercism practice exercises across six
// languages. Each exercise ships a stub the solver fills in, a test suite that
// grades it, and a reference solution under .meta/. genAider renders each one into
// the lab task shape, so the harness grades an Aider task exactly like a
// hand-written scenario.
//
// It runs the languages the shared base image already carries a toolchain for: Go
// (go test) and Python (unittest, standard library, no pytest). Rust, Java, C++,
// and JavaScript need toolchains the base does not ship, so they are left out
// until the base grows them.
//
// Exercism deliberately gives the solver the tests, so the test files are laid
// into the work tree and it is not a leak that they sit in the task dir. The
// reference solution is different: it is kept under answers/, which the harness
// never mounts, so a task can be proven correct without ever showing the answer to
// an agent.

const (
	aiderRepo = "Aider-AI/polyglot-benchmark"
	aiderRaw  = "https://raw.githubusercontent.com/" + aiderRepo + "/main"
	aiderAPI  = "https://api.github.com/repos/" + aiderRepo
)

// aiderLangs is the set genAider knows how to grade. defaultLimit bounds the
// per-language sample when the caller does not ask for all of them.
var aiderLangs = map[string]bool{"go": true, "python": true}

// aiderConfig is the slice of .meta/config.json a generator needs: which files are
// the stub, the tests, the reference, and any support files.
type aiderConfig struct {
	Blurb string `json:"blurb"`
	Files struct {
		Solution    []string `json:"solution"`
		Test        []string `json:"test"`
		Example     []string `json:"example"`
		Editor      []string `json:"editor"`
		Invalidator []string `json:"invalidator"`
	} `json:"files"`
}

// aiderExercise holds one fetched exercise, ready to write.
type aiderExercise struct {
	lang, name, blurb, instructions string
	solution, tests                 []string          // work-tree file paths the agent edits / must pass
	work                            map[string][]byte // every file laid into the work tree
	answer                          map[string][]byte // reference solution, kept out of the work tree
}

func (l *Lab) genAider(ctx context.Context, opts GenOptions) (int, error) {
	langs := opts.Langs
	if len(langs) == 0 {
		langs = []string{"go", "python"}
	}
	limit := opts.Limit
	if limit <= 0 {
		limit = 5
	}

	kept, dropped := 0, 0
	for _, lang := range langs {
		if !aiderLangs[lang] {
			fmt.Fprintf(os.Stderr, "  unsupported language: %s\n", lang)
			continue
		}
		names, err := aiderList(ctx, lang)
		if err != nil {
			return kept, err
		}
		if !opts.All && len(names) > limit {
			names = names[:limit]
		}
		for _, name := range names {
			ex, err := l.aiderRender(ctx, lang, name)
			if err != nil {
				fmt.Fprintf(os.Stderr, "  skip %s/%s: %v\n", lang, name, err)
				dropped++
				continue
			}
			task, ans, err := l.aiderMaterialize(ex)
			if err != nil {
				return kept, err
			}
			if opts.NoValidate {
				fmt.Printf("  wrote %s\n", filepath.Base(task))
				kept++
				continue
			}
			ok, out, err := validateTask(ctx, task, func(work string) error {
				return overlay(ans, work)
			})
			if err != nil {
				return kept, err
			}
			if ok {
				fmt.Printf("  ok   %s\n", filepath.Base(task))
				kept++
			} else {
				fmt.Fprintf(os.Stderr, "  DROP %s: reference solution failed check\n%s\n",
					filepath.Base(task), trim(out, 400))
				os.RemoveAll(task)
				os.RemoveAll(ans)
				dropped++
			}
		}
	}
	fmt.Printf("\naider: kept %d, dropped %d\n", kept, dropped)
	return kept, nil
}

// aiderList returns the practice exercise names for a language track.
func aiderList(ctx context.Context, lang string) ([]string, error) {
	var entries []struct {
		Name string `json:"name"`
		Type string `json:"type"`
	}
	url := fmt.Sprintf("%s/contents/%s/exercises/practice", aiderAPI, lang)
	if err := httpGetJSON(ctx, url, &entries); err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if e.Type == "dir" {
			names = append(names, e.Name)
		}
	}
	sort.Strings(names)
	return names, nil
}

// aiderRender fetches one exercise and everything needed to write it, or an error
// if it is incomplete.
func (l *Lab) aiderRender(ctx context.Context, lang, name string) (*aiderExercise, error) {
	root := fmt.Sprintf("%s/%s/exercises/practice/%s", aiderRaw, lang, name)
	var cfg aiderConfig
	if err := httpGetJSON(ctx, root+"/.meta/config.json", &cfg); err != nil {
		return nil, fmt.Errorf("no config: %w", err)
	}
	f := cfg.Files
	if len(f.Solution) == 0 || len(f.Test) == 0 || len(f.Example) == 0 {
		return nil, fmt.Errorf("incomplete config")
	}

	// The work-tree files: the stub the agent edits, the tests it must pass, and
	// any support files (go.mod, extra test data). All come from the exercise root,
	// which holds the stub versions.
	work := map[string][]byte{}
	for _, rel := range concat(f.Solution, f.Test, f.Editor, f.Invalidator) {
		data, err := httpGet(ctx, root+"/"+rel)
		if err != nil {
			return nil, fmt.Errorf("missing %s: %w", rel, err)
		}
		work[rel] = data
	}

	// The reference solution overlays the stub file(s); config maps each example
	// file to a solution slot by position.
	answer := map[string][]byte{}
	for i, rel := range f.Example {
		data, err := httpGet(ctx, root+"/"+rel)
		if err != nil {
			return nil, fmt.Errorf("missing example %s: %w", rel, err)
		}
		slot := f.Solution[min(i, len(f.Solution)-1)]
		answer[slot] = data
	}

	instructions := cfg.Blurb
	if b, err := httpGet(ctx, root+"/.docs/instructions.md"); err == nil {
		instructions = string(b)
	}

	return &aiderExercise{
		lang: lang, name: name, blurb: cfg.Blurb, instructions: instructions,
		solution: f.Solution, tests: f.Test, work: work, answer: answer,
	}, nil
}

// aiderTestCommand is what check.sh runs in the work tree; it must exit zero
// exactly when the solution is correct. Go runs the whole package; Python runs the
// test modules by name so no pytest is needed.
func aiderTestCommand(ex *aiderExercise) string {
	if ex.lang == "go" {
		return "go test ./..."
	}
	var mods []string
	for _, t := range ex.tests {
		mods = append(mods, strings.TrimSuffix(filepath.Base(t), ".py"))
	}
	return "python3 -m unittest " + strings.Join(mods, " ")
}

func (l *Lab) aiderMaterialize(ex *aiderExercise) (task, ans string, err error) {
	name := ex.lang + "-" + ex.name
	task = filepath.Join(l.tasksDir(), name)
	ans = filepath.Join(l.suiteDir(), "answers", name)
	os.RemoveAll(task)
	os.RemoveAll(ans)

	for rel, data := range ex.work {
		if err = writeFile(filepath.Join(task, "files", rel), data, 0o644); err != nil {
			return
		}
	}
	for rel, data := range ex.answer {
		if err = writeFile(filepath.Join(ans, rel), data, 0o644); err != nil {
			return
		}
	}

	cmd := aiderTestCommand(ex)
	prompt := fmt.Sprintf("%s\n\nImplement your solution in %s. The work tree already holds the "+
		"tests (%s); do not edit them. Your solution is correct when `%s` passes with no failures.\n",
		strings.TrimSpace(ex.instructions), strings.Join(ex.solution, ", "),
		strings.Join(ex.tests, ", "), cmd)
	if err = writeFile(filepath.Join(task, "prompt.txt"), []byte(prompt), 0o644); err != nil {
		return
	}
	blurb := ex.blurb
	if blurb == "" {
		blurb = ex.name
	}
	if err = writeFile(filepath.Join(task, "desc"), []byte(ex.lang+": "+blurb+"\n"), 0o644); err != nil {
		return
	}

	setup := "#!/usr/bin/env bash\n" +
		"# Lay the exercise stub and its tests into the work tree.\n" +
		"set -e\n" +
		"W=\"$1\"\n" +
		"cp -R \"$(dirname \"$0\")/files/.\" \"$W/\"\n"
	if err = writeFile(filepath.Join(task, "setup.sh"), []byte(setup), 0o755); err != nil {
		return
	}
	check := "#!/usr/bin/env bash\n" +
		"# Pass when the exercise's own test suite is green.\n" +
		"W=\"$1\"\n" +
		"cd \"$W\" && " + cmd + "\n"
	if err = writeFile(filepath.Join(task, "check.sh"), []byte(check), 0o755); err != nil {
		return
	}
	return task, ans, nil
}
