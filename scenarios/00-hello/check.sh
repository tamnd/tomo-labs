#!/usr/bin/env bash
# This is the baseline scenario: the prompt is a bare "Hi!", so there is no
# artifact to grade and no right answer to check. It exists to measure the fixed
# cost every tool pays for one trivial round trip, the tokens, requests, latency,
# and memory a tool spends just to greet back. Completing the exchange is the only
# bar, so the check passes on its own; read the metrics columns, not this pass.
W="$1"
[ -d "$W" ] || { echo "work tree missing"; exit 1; }
echo "baseline greeting round trip completed"
exit 0
