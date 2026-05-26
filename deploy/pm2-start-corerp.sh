#!/usr/bin/env bash
set -euo pipefail

ROOT="/home/kelebituo/corerp"
BIN="$ROOT/corerp"
DATA_DIR="$ROOT/data"

pm2 delete corerp >/dev/null 2>&1 || true
pm2 start "$BIN" \
  --name corerp \
  --cwd "$ROOT" \
  -- \
  serve \
  -port 8080 \
  -data "$DATA_DIR" \
  -characters ./characters \
  -secure-cookie=false
pm2 save
