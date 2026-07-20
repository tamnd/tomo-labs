package lab

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Config is everything a Lab needs to build images and run scenarios. Every
// field has an environment fallback so the binary can build a Config with
// DefaultConfig and an embedder can set fields directly.
type Config struct {
	Root     string // repo root holding scenarios/ and tools/
	Data     string // where traces and results land, per tool/scenario/timestamp
	Suite    string // empty runs the core scenarios/; a name runs evals/<name>/tasks/ as a separate tier
	Model    string // bare upstream model id
	Upstream string // OpenAI-compatible base the proxy forwards to
	APIKey   string // upstream key, forwarded to the tool, never written to a trace

	MaxTurns    int // agent turn budget handed to the tool
	Attempts    int // capability tries before a scenario is called failed; 1 is pure pass@1, higher is best-of-N
	AttemptSecs int // per-attempt wall-clock ceiling; a tool that runs past it is killed and its partial work graded, 0 disables the bound
	PrepSecs    int // task-environment preparation ceiling; a timeout falls back to the bare checkout, 0 disables the bound
	ProxyPort   int // host port the proxy publishes for the readiness probe (worker 0); later workers take the next ports
	KeepRuns    int // how many timestamped runs to keep per tool/scenario, 0 keeps all
	Concurrency int // how many tool/scenario runs to keep in flight at once

	Network    string // container network name
	NamePrefix string // prefix for the proxy, web, and run container names
	Isolate    bool   // run the tool on a no-egress internal network so it cannot fetch an answer; the proxy bridges to the model

	// Passthrough runs the trace proxy as a raw tap that never rewrites a request,
	// for a tool talking its own wire to its own backend (real codex on the ChatGPT
	// subscription, reached through the bridge). The default translate mode rewrites
	// a Responses request to chat, which strips the tools codex leaves the backend
	// to inject; passthrough forwards codex's request verbatim so it behaves as it
	// would on its own subscription, and the tap still records every request and
	// response. Set by LAB_PASSTHROUGH.
	Passthrough bool
}

const (
	baseImage  = "tomolab-base"
	proxyImage = "tomolab-proxy"
	toolPrefix = "tomolab-tool-"
)

// proxyName, webName, and runName are the container names a run owns, derived
// from NamePrefix so a second harness process can take a different prefix and
// run alongside the first without colliding on a name. Container names are
// unique per machine, not per network, so varying the network alone is not
// enough; the names have to differ too. The default prefix keeps the bare
// tomolab-* names a single harness always used.
func (c Config) proxyName() string { return c.NamePrefix + "-proxy" }
func (c Config) webName() string   { return c.NamePrefix + "-web" }
func (c Config) runName() string   { return c.NamePrefix + "-run" }

// DefaultConfig reads the environment and fills in defaults, matching the knobs
// the shell harness used so a run reproduces whichever front end starts it.
func DefaultConfig() Config {
	return Config{
		Root:        findRoot(),
		Data:        env("LAB_DATA", filepath.Join(home(), "data", "tomo-labs")),
		Model:       env("LAB_MODEL", "deepseek-v4-flash-free"),
		Upstream:    env("LAB_UPSTREAM", "https://opencode.ai/zen"),
		APIKey:      os.Getenv("OPENCODE_API_KEY"),
		MaxTurns:    envInt("LAB_MAX_TURNS", 12),
		Attempts:    envInt("LAB_ATTEMPTS", 1),
		AttemptSecs: envInt("LAB_ATTEMPT_TIMEOUT", 900),
		PrepSecs:    envInt("LAB_PREP_TIMEOUT", 300),
		ProxyPort:   envInt("LAB_PROXY_PORT", 8899),
		KeepRuns:    envInt("LAB_KEEP_RUNS", 5),
		Concurrency: envInt("LAB_CONCURRENCY", 3),
		Network:     env("LAB_NETWORK", "tomolab"),
		NamePrefix:  env("LAB_NAME_PREFIX", "tomolab"),
		Isolate:     envBool("LAB_ISOLATE", true),
		Passthrough: envBool("LAB_PASSTHROUGH", false),
	}
}

// internalNetwork is the no-egress network the tool container runs on when
// isolation is enabled, derived from the egress network's name so a custom
// LAB_NETWORK carries its internal sibling along. The proxy bridges the two.
func (c Config) internalNetwork() string { return c.Network + "-internal" }

// findRoot locates the repo root by walking up from the working directory until
// it finds a scenarios/ dir, falling back to the working directory. LAB_ROOT
// overrides the search.
func findRoot() string {
	if r := os.Getenv("LAB_ROOT"); r != "" {
		return r
	}
	dir, err := os.Getwd()
	if err != nil {
		return "."
	}
	for {
		if st, err := os.Stat(filepath.Join(dir, "scenarios")); err == nil && st.IsDir() {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return mustWd()
		}
		dir = parent
	}
}

func mustWd() string {
	d, err := os.Getwd()
	if err != nil {
		return "."
	}
	return d
}

func home() string {
	if h, err := os.UserHomeDir(); err == nil {
		return h
	}
	return "."
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func envInt(k string, def int) int {
	if v := os.Getenv(k); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

// envBool reads a boolean knob. An unset variable keeps the default, so isolation
// stays on unless a run explicitly sets LAB_ISOLATE=0 to debug a tool that needs
// the open network.
func envBool(k string, def bool) bool {
	v := os.Getenv(k)
	if v == "" {
		return def
	}
	switch strings.ToLower(v) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	}
	return def
}
