package lab

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tamnd/tomo-labs/pkg/container"
)

// Canonical SWE-bench hands the agent a repository whose environment is already
// built: the pinned Python, the project installed, and its test dependencies
// present, all baked into the task image ahead of time. The agent only edits
// source and runs the tests. Our harness used to hand the agent a bare checkout
// on the shared base image, so every tool spent turns and tokens bootstrapping
// the interpreter and pip-installing the project and pytest before it could run
// a single test. That plumbing is not the task, it is not what the benchmark
// means to measure, and it taxes every tool the same undifferentiated way.
//
// prepEnv closes that gap. Before the agent container starts, it stands up the
// task's Python environment in a throwaway container built on the same base
// image, so the agent sees `python` and `pytest` already resolving to a venv
// with the project installed, exactly as canonical SWE-bench arranges.
//
// The layout is split across two mounts so the venv's interpreter still resolves
// once the agent container takes over:
//
//   - /opt/uv is shared across every task and run (the base image points uv's
//     managed-Python dir and wheel cache here). The pinned CPython a venv links
//     to lives under it, so warming it once serves later tasks offline, and the
//     interpreter path a venv bakes in stays valid in the agent container that
//     mounts the same dir.
//   - /opt/venv is the per-attempt virtualenv. The base image puts it first on
//     PATH and names it VIRTUAL_ENV, so any tool that shells out to `python` or
//     `pytest` lands in the prepared env without knowing this happened.
//
// It is best effort: a prep that cannot build the env logs and returns without
// failing the attempt, so a task whose environment resists provisioning is no
// harder for the agent than it was before, never harder. The grader keeps its
// own independent venv, so what prep installs can never color the verdict.
func (l *Lab) prepEnv(ctx context.Context, sc Scenario, work, envDir string, sl slot) {
	pyver := l.taskPython(sc)
	if pyver == "" {
		return // not a Python task with a pinned interpreter; nothing to prepare
	}
	if err := os.MkdirAll(envDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "[prep] %s: %v\n", sc.Name, err)
		return
	}
	if err := os.MkdirAll(l.uvCacheDir(), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "[prep] %s: %v\n", sc.Name, err)
		return
	}
	name := sl.prep
	l.rt.Remove(ctx, name)
	fmt.Fprintf(os.Stderr, "[prep] %s: building python %s env\n", sc.Name, pyver)
	// The prep container mounts /work at the same path the agent container will,
	// so the editable install it builds bakes in a /work interpreter path that
	// still resolves once the agent takes over the same tree.
	mounts := append(l.envMounts(envDir), container.Mount{Host: work, Container: "/work"})
	// A suite may declare the third-party libraries its tasks lean on (terminal-bench
	// writes them beside the task, the same allowlist the grader installs). Prep puts
	// them in the venv up front so the agent never spends a turn pip-installing what
	// the task needs, matching how the upstream benchmark ships a ready environment.
	// Cache and venv are separate bind mounts, so uv cannot hardlink between
	// them. Symlinks keep heavyweight wheels in the shared cache instead of
	// copying them into every throwaway attempt. Both mounts remain at these
	// stable paths until the agent exits, and only then is the venv removed.
	env := []string{"LAB_PYTHON=" + pyver, "UV_LINK_MODE=symlink"}
	if deps := l.taskPyDeps(sc); deps != "" {
		env = append(env, "LAB_PYDEPS="+deps)
	}
	err := l.rt.Run(ctx, container.RunSpec{
		Name: name, Image: baseImage, Network: l.cfg.Network, Remove: true,
		Mounts: mounts,
		Env:    env,
		Cmd:    []string{"bash", "-c", prepScript},
		Stdout: os.Stderr, Stderr: os.Stderr,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "[prep] %s: env prep failed, agent falls back to bootstrapping: %v\n", sc.Name, err)
	}
}

// envMounts are the two volumes the prepared environment lives on, shared by the
// prep container and the agent container so the venv the one builds is the venv
// the other runs under. The shared uv dir carries the managed interpreter and
// wheel cache; the per-attempt dir carries the venv itself.
func (l *Lab) envMounts(envDir string) []container.Mount {
	return []container.Mount{
		{Host: l.uvCacheDir(), Container: "/opt/uv"},
		{Host: envDir, Container: "/opt/venv"},
	}
}

// uvCacheDir is the host home for uv's managed Pythons and wheel cache, shared
// across tasks so the first task that needs an interpreter version pays the
// download and the rest reuse it. It sits beside the results rather than in any
// one run, since it outlives them all.
func (l *Lab) uvCacheDir() string {
	return filepath.Join(l.cfg.Data, "swebench-env")
}

// taskPython reads the interpreter version the task's hidden environment pins,
// from the oracle the grader also reads. An empty string means the task carries
// no pinned Python, which is every non-SWE-bench scenario, so prep is skipped.
func (l *Lab) taskPython(sc Scenario) string {
	b, err := os.ReadFile(filepath.Join(l.suiteDir(), "oracle", sc.Name, "python"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// taskPyDeps reads the third-party packages a task's grader and reference solution
// import, written beside the task by the generator (terminal-bench). Prep installs
// them into the venv so the agent starts with them present. It is a space-joined
// list for the shell; an absent or empty file means the task needs only stdlib.
func (l *Lab) taskPyDeps(sc Scenario) string {
	b, err := os.ReadFile(filepath.Join(l.suiteDir(), "oracle", sc.Name, "pydeps"))
	if err != nil {
		return ""
	}
	return strings.Join(strings.Fields(string(b)), " ")
}

// prepScript builds the venv under /opt/venv at the pinned interpreter and
// installs the project into it, preferring the normal dependency set before
// falling back to optional test extras, then makes sure pytest is present. The
// ordering matters for projects whose test extra pulls unrelated heavyweight
// stacks: the selected hidden test normally needs the project plus pytest, not
// every optional integration. It always exits zero: prep is an optimization, not
// a gate, so a failure leaves the agent exactly where it was before, free to
// bootstrap on its own. The interpreter and wheels resolve from the shared /opt/uv
// the base image pointed uv at, so a warm cache makes this fast and largely offline.
const prepScript = `set -uo pipefail
PYVER="${LAB_PYTHON:-3}"
LOG=/opt/venv/prep.log
if ! uv venv --clear --python "$PYVER" /opt/venv >"$LOG" 2>&1; then
  echo "[prep] could not build a python $PYVER venv" >&2
  exit 0
fi
PY=/opt/venv/bin/python
set -f
for spec in "-e ." "." "-e .[test]" "-e .[tests]" "-e .[dev]"; do
  if ( cd /work && uv pip install --python "$PY" -q $spec ) >>"$LOG" 2>&1; then
    break
  fi
done
set +f
"$PY" -c "import pytest" >/dev/null 2>&1 || uv pip install --python "$PY" -q pytest >>"$LOG" 2>&1
# The suite's declared third-party libraries, installed up front so the agent finds
# them already importable instead of discovering the closed network the hard way.
if [ -n "${LAB_PYDEPS:-}" ]; then
  uv pip install --python "$PY" -q $LAB_PYDEPS >>"$LOG" 2>&1 || echo "[prep] some pydeps did not install" >&2
fi
echo "[prep] python $PYVER env ready" >&2
exit 0
`
