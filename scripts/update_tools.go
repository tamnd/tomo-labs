//go:build ignore

// update_tools bumps each wired tool to its newest upstream release, betas
// included, by rewriting the version ARG in the tool's Dockerfile.
//
// Every tool image pins its version in one line of its Dockerfile:
//
//	ARG CODEX_VERSION=latest
//	RUN npm install -g @openai/codex@${CODEX_VERSION}
//
// This resolves the newest version for each tool and rewrites that ARG default
// in place. "Newest" means the most recently published version across a tool's
// real release channels, so a beta, alpha, nightly, or preview wins when it is
// newer than the stable line. Branch-snapshot builds that carry the placeholder
// version 0.0.0 are skipped, since they are ephemeral CI artifacts rather than
// releases. npm tools resolve against the npm registry; the Go-installed tomo
// tracks its main branch through the Go module proxy. The tool only reads the
// network and rewrites Dockerfiles, so it is safe to run in CI with no
// container runtime.
//
// Run it with no arguments to update every tool, or name tools to limit it:
//
//	go run scripts/update_tools.go
//	go run scripts/update_tools.go codex gemini-cli
//
// Exit code is 0 whether or not anything changed. With -summary it also writes a
// markdown table of the changes to the given file, for a pull request body.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// channels are the release tags worth considering; the newest published version
// among them wins. Platform tags (linux-x64), branch snapshots (snapshot-*), and
// one-off tags are ignored by not being listed here.
var channels = map[string]bool{
	"latest": true, "stable": true, "next": true, "beta": true,
	"alpha": true, "rc": true, "canary": true, "nightly": true,
	"preview": true, "dev": true, "edge": true, "insiders": true,
}

var argRe = regexp.MustCompile(`(?m)^ARG\s+(\w+_VERSION)=(.*)$`)

func getJSON(url string, dst any) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: %s", url, resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(dst)
}

// newestNPM returns the newest version of an npm package across its release
// channels, judged by publish time so a fresh beta beats an older stable.
func newestNPM(pkg string) (string, error) {
	var doc struct {
		DistTags map[string]string `json:"dist-tags"`
		Time     map[string]string `json:"time"`
	}
	url := "https://registry.npmjs.org/" + strings.ReplaceAll(pkg, "/", "%2F")
	if err := getJSON(url, &doc); err != nil {
		return "", err
	}
	// Sort the channel names for a stable walk, then keep the version with the
	// latest publish timestamp. ISO-8601 strings compare lexically as time.
	tags := make([]string, 0, len(doc.DistTags))
	for tag := range doc.DistTags {
		tags = append(tags, tag)
	}
	sort.Strings(tags)
	var bestVer, bestTime string
	for _, tag := range tags {
		if !channels[tag] {
			continue
		}
		ver := doc.DistTags[tag]
		if strings.HasPrefix(ver, "0.0.0-") { // ephemeral branch snapshot
			continue
		}
		if when := doc.Time[ver]; when > bestTime {
			bestVer, bestTime = ver, when
		}
	}
	if bestVer == "" {
		return "", fmt.Errorf("no release-channel version found for %s", pkg)
	}
	return bestVer, nil
}

// newestGoMain returns the version the module's main branch currently resolves
// to, through the Go module proxy.
func newestGoMain(installPath string) (string, error) {
	parts := strings.Split(installPath, "/")
	if len(parts) < 3 {
		return "", fmt.Errorf("cannot derive module root from %s", installPath)
	}
	root := strings.Join(parts[:3], "/")
	var info struct{ Version string }
	if err := getJSON("https://proxy.golang.org/"+root+"/@v/main.info", &info); err != nil {
		return "", err
	}
	return info.Version, nil
}

// newestPyPI returns the newest stable version of a PyPI package, as the
// registry reports it in the package's info block.
func newestPyPI(pkg string) (string, error) {
	var doc struct {
		Info struct {
			Version string `json:"version"`
		} `json:"info"`
	}
	if err := getJSON("https://pypi.org/pypi/"+pkg+"/json", &doc); err != nil {
		return "", err
	}
	if doc.Info.Version == "" {
		return "", fmt.Errorf("no version found for %s", pkg)
	}
	return doc.Info.Version, nil
}

