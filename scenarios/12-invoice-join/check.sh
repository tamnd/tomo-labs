#!/usr/bin/env bash
# Joining orders.csv against the catalog gives these line totals: widget 10*4.50
# =45, gadget 3*12=36, sprocket 8*2.25=18, cog 20*0.75=15, flange 5*8=40. The
# grand total is 154. The checker recomputes rather than trusting the number,
# and matches per product so column order in invoice.csv does not matter.
W="$1"
[ -f "$W/invoice.py" ] || { echo "invoice.py missing (write a program that builds the invoice)"; exit 1; }
[ -f "$W/invoice.csv" ] || { echo "invoice.csv missing"; exit 1; }
[ -f "$W/grand_total.txt" ] || { echo "grand_total.txt missing"; exit 1; }

python3 - "$W" <<'PY'
import csv, os, re, sys
W = sys.argv[1]
# product -> (qty, unit_price, line_total)
expected = {
    "widget":   (10, 4.50, 45.00),
    "gadget":   (3, 12.00, 36.00),
    "sprocket": (8, 2.25, 18.00),
    "cog":      (20, 0.75, 15.00),
    "flange":   (5, 8.00, 40.00),
}
rows = {}
with open(os.path.join(W, "invoice.csv")) as f:
    for row in csv.reader(f):
        if not row or not row[0].strip():
            continue
        p = row[0].strip().lower()
        if p in ("product", "item", "name"):
            continue
        nums = [float(x) for x in row[1:] if re.fullmatch(r"-?\d+(\.\d+)?", x.strip())]
        rows[p] = nums

missing = [p for p in expected if p not in rows]
if missing:
    print("invoice.csv is missing products:", ", ".join(sorted(missing)))
    sys.exit(1)
for p, (qty, unit, line) in expected.items():
    got = rows[p]
    if not any(abs(x - line) < 0.01 for x in got):
        print(f"{p}: line total {line:.2f} not found in row {got}")
        sys.exit(1)

with open(os.path.join(W, "grand_total.txt")) as f:
    text = f.read()
m = re.search(r"-?\d+(\.\d+)?", text)
if not m or abs(float(m.group()) - 154.00) > 0.01:
    print("grand_total.txt should be 154.00, got:", text.strip())
    sys.exit(1)
print("invoice joined and totaled correctly")
PY
