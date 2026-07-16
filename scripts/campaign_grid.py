#!/usr/bin/env python3
# Abort-aware grid over a campaign sweep's out-directory. One row per model, read
# from each run's summary.json. A run that ends with a non-empty error field and
# did not pass is an infra ABORT on the flaky free tier (gateway 400, network
# drop, deadline), surfaced distinctly so it can never be miscounted as a task
# failure. PASS is a graded green from check.sh; fail is a graded, error-free
# miss the model actually earned.
import glob
import json
import os
import sys

root = sys.argv[1] if len(sys.argv) > 1 else "."
rows = []
for spath in sorted(glob.glob(os.path.join(root, "*", "summary.json"))):
    d = json.load(open(spath))
    m = d.get("model") or os.path.basename(os.path.dirname(spath))
    err = (d.get("error") or "").strip()
    passed = bool(d.get("passed"))
    if passed:
        verdict = "PASS"
    elif err:
        verdict = "ABORT"
    else:
        verdict = "fail"
    rows.append({
        "model": m,
        "verdict": verdict,
        "rounds": d.get("rounds"),
        "actions": d.get("tool_calls"),
        "in": d.get("input_tokens"),
        "cin": d.get("cached_input_tokens"),
        "out": d.get("output_tokens"),
        "cost": round(d.get("cost_usd") or 0, 4),
        "secs": round(d.get("elapsed_secs") or 0, 1),
        "check": (d.get("check_reason") or "")[-46:].strip(),
        "err": err[:70],
    })

if not rows:
    print(f"no runs under {root}")
    sys.exit(0)

cols = ["model", "verdict", "rounds", "actions", "in", "cin", "out", "cost", "secs", "check"]
w = {c: max(len(c), *(len(str(r[c])) for r in rows)) for c in cols}
print("  ".join(c.ljust(w[c]) for c in cols))
for r in rows:
    print("  ".join(str(r[c]).ljust(w[c]) for c in cols))
    if r["verdict"] == "ABORT":
        print(f"    ABORT err: {r['err']}")

# Campaign scoreboard: cheapest passing free model is the number that matters,
# since the goal is solved-and-cheapest. Aborts are excluded from the tally.
passes = [r for r in rows if r["verdict"] == "PASS"]
aborts = [r for r in rows if r["verdict"] == "ABORT"]
fails = [r for r in rows if r["verdict"] == "fail"]
print(f"\n{len(passes)} pass, {len(fails)} fail, {len(aborts)} abort")
if passes:
    cheapest = min(passes, key=lambda r: (r["cost"], r["in"]))
    print(f"cheapest pass: {cheapest['model']}  ${cheapest['cost']}  in={cheapest['in']} tok")
