#!/usr/bin/env bash
# Smoke-verify the Feature Flag API against a base URL (local or production).
# Usage:
#   ./scripts/verify.sh                         # defaults to http://127.0.0.1:8080
#   ./scripts/verify.sh http://127.0.0.1:8080
#   ./scripts/verify.sh https://your-app.ondigitalocean.app
set -euo pipefail

BASE_URL="${1:-${BASE_URL:-http://127.0.0.1:8080}}"
BASE_URL="${BASE_URL%/}"
FLAG="verify_checkout"
USER="verify_user_1"

pass=0
fail=0

check() {
  local name="$1"
  local want_code="$2"
  local body_regex="${3:-}"
  shift 3 || true
  local out code
  out="$(curl -sS -w '\n%{http_code}' "$@" || true)"
  code="$(printf '%s' "$out" | tail -n1)"
  body="$(printf '%s' "$out" | sed '$d')"
  if [[ "$code" != "$want_code" ]]; then
    echo "FAIL ${name}: status=${code} want=${want_code} body=${body}"
    fail=$((fail + 1))
    return
  fi
  if [[ -n "$body_regex" ]] && ! printf '%s' "$body" | grep -Eq "$body_regex"; then
    echo "FAIL ${name}: body did not match /${body_regex}/ body=${body}"
    fail=$((fail + 1))
    return
  fi
  echo "PASS ${name}"
  pass=$((pass + 1))
}

echo "Verifying ${BASE_URL}"

check healthz 200 '"status":"(ok|degraded)"' \
  "${BASE_URL}/healthz"

check readyz 200 '"status":"ready"' \
  "${BASE_URL}/readyz"

# Clean slate for this flag name (ignore errors)
curl -sS -o /dev/null -X DELETE "${BASE_URL}/v1/flags/${FLAG}" || true

check create_flag 201 '"name":"'"${FLAG}"'"' \
  -X POST "${BASE_URL}/v1/flags" \
  -H 'content-type: application/json' \
  -d "{\"name\":\"${FLAG}\",\"description\":\"verify\",\"enabled\":true,\"rollout_percent\":100}"

check evaluate_on 200 '"enabled":true' \
  "${BASE_URL}/v1/evaluate/${FLAG}?user_id=${USER}"

check kill_switch 200 '"enabled":false' \
  -X PATCH "${BASE_URL}/v1/flags/${FLAG}" \
  -H 'content-type: application/json' \
  -d '{"enabled":false}'

check evaluate_off 200 '"enabled":false' \
  "${BASE_URL}/v1/evaluate/${FLAG}?user_id=${USER}"

check override 200 '"enabled":true' \
  -X PUT "${BASE_URL}/v1/flags/${FLAG}/users/${USER}" \
  -H 'content-type: application/json' \
  -d '{"enabled":true}'

check evaluate_override 200 '"enabled":true|"reason":"OVERRIDE"' \
  "${BASE_URL}/v1/evaluate/${FLAG}?user_id=${USER}"

check bulk 200 '"results"' \
  -X POST "${BASE_URL}/v1/evaluate" \
  -H 'content-type: application/json' \
  -d "{\"user_id\":\"${USER}\",\"flags\":[\"${FLAG}\"]}"

check bad_name 400 '"error"' \
  -X POST "${BASE_URL}/v1/flags" \
  -H 'content-type: application/json' \
  -d '{"name":"bad:name","enabled":true}'

echo
echo "Passed=${pass} Failed=${fail}"
[[ "$fail" -eq 0 ]]
