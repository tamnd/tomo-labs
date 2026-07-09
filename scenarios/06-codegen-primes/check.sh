#!/usr/bin/env bash
W="$1"
[ -f "$W/primes.go" ] || { echo "primes.go missing"; exit 1; }
got="$(cd "$W" && go run primes.go 2>/dev/null | tr -s ' \n' '\n' | grep -E '^[0-9]+$')"
want=$'2\n3\n5\n7\n11\n13\n17\n19\n23\n29\n31\n37\n41\n43\n47'
if [ "$got" = "$want" ]; then echo "primes program is correct"; exit 0; fi
echo "primes output did not match"; echo "$got"; exit 1
