#!/bin/bash
# ═══════════════════════════════════════════════════════════════════
#  SIMARC — Jalan Aplikasi Desktop (Wails) + Support LAN / Wi-Fi
#  ───────────────────────────────────────────────────────────────────
#  Gunakan:
#    ./run.sh                — auto detect & jalan local
#    ./run.sh 8080           — jalan dengan port tertentu
#    ./run.sh --web          — alternatif web server mode (LAN)
#    ./run.sh --lan          — aktifkan binding ke semua interface
# ═══════════════════════════════════════════════════════════════════

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

# ── 1. Cari dan tampilkan IP publik / LAN ─────────────────────────────
DISPLAY_IP=$(ip -4 addr show eth0 2>/dev/null | grep inet | awk '{print $2}' | head -1 | cut -d/ -f1 || true)
if [ -z "$DISPLAY_IP" ]; then
  DISPLAY_IP=$(hostname -I 2>/dev/null | awk '{print $1}' || true)
fi
echo "=== Alamat jaringan ==="
echo "Host:     $HOSTNAME"
echo "IP LAN:   ${DISPLAY_IP:-tidak ditemukan}"
echo "Root:     $(realpath /)"

# ── 2. Parse argumen (port / mode) ───────────────────────────────────
PORT=""
WEB_MODE=false
while [ $# -gt 0 ]; do
  case "$1" in
    --web) WEB_MODE=true; shift ;;
    -p|--port=*) PORT="${1#*=}"; shift ;;
    *) echo "Arg: $1"; shift ;;
  esac
done

APP_PORT="${PORT:-${SIMARC_APP_PORT:-8080}}"
APP_HOST="${SIMARC_HOST:-}"

export APP_NAME="${APP_NAME:-SIMARC-Arsip Record Center}"
export APP_URL="${APP_URL:-http://localhost:${APP_PORT}}"
export APP_PORT="${APP_PORT}"
export APP_DEBUG="${APP_DEBUG:-true}"
export APP_TIMEZONE="${APP_TIMEZONE:-Asia/Jakarta}"
export SIMARC_WINDOW="${SIMARC_WINDOW:-fullscreen}"
export SIMARC_SERVER_URL="${SIMARC_SERVER_URL:-}"
export SIMARC_EMBEDDED_DB="${SIMARC_EMBEDDED_DB:-}"
export SESSION_KEY="${SESSION_KEY:-}"

export DB_HOST="${DB_HOST:-127.0.0.1}"
export DB_PORT="${DB_PORT:-3306}"
export DB_DATABASE="${DB_DATABASE:-simarc}"
export DB_USERNAME="${DB_USERNAME:-chris}"
export DB_PASSWORD="${DB_PASSWORD:-04}"
export DB_TLS="${DB_TLS:-preferred}"
export DB_DRIVER="${DB_DRIVER:-mysql}"
export DB_PATH="${DB_PATH:-storage/simarc.db}"
export SYNC_ENABLED="${SYNC_ENABLED:-false}"
export SYNC_NODE_ID="${SYNC_NODE_ID:-}"
export SYNC_INTERVAL_MINUTES="${SYNC_INTERVAL_MINUTES:-15}"
export SYNC_HUB_DB_HOST="${SYNC_HUB_DB_HOST:-}"
export SYNC_HUB_DB_PORT="${SYNC_HUB_DB_PORT:-3306}"
export SYNC_HUB_DB_NAME="${SYNC_HUB_DB_NAME:-}"
export SYNC_HUB_DB_USER="${SYNC_HUB_DB_USER:-}"
export SYNC_HUB_DB_PASS="${SYNC_HUB_DB_PASS:-}"
export SYNC_HUB_DB_TLS="${SYNC_HUB_DB_TLS:-preferred}"
export SYNC_FILES_ENABLED="${SYNC_FILES_ENABLED:-true}"
export SYNC_FILES_MAX_MB="${SYNC_FILES_MAX_MB:-128}"
export GOOGLE_DRIVE_CLIENT_ID="${GOOGLE_DRIVE_CLIENT_ID:-}"
export GOOGLE_DRIVE_FOLDER_ID="${GOOGLE_DRIVE_FOLDER_ID:-}"

echo ""
echo "=== Konfigurasi SIMARC ==="
echo "  APP_NAME         : $APP_NAME"
echo "  APP_PORT         : $APP_PORT"
echo "  SYNC_ENABLED     : $SYNC_ENABLED"
echo ""

FRONTEND_DIR="$SCRIPT_DIR/frontend/dist"

# ── 3. Mode Web Server (buka via browser LAN) ───────────────────────
# Server HTTP — fully detached agar survive penutup terminal.
# Ketikkan 'pkill -f python.*http.server' kalau mau matikan.
if [ "$WEB_MODE" = true ] || [ -z "$DISPLAY_IP" ]; then
  echo "=== Mode Web Server (buka via browser LAN) ==="
  echo "Menjalankan HTTP server di port :$APP_PORT."
  echo "Kalau mau keluar ketikkan: pkill -f 'python.*http.server'"
  echo ""
  if [ ! -d "$FRONTEND_DIR" ]; then
    echo "!!! Frontend tidak ada di $FRONTEND_DIR !!!"
    echo "Buat minimal: mkdir -p $FRONTEND_DIR && echo '<html></html>' > $FRONTEND_DIR/index.html"
    exit 1
  fi
  setsid python3 -m http.server \
    "$APP_PORT" \
    --bind 0.0.0.0 \
    --directory "$FRONTEND_DIR" \
    > /tmp/simarc_http.log 2>&1 < /dev/null &
  sleep 2
  echo "Server hidup di :$APP_PORT."
  echo "Kalau mau keluar ketikkan: pkill -f 'python.*http.server'"
  echo ""
  echo "Open browser di komputer lain di LAN:"
  echo "  http://$DISPLAY_IP:$APP_PORT"
  exit 0
fi

# ── 4. Mode desktop Wails (default) ───────────────────────────────────
echo "=== Jalan Aplikasi Desktop (Wails) ==="
echo "Ketikkan perintah berikut di komputer lain agar bisa buka via SSH+X11:"
echo ""
echo "  ssh -X chris@\"$DISPLAY_IP\" 'cd ~/Documents/aplikasi/simarc && SIMARC_WINDOW=fullscreen go run .'"
echo ""
echo "Kalau ingin jalan langsung di sini (akan menampilkan window Wails):"
echo "  ./run.sh"
echo ""

command -v go >/dev/null 2>&1 || { echo "!!! Go tidak terinstall !!!"; exit 1; }

export GOFLAGS="-tags=release"
export CGO_ENABLED=1
export GOPATH="${GOPATH:-$(go env GOPATH)}"
export PATH="${PATH}:${GOPATH}/bin"

mysql -u "$DB_USERNAME" -p"$DB_PASSWORD" -h "$DB_HOST" -e "SELECT 1;" 2>/dev/null \
  && echo "Database terkoneksi OK ✅" \
  || echo "!!! Database tidak terkoneksi — cek kredensial .env !!!"

SIMARC_DB_HOST="$DB_HOST" SIMARC_DB_USER="$DB_USERNAME" SIMARC_DB_PASS="$DB_PASSWORD" SIMARC_DB_NAME="$DB_DATABASE" SIMARC_WINDOW="fullscreen" go run .
