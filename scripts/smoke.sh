#!/usr/bin/env bash
set -euo pipefail

SMOKE_WAIT_SECONDS=60
DEFAULT_PHOENIX_HOST_PORT=6006
DEFAULT_PROMETHEUS_HOST_PORT=9090

check() {
  local name="$1" url="$2"
  if curl -fsS "$url" >/dev/null; then
    echo "ok   $name"
  else
    echo "FAIL $name ($url)"; exit 1
  fi
}

# host_port VAR ENV_FILE DEFAULT: the process environment wins, then the stack's .env, then the compose default.
host_port() {
  local var="$1" file="$2" fallback="$3" value="${!1:-}"
  if [[ -z "$value" && -f "$file" ]]; then
    value="$(sed -n "s/^${var}=//p" "$file" | head -n 1 | tr -d '\r')"
  fi
  echo "${value:-$fallback}"
}

# wait_for NAME COMMAND: retries COMMAND every 2 s for SMOKE_WAIT_SECONDS (log batches and scrapes are periodic).
wait_for() {
  local name="$1"; shift
  local deadline=$((SECONDS + SMOKE_WAIT_SECONDS))
  until "$@" >/dev/null 2>&1; do
    if (( SECONDS >= deadline )); then echo "FAIL $name"; exit 1; fi
    sleep 2
  done
  echo "ok   $name"
}

if command -v uv >/dev/null 2>&1; then
  uv_bin=uv
elif command -v uv.exe >/dev/null 2>&1; then
  uv_bin=uv.exe
else
  echo "FAIL identity smoke: uv is unavailable"; exit 1
fi

check core-health      http://localhost:8080/healthz
check core-ready       http://localhost:8080/readyz
check grafana          http://localhost:3001/api/health
check victorialogs     http://127.0.0.1:9428/health
prometheus_host_port="$(host_port PROMETHEUS_HOST_PORT deploy/core/.env "$DEFAULT_PROMETHEUS_HOST_PORT")"
check prometheus       "http://127.0.0.1:${prometheus_host_port}/-/ready"

body="$(curl -sS http://localhost:8080/does-not-exist)"
echo "$body" | grep -q '"code":"NOT_FOUND"' && echo "ok   error-response-404" || { echo "FAIL error-response-404: $body"; exit 1; }

smoke_id="req_smoke-$(date +%s)-$RANDOM"
curl -fsS -H "X-Request-Id: ${smoke_id}" http://localhost:8080/healthz >/dev/null
wait_for "request id in victorialogs" bash -c \
  "curl -fsS http://127.0.0.1:9428/select/logsql/query --data-urlencode 'query=request_id:\"${smoke_id}\"' | grep -q '${smoke_id}'"
wait_for "http metrics in prometheus" bash -c \
  "curl -fsS 'http://127.0.0.1:${prometheus_host_port}/api/v1/query?query=http_server_requests_total' | grep -q 'route'"

if curl -fsS http://localhost:8090/healthz >/dev/null 2>&1; then
  echo "ok   ragx-health"
  phoenix_host_port="$(host_port PHOENIX_HOST_PORT deploy/ragx/.env "$DEFAULT_PHOENIX_HOST_PORT")"
  check phoenix-health "http://localhost:${phoenix_host_port}/healthz"
  ragx_trace="$(curl -fsS -D - -o /dev/null -H "X-Request-Id: ${smoke_id}" http://localhost:8090/healthz | tr -d '\r' | awk -F': ' 'tolower($1)=="x-ragx-trace-id"{print $2}')"
  if [[ "$ragx_trace" =~ ^[0-9a-f]{32}$ ]] && [[ "$ragx_trace" != 00000000000000000000000000000000 ]]; then
    echo "ok   ragx-trace-id ($ragx_trace; open http://localhost:${phoenix_host_port}/redirects/traces/$ragx_trace)"
  else
    echo "FAIL ragx-trace-id"; exit 1
  fi
  check ragx-ready http://localhost:8090/readyz
  check ragx-ui-login http://localhost:8090/ui/login
  check ragx-ui-css   http://localhost:8090/ui/static/ragx.css
  dev_key="deploy/ragx/dev-app-signing-key.pem"
  if [[ -f "$dev_key" ]]; then
    ragx_compose="docker compose --env-file deploy/ragx/.env -f deploy/ragx/compose.yaml"
    $ragx_compose exec -T ragx-api ragx seed-dev >/dev/null
    # mint SCOPE REQUEST_ID: ragx rejects a token whose rid differs from the X-Request-Id sent with it.
    mint() {
      "$uv_bin" run --directory ragx python ../scripts/app_token_dev.py mint --key "../$dev_key" \
        --kid "${RAGX_DEV_KID:-dev-1}" --org smoke-org --scope "$1" --rid "$2" | tr -d '\r\n'
    }
    provision_body='{"name":"Smoke","externalParsing":true,"settingsVersion":1}'
    provision() {
      local rid="req_smoke-provision-$(date +%s)-$RANDOM"
      curl -fsS -X PUT -H "Authorization: Bearer $(mint org:provision "$rid")" -H "X-Request-Id: ${rid}" \
        -H 'Content-Type: application/json' -d "$provision_body" http://localhost:8090/v1/orgs/smoke-org | sed -n 's/.*"defaultKbId":"\([^"]*\)".*/\1/p'
    }
    kb_first="$(provision)"
    kb_second="$(provision)"
    if [[ -n "$kb_first" && "$kb_first" == "$kb_second" ]]; then
      echo "ok   ragx-provision-idempotent"
    else
      echo "FAIL ragx-provision-idempotent"; exit 1
    fi
  else
    echo "skip ragx-provision (run task ragx:dev-key and put the printed RAG_CORE_PUBLIC_KEYS in deploy/ragx/.env)"
  fi
else
  if [[ -f deploy/ragx/.env ]] && docker compose --env-file deploy/ragx/.env -f deploy/ragx/compose.yaml ps --all --services ragx-api | grep -qx ragx-api; then
    echo "FAIL ragx-health (ragx API container exists but health endpoint failed)"; exit 1
  fi
  echo "skip ragx-health (ragx stack not running; start it with task up:ragx)"
fi

"$uv_bin" run --directory ragx python ../scripts/smoke_identity.py

if curl -fsS http://localhost:8090/healthz >/dev/null 2>&1 && [[ -f deploy/ragx/dev-app-signing-key.pem ]]; then
  "$uv_bin" run --directory ragx python ../scripts/smoke_ragx_app.py
else
  echo "skip ragx-app-through-core (needs task ragx:dev-key, task up:ragx and the core stack)"
fi
