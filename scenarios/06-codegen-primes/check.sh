#!/usr/bin/env bash
# Compare the program's output against a trusted sieve, so the expected list is
# computed rather than pasted in.
W="$1"
[ -f "$W/primes.go" ] || { echo "primes.go missing"; exit 1; }
got="$(cd "$W" && go run primes.go 2>/dev/null | tr -s ' \n' '\n' | grep -E '^[0-9]+$')"
want="$(python3 - <<'PY'
for n in range(2, 100):
    if all(n % d for d in range(2, int(n ** 0.5) + 1)):
        print(n)
PY
)"
if [ "$got" = "$want" ]; then echo "primes program is correct"; exit 0; fi
echo "primes output did not match"; echo "$got"; exit 1
