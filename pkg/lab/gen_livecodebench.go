package lab

import (
	"bufio"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// LiveCodeBench is a contamination-free code benchmark: problems are scraped from
// LeetCode, AtCoder, and Codeforces with a release date attached, so a run can be
// scoped to problems published after a model's training cutoff. Each problem is
// one of two shapes. A stdin problem reads a case from standard input and writes
// the answer to standard output, the way a competitive judge runs it. A functional
// problem hands over a LeetCode-style `class Solution` with a method to fill in,
// and the hidden tests import that class and call the method directly.
//
// genLiveCodeBench renders each problem into the lab task shape. The agent starts
// from solution.py: an empty program for a stdin problem, or the starter class for
// a functional one. Grading reuses LiveCodeBench's own test runner verbatim,
// vendored under oracle/_lcb/, so a solution is judged exactly the way the upstream
// benchmark judges it, feeding stdin and diffing stdout for a stdin problem or
// calling the method and comparing return values for a functional one.
//
// The hidden tests encode the answers, so they never reach the agent. They are
// written under oracle/, a sibling directory the harness never mounts, in the same
// wire form the dataset ships (a plain JSON array for the public cases, and the
// base64/zlib/pickle blob for the private cases), and grade.py decodes them at
// grading time. Like the EvalPlus tier, the runner imports numpy, so check.sh
// builds one suite-local venv and reuses it.
//
// Unlike the other tiers, LiveCodeBench ships no reference solution, by design: it
// is a held-out benchmark. So a task cannot be proven by grading a known-good
// answer. Instead each task is smoke-proven the other way: the generator grades
// the untouched stub and keeps the task only if the runner ran to completion and
// correctly rejected it, which shows the grader is wired end to end and does not
// pass for free.

//go:embed lcb_testing_util.py
var lcbTestingUtil []byte

//go:embed lcb_grade.py
var lcbGrade []byte

// lcbFiles maps a version tag to the split file that carries that version's
// problems, matching the dataset's own layout. Higher versions are later problems.
var lcbFiles = map[string]string{
	"v1": "test.jsonl",
	"v2": "test2.jsonl",
	"v3": "test3.jsonl",
	"v4": "test4.jsonl",
	"v5": "test5.jsonl",
	"v6": "test6.jsonl",
}

const (
	lcbBase = "https://huggingface.co/datasets/livecodebench/code_generation_lite/resolve/main/"
	// A problem's private tests are inlined, so a single record can be multiple
	// megabytes. Skip the giant ones to keep the rendered sample lean, and stop
	// reading the split once enough small problems are in hand.
	lcbMaxPrivateBytes = 60_000
	lcbMaxScanBytes    = 96 << 20 // stop streaming the split after this much
	lcbMaxScanRecords  = 240
)

// lcbRow is one dataset record. Every field is a string; the test-case and
// metadata fields hold JSON (or, for the private cases, a compressed blob).
type lcbRow struct {
	QuestionTitle    string `json:"question_title"`
	QuestionContent  string `json:"question_content"`
	Platform         string `json:"platform"`
	QuestionID       string `json:"question_id"`
	ContestID        string `json:"contest_id"`
	ContestDate      string `json:"contest_date"`
	StarterCode      string `json:"starter_code"`
	Difficulty       string `json:"difficulty"`
	PublicTestCases  string `json:"public_test_cases"`
	PrivateTestCases string `json:"private_test_cases"`
	Metadata         string `json:"metadata"`
}

// functional reports whether a row is a call-based (LeetCode) problem, which the
// runner detects by a func_name in the metadata.
func (r lcbRow) functional() bool {
	var m struct {
		FuncName string `json:"func_name"`
	}
	_ = json.Unmarshal([]byte(r.Metadata), &m)
	return m.FuncName != ""
}

func (l *Lab) genLiveCodeBench(ctx context.Context, opts GenOptions) (int, error) {
	// --langs doubles as the version selector so a caller can pin a release window;
	// empty takes the first version. The split files are cumulative deltas, so v1 is
	// the smallest download and the natural default for a sample.
	version := "v1"
	if len(opts.Langs) > 0 {
		version = strings.ToLower(opts.Langs[0])
	}
	file, ok := lcbFiles[version]
	if !ok {
		return 0, fmt.Errorf("unknown livecodebench version %q: try v1..v6", version)
	}
	limit := opts.Limit
	if limit <= 0 {
		limit = 6
	}
	if opts.All {
		limit = lcbMaxScanRecords
	}

	// The dataset tags every problem easy, medium, or hard. A caller can pin the
	// tier with --difficulty so a run showcases a tool where it is strongest (easy
	// problems for a clean green sweep) or stresses it (hard problems), or leaves
	// it open to take whatever difficulty comes first.
	want, err := lcbWantDifficulty(opts.Difficulty)
	if err != nil {
		return 0, err
	}

	// Collect qualifying problems into two buckets so the sample exercises both the
	// stdin and the functional grading paths, then draw a balanced set.
	var stdin, funcs []lcbRow
	if err := lcbStream(ctx, lcbBase+file, func(row lcbRow) bool {
		if len(row.PrivateTestCases) > lcbMaxPrivateBytes || strings.TrimSpace(row.QuestionContent) == "" {
			return false
		}
		if want != nil && !want[normDifficulty(row.Difficulty)] {
			return false
		}
		if row.functional() {
			funcs = append(funcs, row)
		} else {
			stdin = append(stdin, row)
		}
		enough := len(stdin) >= 1 && len(funcs) >= 1 && len(stdin)+len(funcs) >= limit
		return !opts.All && enough
	}); err != nil {
		return 0, err
	}

	picked := lcbBalance(stdin, funcs, limit)
	if len(picked) == 0 {
		return 0, fmt.Errorf("no livecodebench problems small enough to render from %s", file)
	}

	if err := l.lcbWriteShared(); err != nil {
		return 0, err
	}

	kept, dropped := 0, 0
	for _, row := range picked {
		task, oracle, err := l.lcbMaterialize(row)
		if err != nil {
			return kept, err
		}
		name := filepath.Base(task)
		if opts.NoValidate {
			fmt.Printf("  wrote %s\n", name)
			kept++
			continue
		}
		// Smoke proof: the untouched stub must be rejected by a grader that actually
		// ran. A pass here, or output with neither verdict, means the grader is not
		// wired, so the task is dropped.
		ok, out, err := validateTask(ctx, task, func(work string) error { return nil })
		if err != nil {
			return kept, err
		}
		ran := strings.Contains(out, "PASS") || strings.Contains(out, "FAIL")
		if !ok && ran {
			fmt.Printf("  ok   %s\n", name)
			kept++
		} else {
			why := "stub unexpectedly passed"
			if !ran {
				why = "grader did not run"
			}
			fmt.Fprintf(os.Stderr, "  DROP %s: %s\n%s\n", name, why, trim(out, 400))
			os.RemoveAll(task)
			os.RemoveAll(oracle)
			dropped++
		}
	}
	tier := "any difficulty"
	if want != nil {
		tier = strings.Join(opts.Difficulty, ",")
	}
	fmt.Printf("\nlivecodebench %s (%s): kept %d, dropped %d\n", version, tier, kept, dropped)
	return kept, nil
}

// lcbDifficulties are the tiers the dataset tags each problem with. A --difficulty
// value has to be one of these.
var lcbDifficulties = map[string]bool{"easy": true, "medium": true, "hard": true}

// normDifficulty lowercases and trims a dataset difficulty tag so a request and a
// row compare on the same footing regardless of the casing either arrives in.
func normDifficulty(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

// lcbWantDifficulty turns the requested tiers into a lookup set, lowercased so it
// matches the dataset's own casing. An empty request returns a nil set, which the
// caller reads as "take any difficulty". An unknown tier is an error rather than a
// silent no-match, so a typo does not quietly render zero tasks.
func lcbWantDifficulty(diffs []string) (map[string]bool, error) {
	if len(diffs) == 0 {
		return nil, nil
	}
	want := make(map[string]bool, len(diffs))
	for _, d := range diffs {
		d = normDifficulty(d)
		if !lcbDifficulties[d] {
			return nil, fmt.Errorf("unknown difficulty %q: try easy, medium, or hard", d)
		}
		want[d] = true
	}
	return want, nil
}

// lcbBalance draws up to limit problems, alternating between the functional and
// stdin buckets so both grading paths are represented, then fills from whichever
// bucket still has problems if one runs short.
func lcbBalance(stdin, funcs []lcbRow, limit int) []lcbRow {
	var out []lcbRow
	i, j := 0, 0
	for len(out) < limit && (i < len(funcs) || j < len(stdin)) {
		if i < len(funcs) {
			out = append(out, funcs[i])
			i++
		}
		if len(out) < limit && j < len(stdin) {
			out = append(out, stdin[j])
			j++
		}
	}
	return out
}

// lcbStream reads the split as line-delimited JSON, handing each record to visit
// and stopping when visit returns true or the scan budget is spent. Records are
// large, so it caps both the bytes pulled and the records seen rather than
// downloading the whole multi-gigabyte split.
func lcbStream(ctx context.Context, url string, visit func(lcbRow) bool) error {
	cctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(cctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: %s", url, resp.Status)
	}

	br := bufio.NewReaderSize(resp.Body, 1<<20)
	var read int64
	for records := 0; records < lcbMaxScanRecords && read < lcbMaxScanBytes; records++ {
		line, err := br.ReadBytes('\n')
		read += int64(len(line))
		if len(line) > 0 {
			var row lcbRow
			if json.Unmarshal(line, &row) == nil && row.QuestionID != "" {
				if visit(row) {
					return nil
				}
			}
		}
		if err != nil {
			break
		}
	}
	return nil
}

var lcbSlugRe = regexp.MustCompile(`[^a-z0-9]+`)

// lcbSlug turns a platform and question id into a directory-safe task name.
func lcbSlug(platform, id string) string {
	s := strings.ToLower(platform + "-" + id)
	s = lcbSlugRe.ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}

// lcbWriteShared drops the vendored runner and the grade script under the suite's
// oracle/_lcb/, where every task's check.sh reaches them and the harness never
// mounts them. It is idempotent, rewritten on each generation.
func (l *Lab) lcbWriteShared() error {
	dir := filepath.Join(l.suiteDir(), "oracle", "_lcb")
	if err := writeFile(filepath.Join(dir, "testing_util.py"), lcbTestingUtil, 0o644); err != nil {
		return err
	}
	return writeFile(filepath.Join(dir, "grade.py"), lcbGrade, 0o644)
}

func (l *Lab) lcbMaterialize(row lcbRow) (task, oracle string, err error) {
	name := lcbSlug(row.Platform, row.QuestionID)
	task = filepath.Join(l.tasksDir(), name)
	oracle = filepath.Join(l.suiteDir(), "oracle", name)
	os.RemoveAll(task)

	// solution.py is what the agent completes: the starter class for a functional
	// problem, or an empty stdin program otherwise.
	stub := "# Read the input from standard input and write the answer to standard output.\n"
	fn := row.functional()
	if fn {
		stub = row.StarterCode
		if strings.TrimSpace(stub) == "" {
			stub = "class Solution:\n    pass\n"
		}
	}
	if err = writeFile(filepath.Join(task, "files", "solution.py"), []byte(stub), 0o644); err != nil {
		return
	}

	// The hidden tests stay under oracle/, never mounted, in the dataset's own wire
	// form so grade.py decodes them exactly like the upstream loader.
	if err = writeFile(filepath.Join(oracle, "public.json"), []byte(row.PublicTestCases), 0o644); err != nil {
		return
	}
	if err = writeFile(filepath.Join(oracle, "private.txt"), []byte(row.PrivateTestCases), 0o644); err != nil {
		return
	}
	meta := row.Metadata
	if strings.TrimSpace(meta) == "" {
		meta = "{}"
	}
	if err = writeFile(filepath.Join(oracle, "meta.json"), []byte(meta), 0o644); err != nil {
		return
	}

	if err = writeFile(filepath.Join(task, "prompt.txt"), []byte(lcbPrompt(row, fn)), 0o644); err != nil {
		return
	}
	kind := "stdin"
	if fn {
		kind = "functional"
	}
	tier := normDifficulty(row.Difficulty)
	if tier == "" {
		tier = "unknown"
	}
	desc := fmt.Sprintf("livecodebench: %s %s (%s, %s)\n", row.Platform, row.QuestionID, kind, tier)
	if err = writeFile(filepath.Join(task, "desc"), []byte(desc), 0o644); err != nil {
		return
	}

	setup := "#!/usr/bin/env bash\n" +
		"# Lay the starting solution.py into the work tree.\n" +
		"set -e\n" +
		"W=\"$1\"\n" +
		"cp -R \"$(dirname \"$0\")/files/.\" \"$W/\"\n"
	if err = writeFile(filepath.Join(task, "setup.sh"), []byte(setup), 0o755); err != nil {
		return
	}

	// check.sh grades solution.py with the vendored LiveCodeBench runner in the
	// numpy venv. The oracle and the shared runner sit under the suite's oracle/,
	// two levels up from the task, which the harness does not mount.
	check := "#!/usr/bin/env bash\n" +
		"# Grade against the hidden LiveCodeBench tests with the vendored runner.\n" +
		"W=\"$1\"\n" +
		"D=\"$(cd \"$(dirname \"$0\")\" && pwd)\"\n" +
		"SUITE=\"$(cd \"$D/../..\" && pwd)\"\n" +
		"NAME=\"$(basename \"$D\")\"\n" +
		"ORACLE=\"$SUITE/oracle/$NAME\"\n" +
		"LCB=\"$SUITE/oracle/_lcb\"\n" +
		"VENV=\"$SUITE/.venv\"\n" +
		"if [ ! -x \"$VENV/bin/python3\" ]; then\n" +
		"  python3 -m venv \"$VENV\" >/dev/null 2>&1 && \"$VENV/bin/pip\" install -q numpy >/dev/null 2>&1\n" +
		"fi\n" +
		"PYTHONPATH=\"$LCB\" \"$VENV/bin/python3\" \"$LCB/grade.py\" \"$W/solution.py\" \"$ORACLE\"\n"
	if err = writeFile(filepath.Join(task, "check.sh"), []byte(check), 0o755); err != nil {
		return
	}
	return task, oracle, nil
}

// lcbPrompt renders the task instructions for the agent, differing by problem
// shape: a functional problem completes the starter class, a stdin problem writes
// a whole program.
func lcbPrompt(row lcbRow, functional bool) string {
	var b strings.Builder
	if functional {
		var m struct {
			FuncName string `json:"func_name"`
		}
		_ = json.Unmarshal([]byte(row.Metadata), &m)
		fmt.Fprintf(&b, "Solve this problem by completing the given starter code.\n")
		fmt.Fprintf(&b, "Write your solution in solution.py. Keep the class and method names exactly as given, "+
			"since the hidden tests import your class and call %s directly.\n", m.FuncName)
		b.WriteString("Do not read from standard input.\n\n")
		b.WriteString(strings.TrimSpace(row.QuestionContent))
		b.WriteString("\n\nStarter code to complete in solution.py:\n\n")
		b.WriteString(strings.TrimSpace(row.StarterCode))
		b.WriteString("\n")
	} else {
		b.WriteString("Solve this competitive programming problem.\n")
		b.WriteString("Write a self-contained program in solution.py that reads the input from standard input " +
			"and writes the answer to standard output.\n")
		b.WriteString("Print only the required output and nothing else.\n\n")
		b.WriteString(strings.TrimSpace(row.QuestionContent))
		b.WriteString("\n")
	}
	return b.String()
}
