#!/usr/bin/env bash
# cache_key_ab.sh — measure whether prompt_cache_key moves cached_input_tokens on
# the opencode/deepseek free path. A number, not a coin-flip: two warm-then-read
# arms (no key vs a stable key) against a large fixed prefix. Run when the free
# tier is not rate-capped (429 = try later, not a result). Reads OPENCODE_API_KEY
# from the environment; never prints it.
set -euo pipefail
: "${OPENCODE_API_KEY:?source ~/data/.local.env first}"
URL="https://opencode.ai/zen/v1/chat/completions"
MODEL="${MODEL:-deepseek-v4-flash-free}"

PREFIX=$(python3 -c "print('You are a code assistant. Context follows. ' + ('The quick brown fox jumps over the lazy dog. '*400))")
mk() { python3 -c "
import json,sys
key=sys.argv[1]
b={'model':sys.argv[3],'messages':[{'role':'system','content':sys.argv[2]},{'role':'user','content':'Reply with the single word ok.'}],'max_tokens':3}
if key: b['prompt_cache_key']=key
print(json.dumps(b))" "$1" "$PREFIX" "$MODEL"; }
probe() { curl -sS -o /tmp/ck_r.json -w "%{http_code}" "$URL" -H "Authorization: Bearer $OPENCODE_API_KEY" -H "Content-Type: application/json" -d "$1"; }
usage() { python3 -c "import json;d=json.load(open('/tmp/ck_r.json'));print(d.get('usage') or d.get('error'))"; }

arm() { # $1=label $2=body
  echo "== $1 =="
  for i in 1 2; do
    c=$(probe "$2"); echo "  call$i http=$c usage=$(usage)"; sleep 2
  done
}
arm "no key   (baseline)" "$(mk '' )"
arm "with key (tomo-abtest)" "$(mk 'tomo-abtest')"
echo "compare cached_input_tokens on call2 of each arm: higher-with-key => routing helps on this provider."
