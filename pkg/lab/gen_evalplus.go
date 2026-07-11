package lab

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// EvalPlus takes the HumanEval and MBPP benchmarks and adds far more tests per
// problem, so a solution that only fits the original handful of cases is caught.
// Each problem is a function to complete. genEvalPlus renders each one into the
// lab task shape: the stub laid into the work tree is solution.py, a signature and
// docstring for the agent to complete, and the grader concatenates the finished
// solution with the problem's hidden test body.
//
// The test body encodes the expected outputs, so unlike the Aider tests it must
// not reach the agent. It is written under oracle/, which the harness never
// mounts, and check.sh reads it from there. EvalPlus tests import numpy, which the
// base does not carry, so check.sh builds a suite-local venv with numpy once and
// reuses it, keeping the tier self-contained without changing the shared base
// image.
//
// The two datasets differ in shape. HumanEval+ hands us a signature-and-docstring
// prompt, an entry_point, and a completion; its test defines check(candidate) but
// never calls it. MBPP+ hands us a prose prompt and a full reference function with
// no entry_point, and its test drives the function by name itself. normalizeEval
// flattens both into one problem before it is written.

const evalRowsAPI = "https://datasets-server.huggingface.co/rows"

var evalDatasets = map[string]string{
	"humanevalplus": "evalplus/humanevalplus",
	"mbppplus":      "evalplus/mbppplus",
}

// evalRow is the union of the two dataset schemas; a given row fills only the
// fields its dataset carries.
type evalRow struct {
	TaskID            json.RawMessage `json:"task_id"`
	Prompt            string          `json:"prompt"`
	EntryPoint        string          `json:"entry_point"`
	Test              string          `json:"test"`
	CanonicalSolution string          `json:"canonical_solution"`
	Code              string          `json:"code"`
	TestImports       []string        `json:"test_imports"`
	TestList          []string        `json:"test_list"`
}

// evalProblem is one problem after both schemas are flattened: the graded function
// name, the solution.py the agent starts from, the hidden test body, and the full
// solution an ideal agent would leave (used only to validate).
type evalProblem struct {
	name, entry, stub, test, ideal string
}

var defRe = regexp.MustCompile(`def\s+(\w+)\s*\(`)

func (l *Lab) genEvalPlus(ctx context.Context, opts GenOptions) (int, error) {
	// The suite covers both datasets. --langs doubles as the dataset selector so a
	// caller can pull just one; empty takes both.
	datasets := opts.Langs
	if len(datasets) == 0 {
		datasets = []string{"humanevalplus", "mbppplus"}
	}
	limit := opts.Limit
	if limit <= 0 {
		limit = 8
	}

	kept, dropped := 0, 0
	for _, ds := range datasets {
		if _, ok := evalDatasets[ds]; !ok {
			fmt.Fprintf(os.Stderr, "  unknown dataset: %s\n", ds)
			continue
		}
		total := limit
		if opts.All {
			total = 500 // HumanEval+ is 164, MBPP+ is 378; this covers both.
		}
		rows, err := evalFetch(ctx, ds, total)
		if err != nil {
			return kept, err
		}
		for _, row := range rows {
			prob, err := normalizeEval(ds, row)
			if err != nil {
				fmt.Fprintf(os.Stderr, "  skip %s: %v\n", ds, err)
				dropped++
				continue
			}
			task, oracle, err := l.evalMaterialize(prob)
			if err != nil {
				return kept, err
			}
			if opts.NoValidate {
				fmt.Printf("  wrote %s\n", prob.name)
				kept++
				continue
			}
			ideal := prob.ideal
			ok, out, err := validateTask(ctx, task, func(work string) error {
				return os.WriteFile(filepath.Join(work, "solution.py"), []byte(ideal), 0o644)
			})
			if err != nil {
				return kept, err
			}
			if ok {
				fmt.Printf("  ok   %s\n", prob.name)
				kept++
			} else {
				fmt.Fprintf(os.Stderr, "  DROP %s: canonical solution failed check\n%s\n",
					prob.name, trim(out, 400))
				os.RemoveAll(task)
				os.RemoveAll(oracle)
				dropped++
			}
		}
		fmt.Printf("\n%s: kept %d, dropped %d\n", ds, kept, dropped)
	}
	return kept, nil
}

// evalFetch pulls up to total rows from the dataset viewer, which pages at 100.
func evalFetch(ctx context.Context, dataset string, total int) ([]evalRow, error) {
	var out []evalRow
	for offset := 0; len(out) < total; {
		length := min(100, total-len(out))
		q := url.Values{
			"dataset": {evalDatasets[dataset]}, "config": {"default"},
			"split": {"test"}, "offset": {fmt.Sprint(offset)}, "length": {fmt.Sprint(length)},
		}
		var page struct {
			Rows []struct {
				Row evalRow `json:"row"`
			} `json:"rows"`
		}
		if err := httpGetJSON(ctx, evalRowsAPI+"?"+q.Encode(), &page); err != nil {
			return nil, err
		}
		if len(page.Rows) == 0 {
			break
		}
		for _, r := range page.Rows {
			out = append(out, r.Row)
		}
		offset += len(page.Rows)
	}
	return out, nil
}

