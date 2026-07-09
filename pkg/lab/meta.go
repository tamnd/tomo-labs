package lab

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// toolMeta is what a report needs to name the exact build of a tool it ran: the
// version and, where the source records it, the date that version was released.
// Both are properties of the tool image, not of any one run, so they are captured
// once per build and joined into the report the same way install size is.
type toolMeta struct {
	Tool     string `json:"tool"`
	Version  string `json:"version,omitempty"`
	Released string `json:"released,omitempty"` // YYYY-MM-DD, the day the version shipped
}

// RefreshMeta probes every wired tool's built image and writes its version and
// release date under the data dir, without rebuilding or running the tool. It is
// how an existing set of images gets its rich metadata backfilled so a report can
// name what it compared; a build refreshes the same file for the tool it built.
func (l *Lab) RefreshMeta(ctx context.Context) error {
	tools, err := l.Tools()
	if err != nil {
		return err
	}
	for _, t := range tools {
		if !l.rt.ImageExists(ctx, toolPrefix+t) {
			continue
		}
		if err := l.captureToolMeta(ctx, t); err != nil {
			return err
		}
		m := l.toolMetaOf(t)
		fmt.Fprintf(os.Stderr, "[meta] %-12s %s (%s)\n", t, blankDash(m.Version), blankDash(m.Released))
	}
	return nil
}

// captureToolMeta probes one tool image and records its version and release date.
// A probe that comes up empty is not an error: the file is still written so the
// report has a row, it just carries a dash where a value could not be read.
func (l *Lab) captureToolMeta(ctx context.Context, tool string) error {
	m := l.probeToolMeta(ctx, tool)
	m.Tool = tool
	dir := filepath.Join(l.cfg.Data, tool)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "tool.json"), append(b, '\n'), 0o644)
}

// toolMetaOf reads a tool's saved version metadata, or a zero value if it has not
// been captured yet.
func (l *Lab) toolMetaOf(tool string) toolMeta {
	b, err := os.ReadFile(filepath.Join(l.cfg.Data, tool, "tool.json"))
	if err != nil {
		return toolMeta{Tool: tool}
	}
	var m toolMeta
	if json.Unmarshal(b, &m) != nil {
		return toolMeta{Tool: tool}
	}
	return m
}

// probeToolMeta reads the version and release date out of a tool's image. Most
// tools install from npm, where the registry records both the installed version
// and the day it was published; a tool built from Go source instead carries the
// facts in its embedded build info. The Dockerfile says which world a tool is in.
func (l *Lab) probeToolMeta(ctx context.Context, tool string) toolMeta {
	img := toolPrefix + tool
	dockerfile := filepath.Join(l.cfg.Root, "tools", tool, "Dockerfile")
	body, _ := os.ReadFile(dockerfile)

	if pkg := npmPackageOf(string(body)); pkg != "" {
		ver := l.npmInstalledVersion(ctx, img, pkg)
		return toolMeta{Version: ver, Released: l.npmReleaseDate(ctx, img, pkg, ver)}
	}
	if strings.Contains(string(body), "go install") {
		return l.goBuildMeta(ctx, img, tool)
	}
	return toolMeta{}
}

// npmInstallRe pulls the package name out of a global npm install line, keeping a
// scope but dropping the trailing @version (or @${VERSION} build arg).
var npmInstallRe = regexp.MustCompile(`npm install -g\s+(\S+)`)

// npmPackageOf returns the npm package a Dockerfile installs globally, or empty
// if it installs none. The version suffix after the last @ is stripped, so both
// hermes-agent@x and @google/gemini-cli@x resolve to the bare package name.
func npmPackageOf(dockerfile string) string {
	m := npmInstallRe.FindStringSubmatch(dockerfile)
	if m == nil {
		return ""
	}
	spec := m[1]
	if at := strings.LastIndex(spec, "@"); at > 0 {
		return spec[:at]
	}
	return spec
}

// npmInstalledVersion reads the version npm actually installed for a package in
// the image, which is the truth even when the Dockerfile pinned a floating tag.
func (l *Lab) npmInstalledVersion(ctx context.Context, img, pkg string) string {
	out, err := l.rt.Output(ctx, "run", "--rm", "--entrypoint", "npm", img, "ls", "-g", pkg, "--json")
	if err != nil && out == "" {
		return ""
	}
	var doc struct {
		Dependencies map[string]struct {
			Version string `json:"version"`
		} `json:"dependencies"`
	}
	if json.Unmarshal([]byte(out), &doc) != nil {
		return ""
	}
	return doc.Dependencies[pkg].Version
}

// npmReleaseDate asks the registry when a specific version was published and
// returns just the day. It needs network, and comes back empty if the lookup
// fails, so a version still shows even when its date could not be resolved.
func (l *Lab) npmReleaseDate(ctx context.Context, img, pkg, version string) string {
	if version == "" {
		return ""
	}
	out, err := l.rt.Output(ctx, "run", "--rm", "--entrypoint", "npm", img, "view", pkg+"@"+version, "time", "--json")
	if err != nil && out == "" {
		return ""
	}
	var times map[string]string
	if json.Unmarshal([]byte(out), &times) != nil {
		return ""
	}
	return dayOf(times[version])
}

// pseudoStampRe matches the 14-digit UTC timestamp a Go module pseudo-version
// carries, e.g. the 20260709115839 in v0.2.2-0.20260709115839-7658e29f4020.
var pseudoStampRe = regexp.MustCompile(`\.(\d{14})-`)

// goBuildMeta reads a Go-built tool's version and release date from the build
// info embedded in its binary. The module version is exact; the date comes from
// the VCS timestamp a pseudo-version encodes, which is the commit the image was
// built from. The binary lands at /usr/local/bin/<tool>, and the base image
// carries the Go toolchain that reads its build info back.
func (l *Lab) goBuildMeta(ctx context.Context, img, tool string) toolMeta {
	out, err := l.rt.Output(ctx, "run", "--rm", "--entrypoint", "go", img, "version", "-m", "/usr/local/bin/"+tool)
	if err != nil && out == "" {
		return toolMeta{}
	}
	var version string
	for line := range strings.SplitSeq(out, "\n") {
		f := strings.Fields(line)
		if len(f) >= 3 && f[0] == "mod" {
			version = f[2]
			break
		}
	}
	m := toolMeta{Version: version}
	if s := pseudoStampRe.FindStringSubmatch(version); s != nil {
		m.Released = s[1][:4] + "-" + s[1][4:6] + "-" + s[1][6:8]
	}
	return m
}

// dayOf trims an RFC3339 timestamp down to its date, or returns empty for an
// empty input.
func dayOf(ts string) string {
	if len(ts) < 10 {
		return ""
	}
	return ts[:10]
}

// blankDash renders an empty string as a dash for display.
func blankDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
