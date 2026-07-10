#!/usr/bin/env bash
# A correct solution builds, passes go test, and its program writes 32.55 to
# report.txt: Subtotal(2.50, 12) = 30.00, plus CA tax 8.5% = 32.55. The build
# only compiles once taxes.go defines TaxRate, and the tests only pass once the
# Subtotal bug is fixed, so a green run means the whole job was done.
W="$1"
cd "$W" || { echo "no work dir"; exit 1; }
log="$(mktemp)"
if ! go build ./... >"$log" 2>&1; then echo "go build failed:"; cat "$log"; exit 1; fi
if ! go test ./... >"$log" 2>&1; then echo "go test failed:"; cat "$log"; exit 1; fi
[ -f report.txt ] || { echo "report.txt missing (run the program to write it)"; exit 1; }
n="$(grep -oE '[0-9]+\.[0-9]+' report.txt | head -1)"
if awk -v g="$n" 'BEGIN{ exit !(g + 0 > 32.54 && g + 0 < 32.56) }'; then
  echo "build green, tests pass, report is 32.55"; exit 0
fi
echo "report.txt should be 32.55, got '$n'"; exit 1