// normalizeEval flattens either dataset's row into one problem shape.
func normalizeEval(dataset string, row evalRow) (evalProblem, error) {
	test := row.Test
	if len(row.TestImports) > 0 {
		test = strings.Join(row.TestImports, "\n") + "\n" + test
	}
	name := dataset + "-" + evalSlug(row.TaskID)

	if dataset == "humanevalplus" {
		// The test defines check(candidate) but never calls it; the runner invokes
		// it against the completed function.
		test += fmt.Sprintf("\n\ncheck(%s)\n", row.EntryPoint)
		return evalProblem{
			name: name, entry: row.EntryPoint, stub: row.Prompt, test: test,
			ideal: row.Prompt + row.CanonicalSolution,
		}, nil
	}

	// MBPP+: read the entry point off the reference function's def line, and build
	// a stub from the prose prompt plus one example so the agent has a concrete
	// signature to fill. The reference function is the whole ideal solution.
	m := defRe.FindStringSubmatch(row.Code)
	if m == nil {
		return evalProblem{}, fmt.Errorf("no def in code")
	}
	entry := m[1]
	sig := evalDefLine(row.Code, entry)
	if sig == "" {
		sig = fmt.Sprintf("def %s(*args):", entry)
	}
	doc := strings.TrimSpace(row.Prompt)
	if len(row.TestList) > 0 {
		doc += "\n\n    Example:\n    >>> " + strings.TrimSpace(row.TestList[0])
	}
	stub := fmt.Sprintf("%s\n    \"\"\"%s\n    \"\"\"\n", sig, doc)
	return evalProblem{name: name, entry: entry, stub: stub, test: test, ideal: row.Code}, nil
}

// evalSlug turns a task_id into a directory-safe suffix. HumanEval ids are strings
// like "HumanEval/0"; MBPP+ ids are bare integers.
func evalSlug(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return strings.ToLower(strings.ReplaceAll(s, "/", "-"))
	}
	return strings.Trim(string(raw), `"`)
}

// evalDefLine returns the `def entry(...):` line from a reference solution.
func evalDefLine(code, entry string) string {
	prefix := "def " + entry
	for _, line := range strings.Split(code, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, prefix) && strings.ContainsRune(t, '(') {
			return t
		}
	}
	return ""
}

func (l *Lab) evalMaterialize(prob evalProblem) (task, oracle string, err error) {
	task = filepath.Join(l.tasksDir(), prob.name)
	oracle = filepath.Join(l.suiteDir(), "oracle", prob.name)
	os.RemoveAll(task)

	// The stub the agent completes is solution.py; the hidden test stays out of the
	// task dir so it is never mounted.
	if err = writeFile(filepath.Join(task, "files", "solution.py"), []byte(prob.stub), 0o644); err != nil {
		return
	}
	if err = writeFile(filepath.Join(oracle, "test.py"), []byte(prob.test), 0o644); err != nil {
		return
	}

	prompt := fmt.Sprintf("Complete the function in solution.py so it satisfies its docstring. "+
		"Keep the exact signature; write only into solution.py. Your solution is graded by a "+
		"hidden test suite that calls %s.\n\nsolution.py starts as:\n\n%s\n",
		prob.entry, strings.TrimSpace(prob.stub))
	if err = writeFile(filepath.Join(task, "prompt.txt"), []byte(prompt), 0o644); err != nil {
		return
	}
	desc := fmt.Sprintf("%s: complete %s\n", strings.SplitN(prob.name, "-", 2)[0], prob.entry)
	if err = writeFile(filepath.Join(task, "desc"), []byte(desc), 0o644); err != nil {
		return
	}

	setup := "#!/usr/bin/env bash\n" +
		"# Lay the function stub into the work tree.\n" +
		"set -e\n" +
		"W=\"$1\"\n" +
		"cp -R \"$(dirname \"$0\")/files/.\" \"$W/\"\n"
	if err = writeFile(filepath.Join(task, "setup.sh"), []byte(setup), 0o755); err != nil {
		return
	}

	// check.sh assembles the finished solution and the hidden test into one module
	// and runs it in the numpy venv. The test drives the graded function itself, so
	// the runner just concatenates the two. The oracle sits two levels up from the
	// task dir, under the suite's oracle/, which the harness does not mount.
	check := "#!/usr/bin/env bash\n" +
		"# Grade against the hidden EvalPlus test suite.\n" +
		"W=\"$1\"\n" +
		"D=\"$(cd \"$(dirname \"$0\")\" && pwd)\"\n" +
		"SUITE=\"$(cd \"$D/../..\" && pwd)\"\n" +
		"NAME=\"$(basename \"$D\")\"\n" +
		"ORACLE=\"$SUITE/oracle/$NAME/test.py\"\n" +
		"VENV=\"$SUITE/.venv\"\n" +
		"if [ ! -x \"$VENV/bin/python3\" ]; then\n" +
		"  python3 -m venv \"$VENV\" >/dev/null 2>&1 && \"$VENV/bin/pip\" install -q numpy >/dev/null 2>&1\n" +
		"fi\n" +
		"runner=\"$(mktemp)\"\n" +
		"cat \"$W/solution.py\" \"$ORACLE\" > \"$runner\"\n" +
		"cd \"$W\" && \"$VENV/bin/python3\" \"$runner\"\n" +
		"rc=$?\n" +
		"rm -f \"$runner\"\n" +
		"exit $rc\n"
	if err = writeFile(filepath.Join(task, "check.sh"), []byte(check), 0o755); err != nil {
		return
	}
	return task, oracle, nil
}
