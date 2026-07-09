#!/usr/bin/env bash
set -e
W="$1"
cat > "$W/fizzbuzz.py" <<'PY'
for i in range(1, 21):
    if i % 3 == 0:
        print("Fizz")
    elif i % 5 == 0:
        print("Buzz")
    else:
        print(i)
PY
