#!/usr/bin/env bash
set -euo pipefail

SMOKE_WAIT_SECONDS=60
DEFAULT_PHOENIX_HOST_PORT=6006

check() {
  local name="$1" url="$2"
  if curl -fsS "$url" >/dev/null; then
    echo "ok   $name"
  else
    echo "FAIL $name ($url)"; exit 1
  fi
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
check prometheus       http://127.0.0.1:9090/-/ready

body="$(curl -sS http://localhost:8080/does-not-exist)"
echo "$body" | grep -q '"code":"NOT_FOUND"' && echo "ok   error-response-404" || { echo "FAIL error-response-404: $body"; exit 1; }

smoke_id="req_smoke-$(date +%s)-$RANDOM"
curl -fsS -H "X-Request-Id: ${smoke_id}" http://localhost:8080/healthz >/dev/null
wait_for "request id in victorialogs" bash -c \
  "curl -fsS http://127.0.0.1:9428/select/logsql/query --data-urlencode 'query=request_id:\"${smoke_id}\"' | grep -q '${smoke_id}'"
wait_for "http metrics in prometheus" bash -c \
  "curl -fsS 'http://127.0.0.1:9090/api/v1/query?query=http_server_requests_total' | grep -q 'route'"

if curl -fsS http://localhost:8090/healthz >/dev/null 2>&1; then
  echo "ok   ragx-health"
  phoenix_host_port="${PHOENIX_HOST_PORT:-}"
  if [[ -z "$phoenix_host_port" && -f deploy/ragx/.env ]]; then
    phoenix_host_port="$(sed -n 's/^PHOENIX_HOST_PORT=//p' deploy/ragx/.env | head -n 1 | tr -d '\r')"
  fi
  phoenix_host_port="${phoenix_host_port:-$DEFAULT_PHOENIX_HOST_PORT}"
  check phoenix-health "http://localhost:${phoenix_host_port}/healthz"
  ragx_trace="$(curl -fsS -D - -o /dev/null -H "X-Request-Id: ${smoke_id}" http://localhost:8090/healthz | tr -d '\r' | awk -F': ' 'tolower($1)=="x-ragx-trace-id"{print $2}')"
  if [[ "$ragx_trace" =~ ^[0-9a-f]{32}$ ]] && [[ "$ragx_trace" != 00000000000000000000000000000000 ]]; then
    echo "ok   ragx-trace-id ($ragx_trace; open http://localhost:${phoenix_host_port}/redirects/traces/$ragx_trace)"
  else
    echo "FAIL ragx-trace-id"; exit 1
  fi
  check ragx-ready http://localhost:8090/readyz
  dev_key="deploy/ragx/dev-app-signing-key.pem"
  if [[ -f "$dev_key" ]]; then
    ragx_compose="docker compose --env-file deploy/ragx/.env -f deploy/ragx/compose.yaml"
    $ragx_compose exec -T ragx-api ragx seed-dev >/dev/null
    # mint SCOPE REQUEST_ID: ragx rejects a token whose rid differs from the X-Request-Id sent with it.
    mint() {
      "$uv_bin" run --directory ragx python ../scripts/app_token_dev.py mint --key "../$dev_key" \
        --kid "${RAGX_DEV_KID:-dev-1}" --org smoke-org --scope "$1" --rid "$2" | tr -d '\r\n'
    }
    provision_body='{"name":"Smoke","externalParsing":true}'
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
    upload_id="req_smoke-upload-$(date +%s)-$RANDOM"
    # The file goes through stdin: Git Bash does not convert a /tmp path inside curl's -F value for Windows curl.
    upload_token="$(mint kb:write "$upload_id")"
    upload_response="$(printf 'smoke %s\n' "$upload_id" | curl -sS -w '\n%{http_code}' \
      -H "Authorization: Bearer ${upload_token}" -H "X-Request-Id: ${upload_id}" -F "file=@-;filename=smoke.txt" \
      "http://localhost:8090/v1/knowledge-bases/${kb_first}/files")"
    upload_status="${upload_response##*$'\n'}"
    upload_file_id="$(echo "$upload_response" | sed -n 's/.*"id":"\([^"]*\)".*/\1/p' | head -n 1)"
    [[ "$upload_status" == 201 ]] && echo "ok   ragx-upload" || { echo "FAIL ragx-upload ($upload_status)"; exit 1; }
    wait_for "ragx worker handled upload" bash -c \
      "$ragx_compose logs --since 5m ragx-worker | grep '${upload_id}' | grep -q 'job handled'"
    # Remove the smoke file so repeated runs do not pile up data in smoke-org (the purge job deletes the object).
    cleanup_id="req_smoke-cleanup-$(date +%s)-$RANDOM"
    curl -fsS -o /dev/null -X DELETE -H "Authorization: Bearer $(mint kb:write "$cleanup_id")" \
      -H "X-Request-Id: ${cleanup_id}" \
      "http://localhost:8090/v1/knowledge-bases/${kb_first}/files/${upload_file_id}" && echo "ok   ragx-upload-cleanup" \
      || { echo "FAIL ragx-upload-cleanup"; exit 1; }
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
