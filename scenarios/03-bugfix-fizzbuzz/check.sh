#!/usr/bin/env bash
W="$1"
got="$(cd "$W" && python3 fizzbuzz.py 2>/dev/null)"
want="$(python3 - <<'PY'
for i in range(1, 21):
    if i % 15 == 0: print("FizzBuzz")
    elif i % 3 == 0: print("Fizz")
    elif i % 5 == 0: print("Buzz")
    else: print(i)
PY
)"
if [ "$got" = "$want" ]; then echo "fizzbuzz output is correct"; exit 0; fi
echo "output did not match expected FizzBuzz"; exit 1
