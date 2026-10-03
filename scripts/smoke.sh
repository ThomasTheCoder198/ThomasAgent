#!/usr/bin/env bash
set -euo pipefail

check() {
  local name="$1" url="$2"
  if curl -fsS "$url" >/dev/null; then
    echo "ok   $name"
  else
    echo "FAIL $name ($url)"; exit 1
  fi
}

check core-health   http://localhost:8080/healthz
check core-ready    http://localhost:8080/readyz
check rag-health    http://localhost:8090/healthz
check grafana       http://localhost:3001/api/health
check langfuse      http://localhost:3002/api/public/health

body="$(curl -sS http://localhost:8080/does-not-exist)"
echo "$body" | grep -q '"code":"NOT_FOUND"' && echo "ok   envelope-404" || { echo "FAIL envelope-404: $body"; exit 1; }
