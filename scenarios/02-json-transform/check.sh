#!/usr/bin/env bash
# Engineering only, ranked by descending salary, each object intact. Compare as
# ordered [name, salary, department] triples so object field order never matters.
W="$1"
[ -f "$W/engineering.json" ] || { echo "engineering.json missing"; exit 1; }
got="$(jq -c '[.[] | [.name, .salary, .department]]' "$W/engineering.json" 2>/dev/null)"
want='[["Grace Okafor",175000,"Engineering"],["Carol Diaz",160000,"Engineering"],["Peggy Adeyemi",152000,"Engineering"],["Alice Tran",145000,"Engineering"],["Mallory Singh",140000,"Engineering"],["Eve Larsson",132000,"Engineering"],["Ivan Novak",118000,"Engineering"]]'
if [ "$got" = "$want" ]; then echo "engineering filtered and ranked by salary"; exit 0; fi
echo "got:  $got"; echo "want: $want"; exit 1
