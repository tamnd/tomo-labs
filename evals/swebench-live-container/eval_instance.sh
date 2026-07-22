#!/usr/bin/env bash
# SWE-bench-Live-faithful per-instance grader. Mirrors upstream run_evaluation +
# eval.sh: start FRESH instance container (already at base_commit) with NO network,
# apply model patch (fallback chain), reset test files to base, apply test_patch,
# run the row's test_cmds under conda/system python, parse pytest -rA, decide
# resolved = every FAIL_TO_PASS PASSED and every PASS_TO_PASS PASSED.
# Usage: eval_instance.sh IMAGE TEST_CMD MODEL_PATCH TEST_PATCH F2P_JSON P2P_JSON OUTDIR
set -u
IMAGE="$1"; TEST_CMD="$2"; MODEL_PATCH="$3"; TEST_PATCH="$4"; F2P="$5"; P2P="$6"; OUT="$7"
mkdir -p "$OUT"; cp "$F2P" "$OUT/f2p.json"; cp "$P2P" "$OUT/p2p.json"
CID="swe_$(date +%s)_$$"
docker run -d --name "$CID" --platform linux/amd64 --network none "$IMAGE" sleep infinity >/dev/null
cleanup(){ docker rm -f "$CID" >/dev/null 2>&1; }
trap cleanup EXIT
docker cp "$MODEL_PATCH" "$CID":/tmp/model.patch
docker cp "$TEST_PATCH"  "$CID":/tmp/test.patch
# test files the hidden patch touches (to reset to base, exactly like upstream)
TESTFILES=$(grep -E '^\+\+\+ b/' "$TEST_PATCH" | sed 's#^+++ b/##' | tr '\n' ' ')
echo "test_files: $TESTFILES" | tee "$OUT/apply.log"
docker exec -e TESTFILES="$TESTFILES" "$CID" bash -lc '
  set -e
  cd /testbed
  git config --global --add safe.directory /testbed || true
  BASE=$(git rev-parse HEAD)
  echo "== apply model patch (fallback chain) =="
  if [ -s /tmp/model.patch ]; then
    if   git apply --verbose /tmp/model.patch; then echo APPLY_PATCH_PASS
    elif git apply --verbose --reject /tmp/model.patch; then echo APPLY_PATCH_PASS
    elif patch --batch --fuzz=5 -p1 -i /tmp/model.patch; then echo APPLY_PATCH_PASS
    else echo APPLY_PATCH_FAIL; fi
  else echo "(empty model patch)"; fi
  echo "== reset test files to base =="
  # Reset each graded test file to base individually. A single multi-pathspec
  # `git checkout BASE -- a b c` aborts wholesale when any path is new (not in
  # base), leaving agent edits to the OTHER test files in place; that then makes
  # the atomic `git apply test.patch` fail on those edits. Per-file: restore
  # tracked files to base, delete agent-created new ones so test.patch recreates
  # them. Net effect: graded test tree = base + hidden test patch, exactly.
  for f in $TESTFILES; do
    if git cat-file -e "$BASE:$f" 2>/dev/null; then
      git checkout "$BASE" -- "$f"
    else
      rm -f "$f"
    fi
  done
  echo "== apply test patch =="
  git apply -v /tmp/test.patch
' 2>&1 | tee -a "$OUT/apply.log"
echo "== run tests (conda/system python, 1800s): $TEST_CMD ==" | tee -a "$OUT/apply.log"
docker exec "$CID" bash -lc '
  source /opt/miniconda3/bin/activate 2>/dev/null && conda activate testbed 2>/dev/null || true
  cd /testbed
  echo "@@START_TEST_OUTPUT@@"
  timeout 1800 '"$TEST_CMD"'
  echo "@@END_TEST_OUTPUT@@"
' > "$OUT/test.log" 2>&1
echo "test-run exit=$?"
python3 - "$OUT/test.log" "$F2P" "$P2P" <<'PY'
import sys,json,re
raw=open(sys.argv[1],errors='replace').read()
# only parse between the markers, like upstream START/END_TEST_OUTPUT
seg=raw
if '@@START_TEST_OUTPUT@@' in raw and '@@END_TEST_OUTPUT@@' in raw:
    seg=raw.split('@@START_TEST_OUTPUT@@',1)[1].split('@@END_TEST_OUTPUT@@',1)[0]
f2p=json.load(open(sys.argv[2])); p2p=json.load(open(sys.argv[3]))
STATUS=('PASSED','FAILED','ERROR','SKIPPED','XFAIL','XPASS')
smap={}
for line in seg.split('\n'):
    if any(line.startswith(s) for s in STATUS):
        if line.startswith('FAILED'): line=line.replace(' - ',' ')
        parts=line.split()
        if len(parts)<=1: continue
        smap[parts[1]]=parts[0]          # exact node-id -> status, like upstream
def passed(t): return smap.get(t) in ('PASSED','XFAIL')
f2p_res={t:smap.get(t,'MISSING') for t in f2p}
p2p_bad=[t for t in p2p if not passed(t)]   # missing/ERROR/FAILED all count as fail
resolved = all(passed(t) for t in f2p) and not p2p_bad
print("FAIL_TO_PASS:")
for t,v in f2p_res.items(): print(f"  {v:8} {t}")
print(f"PASS_TO_PASS: {len(p2p)} total, not-passing: {len(p2p_bad)}")
for t in p2p_bad[:8]: print("  NOTPASS", smap.get(t,'MISSING'), t)
print("RESOLVED:", resolved)
PY
