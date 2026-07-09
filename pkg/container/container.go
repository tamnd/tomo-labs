// Package container is a thin, typed wrapper over the docker or podman CLI. The
// lab drives real containers, so rather than pull in a heavy SDK it shells out
// to whichever CLI is present and hands back Go errors with the command output
// attached. Every call takes a context, so a stuck build or run is bounded by
// the caller's deadline instead of hanging the whole harness.
package container

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// CLI is a resolved container command, either docker or podman. The whole lab
// uses the shared build/run/network/volume surface, so a run is identical
// whichever backs it.
type CLI struct {
	Bin string
}

// Detect picks the runtime once. LAB_RUNTIME overrides everything; otherwise
// docker wins when its daemon answers, and podman is the fallback.
func Detect(ctx context.Context) (*CLI, error) {
	if forced := os.Getenv("LAB_RUNTIME"); forced != "" {
		return &CLI{Bin: forced}, nil
	}
	if _, err := exec.LookPath("docker"); err == nil {
		if exec.CommandContext(ctx, "docker", "info").Run() == nil {
			return &CLI{Bin: "docker"}, nil
		}
	}
	if _, err := exec.LookPath("podman"); err == nil {
		return &CLI{Bin: "podman"}, nil
	}
	return nil, fmt.Errorf("no container runtime: install docker or podman, or set LAB_RUNTIME")
}

// Mount is a host path bound into a container, optionally read-only.
type Mount struct {
	Host      string
	Container string
	ReadOnly  bool
}

func (m Mount) arg() string {
	s := m.Host + ":" + m.Container
	if m.ReadOnly {
		s += ":ro"
	}
	return s
}

// RunSpec describes one container to start. It maps one-to-one onto the docker
// and podman run flags the lab needs and nothing more.
type RunSpec struct {
	Name    string
	Image   string
	Network string
	Mounts  []Mount
	Env     []string // "KEY=VALUE", order preserved
	Publish string   // host:container, e.g. "127.0.0.1:8899:8080"
	Workdir string
	Cmd     []string // overrides the image entrypoint's default args
	Remove  bool     // --rm
	Detach  bool     // -d
	Stdout  *os.File // where a foreground run streams; nil discards
	Stderr  *os.File
}

func (c *CLI) runArgs(s RunSpec) []string {
	args := []string{"run", "--name", s.Name}
	if s.Remove {
		args = append(args, "--rm")
	}
	if s.Detach {
		args = append(args, "-d")
	}
	if s.Network != "" {
		args = append(args, "--network", s.Network)
	}
	for _, m := range s.Mounts {
		args = append(args, "-v", m.arg())
	}
	for _, e := range s.Env {
		args = append(args, "-e", e)
	}
	if s.Publish != "" {
		args = append(args, "-p", s.Publish)
	}
	if s.Workdir != "" {
		args = append(args, "-w", s.Workdir)
	}
	args = append(args, s.Image)
	args = append(args, s.Cmd...)
	return args
}

// Run starts a container and, unless it is detached, waits for it to exit. A
// foreground run streams its output to the spec's Stdout/Stderr so the operator
// watches the agent work in real time; a detached run returns as soon as the
// container is up.
func (c *CLI) Run(ctx context.Context, s RunSpec) error {
	cmd := exec.CommandContext(ctx, c.Bin, c.runArgs(s)...)
	if s.Detach {
		var out bytes.Buffer
		cmd.Stdout, cmd.Stderr = &out, &out
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("%s run %s: %w: %s", c.Bin, s.Name, err, out.String())
		}
		return nil
	}
	cmd.Stdout, cmd.Stderr = s.Stdout, s.Stderr
	// A non-zero exit from the tool container is not a harness error: the tool
	// simply failed the task, and the checker grades that. Only a failure to
	// launch is an error, which surfaces as a non-ExitError.
	if err := cmd.Run(); err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			return nil
		}
		return fmt.Errorf("%s run %s: %w", c.Bin, s.Name, err)
	}
	return nil
}

// Remove force-deletes a container by name, ignoring the case where it is not
// there, so a crashed prior run never blocks the next one.
func (c *CLI) Remove(ctx context.Context, name string) {
	_ = exec.CommandContext(ctx, c.Bin, "rm", "-f", name).Run()
}

// EnsureNetwork creates the lab network if it does not already exist.
func (c *CLI) EnsureNetwork(ctx context.Context, name string) error {
	if exec.CommandContext(ctx, c.Bin, "network", "inspect", name).Run() == nil {
		return nil
	}
	out, err := exec.CommandContext(ctx, c.Bin, "network", "create", name).CombinedOutput()
	if err != nil {
		return fmt.Errorf("network create %s: %w: %s", name, err, out)
	}
	return nil
}

// Build builds an image from a context directory. A non-default Dockerfile and
// build args are optional; output streams to the given writer so a long build
// is visible.
type BuildSpec struct {
	Tag        string
	Context    string
	Dockerfile string            // empty means <context>/Dockerfile
	Args       map[string]string // --build-arg
	Out        *os.File
}

func (c *CLI) Build(ctx context.Context, s BuildSpec) error {
	args := []string{"build", "-t", s.Tag}
	if s.Dockerfile != "" {
		args = append(args, "-f", s.Dockerfile)
	}
	for k, v := range s.Args {
		args = append(args, "--build-arg", k+"="+v)
	}
	args = append(args, s.Context)
	cmd := exec.CommandContext(ctx, c.Bin, args...)
	cmd.Stdout, cmd.Stderr = s.Out, s.Out
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("build %s: %w", s.Tag, err)
	}
	return nil
}

// ImageExists reports whether an image is present locally.
func (c *CLI) ImageExists(ctx context.Context, ref string) bool {
	return exec.CommandContext(ctx, c.Bin, "image", "inspect", ref).Run() == nil
}

// ImageSize returns an image's on-disk size in bytes, or 0 if it is gone.
func (c *CLI) ImageSize(ctx context.Context, ref string) int64 {
	out, err := exec.CommandContext(ctx, c.Bin, "image", "inspect", ref, "--format", "{{.Size}}").Output()
	if err != nil {
		return 0
	}
	n, _ := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
	return n
}

// Output runs the runtime with the given args and returns its stdout, for the
// short inspection commands the lab reads a value back from (an image's installed
// package version, a binary's build metadata).
func (c *CLI) Output(ctx context.Context, args ...string) (string, error) {
	out, err := exec.CommandContext(ctx, c.Bin, args...).Output()
	return string(out), err
}

// Logs returns a container's captured output, for surfacing why a sidecar never
// became ready.
func (c *CLI) Logs(ctx context.Context, name string) string {
	out, _ := exec.CommandContext(ctx, c.Bin, "logs", name).CombinedOutput()
	return string(out)
}

// PruneImages deletes dangling images, the untagged <none> layers a rebuild
// leaves behind when it retags a name onto fresh layers. The lab rebuilds the
// base, proxy, and every tool image over and over, so without this each rebuild
// orphans the previous copy and the machine's disk creeps up until it fills.
// Failure is not fatal: pruning is hygiene, not correctness, so a prune that
// errors on a busy machine should not sink a build or a run.
func (c *CLI) PruneImages(ctx context.Context) {
	_ = exec.CommandContext(ctx, c.Bin, "image", "prune", "-f").Run()
}
