#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${1:-http://127.0.0.1:8080}"

health_code="$(curl --max-time 5 -s -o /dev/null -w '%{http_code}' "$BASE_URL/api/health")"
ready_code="$(curl --max-time 5 -s -o /dev/null -w '%{http_code}' "$BASE_URL/api/ready")"

if [[ "$health_code" != "200" ]]; then
  echo "smoke failed: /api/health returned $health_code"
  exit 1
fi

if [[ "$ready_code" != "200" ]]; then
  echo "smoke failed: /api/ready returned $ready_code"
  exit 1
fi

echo "smoke ok: /api/health=$health_code /api/ready=$ready_code"
