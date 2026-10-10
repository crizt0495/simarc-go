#!/usr/bin/env bash
# ===========================================================================
#  SIMARC — Desktop App Launcher (Linux)
#  Menjalankan server Go lokal (tanpa membuka browser) lalu membuka jendela
#  aplikasi mandiri memakai Chromium/Chrome mode "--app" (tanpa tab & URL bar).
#  Menutup jendela aplikasi otomatis mematikan server.
#
#  Mode KLIEN: bila SIMARC_SERVER_URL diisi, server lokal tidak dijalankan;
#  jendela langsung diarahkan ke server pusat (satu DB untuk banyak komputer).
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

# ── Multi-komputer: mode KLIEN (server pusat) ───────────────────────────────
CLIENT_URL="${SIMARC_SERVER_URL:-}"
CLIENT_URL="${CLIENT_URL%/}"
if [[ -n "$CLIENT_URL" ]]; then
    URL="${CLIENT_URL}/"
fi

STATE_DIR="${XDG_STATE_HOME:-$HOME/.local/state}/simarc"
PROFILE_DIR="${XDG_CACHE_HOME:-$HOME/.cache}/simarc/app-profile"
mkdir -p "$STATE_DIR" "$PROFILE_DIR"
LOG="$STATE_DIR/server.log"
CHROME_LOG="$STATE_DIR/chrome.log"
PIDFILE="$STATE_DIR/server.pid"
LOCKFILE="$STATE_DIR/launch.lock"
BIN="./tmp/simarc-server"

SERVER_PID=""

notify() {
    if command -v notify-send >/dev/null 2>&1; then
        notify-send -a SIMARC "SIMARC" "$1" 2>/dev/null || true
    fi
}

log() { printf '%s %s\n' "$(date '+%Y-%m-%d %H:%M:%S')" "$*" >>"$LOG"; }

cleanup() {
    if [[ -n "${SERVER_PID:-}" ]] && kill -0 "$SERVER_PID" 2>/dev/null; then
        kill -TERM "$SERVER_PID" 2>/dev/null || true
    fi
    rm -f "$PIDFILE" 2>/dev/null || true
}

on_signal() {
    cleanup
    exit 130
}

trap cleanup EXIT
trap on_signal INT TERM HUP

# ── Pilih binary server: lokal terbaru, binary rilis, atau build dari sumber ─
up_to_date_bin() {
    [[ -x "$BIN" ]] && [[ -z "$(find cmd internal -type f -newer "$BIN" 2>/dev/null | head -n1)" ]]
}

resolve_bin() {
    if up_to_date_bin; then
        return 0
    fi
    local arch rel
    arch="$(uname -m)"
    case "$arch" in
        x86_64|amd64) arch="amd64" ;;
        aarch64|arm64) arch="arm64" ;;
    esac
    rel="dist/simarc-server-linux-${arch}"
    if [[ -f "$rel" ]]; then
        mkdir -p tmp
        cp -f "$rel" "$BIN"
        chmod +x "$BIN"
        log "Memakai binary rilis: $rel"
        return 0
    fi
    if command -v go >/dev/null 2>&1; then
        notify "Menyiapkan aplikasi (build pertama)..."
        log "Build dari sumber..."
        mkdir -p tmp
        if go build -buildvcs=false -ldflags="-s -w" -o "$BIN" ./cmd/server >>"$LOG" 2>&1; then
            return 0
        fi
        log "Build dari sumber gagal."
        return 1
    fi
    return 1
}

server_ready() { curl -sf "http://127.0.0.1:${PORT}/ping" >/dev/null 2>&1; }

wait_server() {
    local i
    for i in $(seq 1 120); do
        server_ready && return 0
        sleep 0.5
    done
    return 1
}

# Pastikan PID di pidfile benar-benar proses simarc-server (hindari PID daur ulang).
pid_is_simarc() {
    local p="$1"
    [[ -n "$p" ]] || return 1
    kill -0 "$p" 2>/dev/null || return 1
    tr '\0' ' ' <"/proc/$p/cmdline" 2>/dev/null | grep -q 'simarc-server'
}

# Adopsi server yang sudah jalan (agar bisa dimatikan saat jendela ditutup,
# termasuk server yatim dari sesi sebelumnya).
adopt_server() {
    [[ -f "$PIDFILE" ]] || return 0
    local p
    p="$(cat "$PIDFILE" 2>/dev/null || true)"
    if pid_is_simarc "$p"; then
        SERVER_PID="$p"
    else
        rm -f "$PIDFILE"
    fi
}

# ── Jalankan server lokal (dilewati pada mode KLIEN) ────────────────────────
if [[ -z "$CLIENT_URL" ]]; then
    if server_ready; then
        adopt_server
    else
        # Serialkan peluncuran agar dua klik tidak menjalankan dua server.
        if command -v flock >/dev/null 2>&1; then
            exec 9>"$LOCKFILE"
            flock 9
        fi
        if ! server_ready; then
            if ! resolve_bin; then
                notify "Gagal menyiapkan aplikasi. Lihat: $LOG"
                exit 1
            fi
            log "Menjalankan server pada port ${PORT}..."
            SIMARC_NO_BROWSER=1 "$BIN" >>"$LOG" 2>&1 &
            SERVER_PID=$!
            printf '%s\n' "$SERVER_PID" >"$PIDFILE"
        fi
        if command -v flock >/dev/null 2>&1; then
            flock -u 9 2>/dev/null || true
        fi
    fi

    if ! wait_server; then
        notify "Server gagal dijalankan. Lihat: $LOG"
        exit 1
    fi
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

    # Tunggu jendela muncul (maks ~10 detik).
    appeared=0
    for _ in $(seq 1 40); do
        if pgrep -f -- "--user-data-dir=$PROFILE_DIR" >/dev/null 2>&1; then
            appeared=1
            break
        fi
        sleep 0.25
    done

    if (( ! appeared )); then
        notify "Gagal membuka jendela aplikasi. Lihat: $CHROME_LOG"
        exit 1
    fi

    # Tunggu sampai SEMUA jendela aplikasi (profil ini) ditutup.
    while pgrep -f -- "--user-data-dir=$PROFILE_DIR" >/dev/null 2>&1; do
        sleep 1
    done
else
    # Fallback tanpa Chromium: pakai browser default dan biarkan server hidup
    # selama server berjalan (Ctrl+C untuk berhenti).
    notify "Chrome/Chromium tidak ditemukan; membuka browser default."
    xdg-open "$URL" >/dev/null 2>&1 || true
    if [[ -n "${SERVER_PID:-}" ]]; then
        wait "$SERVER_PID" 2>/dev/null || true
    fi
fi