// tool is one Dockerfile's version pin: the ARG name, its current value, whether
// it installs from npm, pip, or go, and the package or install path it references.
type tool struct {
	name, arg, current, kind, target string
}

func parseTool(name, dockerfile string) (tool, bool) {
	b, err := os.ReadFile(dockerfile)
	if err != nil {
		return tool{}, false
	}
	text := string(b)
	m := argRe.FindStringSubmatch(text)
	if m == nil {
		return tool{}, false
	}
	arg, current := m[1], strings.TrimSpace(m[2])
	ref := regexp.QuoteMeta("${" + arg + "}")
	if npm := regexp.MustCompile(`npm install[^\n]*?(\S+)@` + ref).FindStringSubmatch(text); npm != nil {
		return tool{name, arg, current, "npm", npm[1]}, true
	}
	if goi := regexp.MustCompile(`go install\s+(\S+)@` + ref).FindStringSubmatch(text); goi != nil {
		return tool{name, arg, current, "go", goi[1]}, true
	}
	if pip := regexp.MustCompile(`pip3?\s+install[^\n]*?(\S+)==` + ref).FindStringSubmatch(text); pip != nil {
		return tool{name, arg, current, "pip", pip[1]}, true
	}
	return tool{}, false
}

func rewriteArg(dockerfile, arg, newVer string) error {
	b, err := os.ReadFile(dockerfile)
	if err != nil {
		return err
	}
	line := regexp.MustCompile(`(?m)^ARG\s+` + arg + `=.*$`)
	out := line.ReplaceAllString(string(b), "ARG "+arg+"="+newVer)
	return os.WriteFile(dockerfile, []byte(out), 0o644)
}

func main() {
	root := flag.String("root", ".", "repo root holding tools/")
	summary := flag.String("summary", "", "write a markdown change table to this file")
	flag.Parse()

	toolsDir := filepath.Join(*root, "tools")
	names := flag.Args()
	if len(names) == 0 {
		entries, err := os.ReadDir(toolsDir)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		for _, e := range entries {
			if e.IsDir() && e.Name() != "base" {
				if _, err := os.Stat(filepath.Join(toolsDir, e.Name(), "Dockerfile")); err == nil {
					names = append(names, e.Name())
				}
			}
		}
		sort.Strings(names)
	}

	type row struct {
		name, from, to string
		moved          bool
	}
	var rows []row
	changed := false
	for _, name := range names {
		dockerfile := filepath.Join(toolsDir, name, "Dockerfile")
		t, ok := parseTool(name, dockerfile)
		if !ok {
			fmt.Fprintf(os.Stderr, "%s: no version arg found, skipping\n", name)
			continue
		}
		var newVer string
		var err error
		switch t.kind {
		case "go":
			newVer, err = newestGoMain(t.target)
		case "pip":
			newVer, err = newestPyPI(t.target)
		default:
			newVer, err = newestNPM(t.target)
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", name, err)
			os.Exit(1)
		}
		moved := newVer != t.current
		changed = changed || moved
		if moved {
			if err := rewriteArg(dockerfile, t.arg, newVer); err != nil {
				fmt.Fprintf(os.Stderr, "%s: %v\n", name, err)
				os.Exit(1)
			}
		}
		mark := "=="
		if moved {
			mark = "->"
		}
		fmt.Printf("%s: %s %s %s\n", name, t.current, mark, newVer)
		rows = append(rows, row{name, t.current, newVer, moved})
	}

	if *summary != "" {
		var b strings.Builder
		b.WriteString("| Tool | From | To |\n|---|---|---|\n")
		for _, r := range rows {
			to := r.to
			if r.moved {
				to = "**" + r.to + "**"
			}
			fmt.Fprintf(&b, "| %s | %s | %s |\n", r.name, r.from, to)
		}
		if err := os.WriteFile(*summary, []byte(b.String()), 0o644); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}

	if changed {
		fmt.Println("changed")
	} else {
		fmt.Println("no changes")
	}
}
