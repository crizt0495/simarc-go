#!/usr/bin/env bash
# ===========================================================================
#  SIMARC — Desktop App Launcher
#  Menjalankan server Go (tanpa membuka browser) lalu membuka jendela
#  aplikasi mandiri memakai Chromium/Chrome mode "--app" (tanpa tab & URL bar).
#  Saat jendela aplikasi ditutup, server otomatis dimatikan.
#
#  Pemakaian: ./run-desktop.sh
# ===========================================================================
set -euo pipefail

APP_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$APP_DIR"

# ── Konfigurasi ─────────────────────────────────────────────────────────────
if [[ -f .env ]]; then
    set -a
    # shellcheck disable=SC1091
    source .env
    set +a
fi
PORT="${APP_PORT:-8080}"
URL="http://127.0.0.1:${PORT}/"

# Mode jendela aplikasi: normal (default) | maximized | fullscreen | kiosk
WINDOW_MODE="$(printf '%s' "${SIMARC_WINDOW:-normal}" | tr '[:upper:]' '[:lower:]')"
WINDOW_ARGS=()
case "$WINDOW_MODE" in
    maximized)  WINDOW_ARGS+=(--start-maximized) ;;
    fullscreen) WINDOW_ARGS+=(--start-fullscreen) ;;
    kiosk)      WINDOW_ARGS+=(--kiosk) ;;
esac

STATE_DIR="${XDG_STATE_HOME:-$HOME/.local/state}/simarc"
PROFILE_DIR="${XDG_CACHE_HOME:-$HOME/.cache}/simarc/app-profile"
mkdir -p "$STATE_DIR" "$PROFILE_DIR"
LOG="$STATE_DIR/server.log"
CHROME_LOG="$STATE_DIR/chrome.log"
BIN="./tmp/simarc-server"

notify() {
    if command -v notify-send >/dev/null 2>&1; then
        notify-send -a SIMARC "SIMARC" "$1" 2>/dev/null || true
    fi
}

# ── Build bila perlu ────────────────────────────────────────────────────────
need_build=0
if [[ ! -x "$BIN" ]]; then
    need_build=1
elif [[ -n "$(find cmd internal -type f -newer "$BIN" 2>/dev/null | head -n 1)" ]]; then
    need_build=1
fi
if (( need_build )); then
    notify "Menyiapkan aplikasi (build pertama)..."
    if ! go build -buildvcs=false -ldflags="-s -w" -o "$BIN" ./cmd/server/main.go >>"$LOG" 2>&1; then
        notify "Gagal build. Lihat: $LOG"
        exit 1
    fi
fi

# ── Jalankan server (SIMARC_NO_BROWSER mencegah browser default terbuka) ────
SERVER_PID=""
if curl -sf "http://127.0.0.1:${PORT}/ping" >/dev/null 2>&1; then
    : # sudah ada server yang jalan, pakai itu
else
    SIMARC_NO_BROWSER=1 "$BIN" >>"$LOG" 2>&1 &
    SERVER_PID=$!
fi

ready=0
for _ in $(seq 1 120); do
    if curl -sf "http://127.0.0.1:${PORT}/ping" >/dev/null 2>&1; then
        ready=1
        break
    fi
    sleep 0.5
done
if (( ! ready )); then
    notify "Server gagal dijalankan. Lihat: $LOG"
    if [[ -n "${SERVER_PID:-}" ]]; then kill -TERM "$SERVER_PID" 2>/dev/null || true; fi
    exit 1
fi

# ── Cari browser berbasis Chromium ──────────────────────────────────────────
CHROME_BIN=""
for b in google-chrome google-chrome-stable chromium chromium-browser brave-browser microsoft-edge; do
    if command -v "$b" >/dev/null 2>&1; then
        CHROME_BIN="$(command -v "$b")"
        break
    fi
done

# ── Buka jendela aplikasi ───────────────────────────────────────────────────
if [[ -n "$CHROME_BIN" ]]; then
    "$CHROME_BIN" \
        --app="$URL" \
        --user-data-dir="$PROFILE_DIR" \
        --no-first-run \
        --no-default-browser-check \
        --disable-translate \
        --class=SIMARC \
        --name=SIMARC \
        ${WINDOW_ARGS[@]+"${WINDOW_ARGS[@]}"} \
        >>"$CHROME_LOG" 2>&1 &
    APP_PID=$!

    # Tunggu jendela muncul, lalu tunggu sampai ditutup.
    for _ in $(seq 1 40); do
        if pgrep -f -- "--user-data-dir=$PROFILE_DIR" >/dev/null 2>&1; then break; fi
        sleep 0.25
    done
    while pgrep -f -- "--user-data-dir=$PROFILE_DIR" >/dev/null 2>&1; do
        sleep 1
    done
else
    # Fallback bila tidak ada Chromium: pakai browser default
    xdg-open "$URL" >/dev/null 2>&1 || true
fi

# ── Tutup server saat jendela aplikasi ditutup ──────────────────────────────
if [[ -n "${SERVER_PID:-}" ]]; then
    kill -TERM "$SERVER_PID" 2>/dev/null || true
fi
