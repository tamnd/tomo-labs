// Command lab is the whole harness from the outside: build the images, run a
// tool over the scenarios, and report. Each run is one tool against one scenario
// in a throwaway container, with its LLM traffic routed through the trace proxy
// and every artifact and resource number captured under the data dir.
//
//	lab build [tool] [--no-cache]  build base, proxy, and tool images
//	lab run [tool] [scenario]   run all, or one tool, or one pair
//	lab -p "<prompt>" [tool...] run one ad-hoc prompt through every tool, or some
//	lab tools                   list wired tools
//	lab scenarios               list scenarios
//	lab prompts <tool> [scen]   extract the system prompts a tool sent, from its traces
//	lab meta                    capture each tool's version and release date
//	lab gen                     materialize a benchmark into the suite's tasks/
//	lab report [--json]         summarize captured runs
//	lab reparse                 recompute metrics of captured runs from their traces
//	lab clean                   remove lab containers and dangling images
//
// Any command that runs, lists, reports, or generates over tasks takes --suite
// <name> to work on a separate eval tier under evals/<name>/ instead of the core
// scenarios/, e.g. `lab run tomo --suite aider` or `lab gen --suite evalplus`.
// gen fetches a public benchmark and writes its tasks; it takes --limit, --all,
// --langs, --difficulty, and --no-validate after the command.
//
// It needs OPENCODE_API_KEY (or another OpenAI-compatible key, with LAB_UPSTREAM
// and LAB_MODEL pointed to match). All logic lives in pkg/lab; this is a thin
// front end so the same harness can be embedded as a library.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"slices"
	"strconv"
	"strings"
	"syscall"

	"github.com/tamnd/tomo-labs/pkg/lab"
	"github.com/tamnd/tomo-labs/pkg/lab/inspect"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	args := os.Args[1:]
	if len(args) == 0 {
		usage()
		os.Exit(2)
	}

	// --suite selects a task tier before anything else, since it changes both where
	// tasks are read from and where results land; pull it out of the args so the
	// positional command and its arguments read the same with or without it.
	suite, args := takeFlagValue(args, "--suite")
	cfg := lab.DefaultConfig()
	cfg.Suite = suite

	l, err := lab.New(ctx, cfg)
	if err != nil {
		die(err)
	}

	switch args[0] {
	case "build":
		only, noCache := "", false
		for _, a := range args[1:] {
			if a == "--no-cache" {
				noCache = true
			} else if only == "" && !strings.HasPrefix(a, "--") {
				only = a
			}
		}
		die(l.Build(ctx, only, noCache))
	case "run":
		die(cmdRun(ctx, l, arg(args, 1), arg(args, 2)))
	case "-p", "--prompt", "prompt":
		die(cmdPrompt(ctx, l, args[1:]))
	case "tools":
		die(cmdTools(l))
	case "scenarios":
		die(cmdScenarios(l))
	case "prompts":
		die(cmdPrompts(l, arg(args, 1), arg(args, 2), hasFlag(args, "--json"), hasFlag(args, "--brief")))
	case "inspect":
		die(cmdInspect(l, arg(args, 1), arg(args, 2), hasFlag(args, "--full"), hasFlag(args, "--json")))
	case "gen":
		die(cmdGen(ctx, l, args[1:]))
	case "meta":
		die(l.RefreshMeta(ctx))
	case "report":
		die(cmdReport(ctx, l, arg(args, 1), hasFlag(args, "--json")))
	case "reparse":
		n, err := l.Reparse(ctx)
		if err == nil {
			fmt.Printf("reparsed %d runs\n", n)
		}
		die(err)
	case "clean":
		l.Clean(ctx)
	default:
		usage()
		os.Exit(2)
	}
}

// cmdGen materializes a public benchmark into the active suite's tasks/ dir. The
// suite is chosen with the global --suite flag; the flags after gen tune the pull:
// --limit N per track, --all for the whole benchmark, --langs a,b to select
// tracks (aider) or datasets (evalplus), --difficulty easy,medium,hard to keep
// only certain LiveCodeBench tiers, and --no-validate to skip the proof.
func cmdGen(ctx context.Context, l *lab.Lab, rest []string) error {
	var opts lab.GenOptions
	langs, rest := takeFlagValue(rest, "--langs")
	if langs != "" {
		for _, s := range strings.Split(langs, ",") {
			if s = strings.TrimSpace(s); s != "" {
				opts.Langs = append(opts.Langs, s)
			}
		}
	}
	limit, rest := takeFlagValue(rest, "--limit")
	if limit != "" {
		n, err := strconv.Atoi(limit)
		if err != nil {
			return fmt.Errorf("--limit: %w", err)
		}
		opts.Limit = n
	}
	diff, rest := takeFlagValue(rest, "--difficulty")
	if diff != "" {
		for _, s := range strings.Split(diff, ",") {
			if s = strings.TrimSpace(s); s != "" {
				opts.Difficulty = append(opts.Difficulty, s)
			}
		}
	}
	perRepo, rest := takeFlagValue(rest, "--per-repo")
	if perRepo != "" {
		n, err := strconv.Atoi(perRepo)
		if err != nil {
			return fmt.Errorf("--per-repo: %w", err)
		}
		opts.PerRepo = n
	}
	opts.All = hasFlag(rest, "--all")
	opts.NoValidate = hasFlag(rest, "--no-validate")
	_, err := l.Generate(ctx, opts)
	return err
}

