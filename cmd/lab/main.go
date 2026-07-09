// Command lab is the whole harness from the outside: build the images, run a
// tool over the scenarios, and report. Each run is one tool against one scenario
// in a throwaway container, with its LLM traffic routed through the trace proxy
// and every artifact and resource number captured under the data dir.
//
//	lab build [tool]            build base, proxy, and tool images
//	lab run [tool] [scenario]   run all, or one tool, or one pair
//	lab -p "<prompt>" [tool...] run one ad-hoc prompt through every tool, or some
//	lab tools                   list wired tools
//	lab scenarios               list scenarios
//	lab meta                    capture each tool's version and release date
//	lab report [--json]         summarize captured runs
//	lab clean                   remove lab containers and dangling images
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
	"syscall"

	"github.com/tamnd/tomo-labs/pkg/lab"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	args := os.Args[1:]
	if len(args) == 0 {
		usage()
		os.Exit(2)
	}

	l, err := lab.New(ctx, lab.DefaultConfig())
	if err != nil {
		die(err)
	}

	switch args[0] {
	case "build":
		die(l.Build(ctx, arg(args, 1)))
	case "run":
		die(cmdRun(ctx, l, arg(args, 1), arg(args, 2)))
	case "-p", "--prompt", "prompt":
		die(cmdPrompt(ctx, l, args[1:]))
	case "tools":
		die(cmdTools(l))
	case "scenarios":
		die(cmdScenarios(l))
	case "meta":
		die(l.RefreshMeta(ctx))
	case "report":
		die(cmdReport(ctx, l, hasFlag(args, "--json")))
	case "clean":
		l.Clean(ctx)
	default:
		usage()
		os.Exit(2)
	}
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

func cmdReport(ctx context.Context, l *lab.Lab, asJSON bool) error {
	sums, err := l.Report(ctx)
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

func usage() {
	fmt.Fprintln(os.Stderr, "usage: lab {build|run|-p|tools|scenarios|meta|report|clean} [args]")
}

func die(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
