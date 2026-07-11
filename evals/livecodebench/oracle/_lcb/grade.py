"""Grade one LiveCodeBench solution against its hidden tests.

Called as: grade.py <solution.py> <oracle-dir>

It rebuilds the exact input_output sample LiveCodeBench feeds its own runner,
then calls the vendored run_test. run_test returns a per-test results list; a
solution passes only when every test is True. The hidden tests live in the
oracle dir the harness never mounts, so the agent never sees them.

The private tests are stored the way the upstream dataset ships them: either a
plain JSON string or a base64 -> zlib -> pickle -> JSON blob. This mirrors the
decode in LiveCodeBench's own benchmark loader.
"""

import base64
import json
import pickle
import sys
import zlib

from testing_util import run_test


def load_sample(oracle):
    with open(oracle + "/public.json") as f:
        public = json.loads(f.read())
    with open(oracle + "/private.txt") as f:
        raw = f.read()
    try:
        private = json.loads(raw)
    except Exception:
        private = json.loads(
            pickle.loads(zlib.decompress(base64.b64decode(raw.encode("utf-8"))))
        )
    with open(oracle + "/meta.json") as f:
        meta = json.loads(f.read())
    tests = public + private
    return {
        "input_output": json.dumps(
            {
                "inputs": [t["input"] for t in tests],
                "outputs": [t["output"] for t in tests],
                "fn_name": meta.get("func_name", None),
            }
        )
    }, len(tests)


def main():
    solution_path, oracle = sys.argv[1], sys.argv[2]
    with open(solution_path) as f:
        code = f.read()
    sample, ntests = load_sample(oracle)
    try:
        results, metadata = run_test(sample, test=code, timeout=6)
    except Exception as e:
        print("grader error:", repr(e))
        sys.exit(1)
    ok = len(results) > 0 and all(r == True for r in results)
    if ok:
        print("PASS", ntests, "tests")
        sys.exit(0)
    print("FAIL", str(metadata))
    sys.exit(1)


main()