func cmdRun(ctx context.Context, l *lab.Lab, tool, scenario string) error {
	var tools, scenarios []string
	if tool != "" {
		tools = []string{tool}
	}
	if scenario != "" {
		scenarios = []string{scenario}
	}
	_, err := l.RunAll(ctx, tools, scenarios)
	return err
}

// cmdPrompt runs one ad-hoc prompt through every tool, or through the tools named
// after it, and prints the comparison. rest is everything after the -p flag: the
// first element is the prompt, the rest are optional tool filters.
func cmdPrompt(ctx context.Context, l *lab.Lab, rest []string) error {
	if len(rest) == 0 || rest[0] == "" {
		return fmt.Errorf("usage: lab -p \"<prompt>\" [tool...]")
	}
	prompt, tools := rest[0], rest[1:]
	results, err := l.RunPrompt(ctx, prompt, tools)
	if err != nil {
		return err
	}
	if len(results) == 0 {
		fmt.Fprintln(os.Stderr, "no tools ran")
		return nil
	}
	lab.WritePromptReport(os.Stdout, prompt, results)
	return nil
}

// cmdPrompts extracts the system prompt(s) a tool actually sent to the model,
// read from its captured traces. The full text prints by default so it can be
// saved next to the tool's docs; --brief drops the text and keeps the headers,
// and --json emits the structured form.
func cmdPrompts(l *lab.Lab, tool, scenario string, asJSON, brief bool) error {
	if tool == "" {
		return fmt.Errorf("usage: lab prompts <tool> [scenario] [--json] [--brief]")
	}
	tp, err := l.Prompts(tool, scenario)
	if err != nil {
		return err
	}
	if asJSON {
		b, err := json.MarshalIndent(tp, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(b))
		return nil
	}
	lab.WritePrompts(os.Stdout, tp, !brief)
	return nil
}

func cmdInspect(l *lab.Lab, tool, scenario string, full, asJSON bool) error {
	t, err := l.Inspect(tool, scenario)
	if err != nil {
		return err
	}
	if asJSON {
		b, err := json.MarshalIndent(t, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(b))
		return nil
	}
	inspect.WriteTranscript(os.Stdout, t, full)
	return nil
}

func cmdTools(l *lab.Lab) error {
	tools, err := l.Tools()
	if err != nil {
		return err
	}
	for _, t := range tools {
		fmt.Println(t)
	}
	return nil
}

func cmdScenarios(l *lab.Lab) error {
	scenarios, err := l.Scenarios()
	if err != nil {
		return err
	}
	for _, s := range scenarios {
		fmt.Printf("%-22s %s\n", s.Name, s.Desc)
	}
	return nil
}

func cmdReport(ctx context.Context, l *lab.Lab, scenario string, asJSON bool) error {
	sums, err := l.Report(ctx, scenario)
	if err != nil {
		return err
	}
	if len(sums) == 0 {
		fmt.Fprintln(os.Stderr, "no runs captured yet")
		return nil
	}
	if asJSON {
		b, err := json.MarshalIndent(sums, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(b))
		return nil
	}
	lab.WriteTable(os.Stdout, sums)
	return nil
}

func arg(args []string, i int) string {
	if i < len(args) && args[i] != "" && args[i][0] != '-' {
		return args[i]
	}
	return ""
}

func hasFlag(args []string, flag string) bool {
	return slices.Contains(args, flag)
}

// takeFlagValue pulls a "--flag value" (or "--flag=value") pair out of args and
// returns the value and the remaining args, so the caller's positional parsing
// never has to account for it. A missing flag yields an empty value and the args
// unchanged.
func takeFlagValue(args []string, flag string) (string, []string) {
	out := make([]string, 0, len(args))
	value := ""
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == flag && i+1 < len(args):
			value = args[i+1]
			i++
		case strings.HasPrefix(a, flag+"="):
			value = strings.TrimPrefix(a, flag+"=")
		default:
			out = append(out, a)
		}
	}
	return value, out
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: lab {build|run|-p|tools|scenarios|prompts|inspect|meta|gen|report|reparse|clean} [--suite <name>] [args]")
}

func die(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
