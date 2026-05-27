#!/usr/bin/env bash
set -uo pipefail

# CoreRP Health Check & Patrol Script
# Usage: ./health-check.sh [base_url]
# Checks: process status, API endpoints, SQLite integrity, disk space

BASE_URL="${1:-http://127.0.0.1:8080}"
ROOT="/home/kelebituo/corerp"
DATA_DIR="$ROOT/data"
DB_FILE="$DATA_DIR/memory.db"
PASS=0
FAIL=0
WARN=0

ok()   { echo "  [OK]   $1"; ((PASS++)); }
fail() { echo "  [FAIL] $1"; ((FAIL++)); }
warn() { echo "  [WARN] $1"; ((WARN++)); }

echo "=== CoreRP Health Check ==="
echo "Target: $BASE_URL"
echo "Time:   $(date -u '+%Y-%m-%d %H:%M:%S UTC')"
echo ""

# --- 1. Process Status ---
echo "[Process]"

# Check PM2
if command -v pm2 &>/dev/null; then
  pm2_status=$(pm2 jlist 2>/dev/null | python3 -c "
import sys,json
try:
    apps=json.load(sys.stdin)
    for a in apps:
        if a['name']=='corerp':
            print(a['pm2_env']['status'],end='')
            break
    else:
        print('not_found',end='')
except: print('error',end='')
" 2>/dev/null || echo "error")
  if [[ "$pm2_status" == "online" ]]; then
    ok "PM2 corerp is online"
  elif [[ "$pm2_status" == "not_found" ]]; then
    warn "PM2 corerp not found (may use systemd)"
  else
    fail "PM2 corerp status: $pm2_status"
  fi
else
  warn "PM2 not installed"
fi

# Check systemd (skip warn if PM2 already confirmed online)
if command -v systemctl &>/dev/null; then
  svc_status=$(systemctl is-active corerp.service 2>/dev/null) || true
  svc_status=${svc_status:-inactive}
  if [[ "$svc_status" == "active" ]]; then
    ok "systemd corerp.service is active"
  elif [[ "$svc_status" == "inactive" ]] && [[ "$pm2_status" == "online" ]]; then
    : # PM2 is handling it, no need to warn
  elif [[ "$svc_status" == "inactive" ]]; then
    warn "systemd corerp.service inactive (may use PM2)"
  else
    fail "systemd corerp.service: $svc_status"
  fi
fi

# Check port listener
if command -v ss &>/dev/null; then
  if ss -tlnp 2>/dev/null | grep -q ':8080'; then
    ok "Port 8080 is listening"
  else
    fail "Port 8080 is not listening"
  fi
fi

echo ""

# --- 2. API Endpoints (public) ---
echo "[API Endpoints]"

for path in /api/health /api/ready; do
  code=$(curl --max-time 5 -s -o /dev/null -w '%{http_code}' "$BASE_URL$path" 2>/dev/null || echo "000")
  if [[ "$code" == "200" ]]; then
    ok "$path → $code"
  else
    fail "$path → $code (expected 200)"
  fi
done

# Auth-protected endpoints — try login first, then check with cookie
COOKIE_JAR=$(mktemp)
trap "rm -f $COOKIE_JAR" EXIT

login_code=$(curl --max-time 5 -s -o /dev/null -w '%{http_code}' \
  -c "$COOKIE_JAR" -d 'password=admin' "$BASE_URL/login" 2>/dev/null || echo "000")

if [[ "$login_code" == "200" || "$login_code" == "302" || "$login_code" == "303" ]]; then
  ok "/login → $login_code"
  for path in /api/state /api/worlds /api/characters; do
    code=$(curl --max-time 5 -s -o /dev/null -w '%{http_code}' -b "$COOKIE_JAR" "$BASE_URL$path" 2>/dev/null || echo "000")
    if [[ "$code" == "200" ]]; then
      ok "$path → $code"
    else
      fail "$path → $code (expected 200)"
    fi
  done
else
  warn "/login → $login_code, skipping auth-protected endpoint checks"
fi

echo ""

# --- 3. Database ---
echo "[Database]"

if [[ -f "$DB_FILE" ]]; then
  size=$(du -h "$DB_FILE" | cut -f1)
  ok "SQLite file exists ($size)"

  if command -v sqlite3 &>/dev/null; then
    integrity=$(sqlite3 "$DB_FILE" "PRAGMA integrity_check;" 2>/dev/null)
    if [[ "$integrity" == "ok" ]]; then
      ok "SQLite integrity check passed"
    else
      fail "SQLite integrity: $integrity"
    fi

    event_count=$(sqlite3 "$DB_FILE" "SELECT COUNT(*) FROM events;" 2>/dev/null || echo "?")
    ok "Events table: $event_count rows"
  else
    warn "sqlite3 not available, skipping integrity check"
  fi
else
  fail "SQLite file not found: $DB_FILE"
fi

echo ""

# --- 4. Disk Space ---
echo "[Disk]"

disk_pct=$(df "$ROOT" 2>/dev/null | awk 'NR==2{print $5}' | tr -d '%')
if [[ -n "$disk_pct" ]]; then
  if (( disk_pct > 95 )); then
    fail "Disk usage: ${disk_pct}%"
  elif (( disk_pct > 85 )); then
    warn "Disk usage: ${disk_pct}%"
  else
    ok "Disk usage: ${disk_pct}%"
  fi
fi

echo ""

# --- 5. Reverse Proxy ---
echo "[Reverse Proxy]"

if command -v nginx &>/dev/null; then
  nginx_test=$(nginx -t 2>&1)
  if echo "$nginx_test" | grep -q "successful"; then
    ok "Nginx config valid"
  elif echo "$nginx_test" | grep -q "Permission denied"; then
    warn "Nginx config test needs root (permission denied)"
  else
    fail "Nginx config invalid"
  fi

  if systemctl is-active nginx &>/dev/null; then
    ok "Nginx is running"
  else
    warn "Nginx is not running"
  fi
else
  warn "Nginx not installed"
fi

echo ""

# --- Summary ---
echo "=== Summary ==="
echo "  PASS: $PASS  FAIL: $FAIL  WARN: $WARN"
if (( FAIL > 0 )); then
  echo "  Status: UNHEALTHY"
  exit 1
elif (( WARN > 0 )); then
  echo "  Status: DEGRADED"
  exit 0
else
  echo "  Status: HEALTHY"
  exit 0
fi
