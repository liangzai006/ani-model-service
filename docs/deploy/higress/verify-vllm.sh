#!/usr/bin/env bash
set -euo pipefail

base_url="${ANI_HIGRESS_URL:-http://192.168.102.68:30090}"
model="${ANI_TEST_MODEL:?set ANI_TEST_MODEL to a served model name}"
completion_url="${base_url%/}/v1/completions"

body=$(printf '{"model":"%s","prompt":"higress smoke test","max_tokens":2}' "$model")
status=$(curl -sS -m 60 -o /tmp/ani-higress-completion.json -w '%{http_code}' \
  -H 'content-type: application/json' -d "$body" "$completion_url")
test "$status" = 200
python3 -c 'import json,sys; d=json.load(open(sys.argv[1])); assert d["model"] and d["choices"][0]["text"]' /tmp/ani-higress-completion.json

unknown='ani-unknown-model'
unknown_body=$(printf '{"model":"%s","prompt":"higress negative test","max_tokens":1}' "$unknown")
unknown_status=$(curl -sS -m 20 -o /dev/null -w '%{http_code}' \
  -H 'content-type: application/json' -d "$unknown_body" "$completion_url")
test "$unknown_status" = 404

printf 'Higress completion passed for %s; unknown model returned 404.\n' "$model"
