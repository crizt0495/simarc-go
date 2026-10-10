#!/usr/bin/env bash
# ===========================================================================
#  SIMARC — Pasang service systemd (server pusat)
#
#  Menjadikan SIMARC sebagai layanan sistem yang:
#    - otomatis menyala saat komputer dinyalakan
#    - dijalankan ulang otomatis bila crash
#    - berjalan sebagai user pemilik folder aplikasi (agar izin file .env &
#      storage/ konsisten dengan launcher desktop)
#
#  Pemakaian:
#     sudo ./scripts/install-systemd.sh
#     sudo ./scripts/install-systemd.sh --user namauser
#     ./scripts/install-systemd.sh --dry-run        # lihat unit tanpa memasang
# ===========================================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
APP_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
UNIT_NAME="simarc"
UNIT_DST="/etc/systemd/system/${UNIT_NAME}.service"
TEMPLATE="$APP_DIR/deploy/simarc.service"

RUN_USER=""
BIN_OVERRIDE=""
DRY_RUN=0

usage() {
    sed -n '2,20p' "$0" | sed 's/^# \{0,1\}//'
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        --user) RUN_USER="${2:-}"; shift 2 ;;
        --bin) BIN_OVERRIDE="${2:-}"; shift 2 ;;
        --dry-run) DRY_RUN=1; shift ;;
        -h|--help) usage; exit 0 ;;
        *) echo "Opsi tidak dikenal: $1" >&2; usage; exit 1 ;;
    esac
done

if [[ ! -f "$TEMPLATE" ]]; then
    echo "Template tidak ditemukan: $TEMPLATE" >&2
    exit 1
fi

# ── Tentukan user/group layanan ─────────────────────────────────────────────
if [[ -z "$RUN_USER" ]]; then
    if [[ -n "${SUDO_USER:-}" && "${SUDO_USER}" != "root" ]]; then
        RUN_USER="$SUDO_USER"
    else
        RUN_USER="$(stat -c '%U' "$APP_DIR")"
    fi
fi
if ! id "$RUN_USER" >/dev/null 2>&1; then
    echo "User '$RUN_USER' tidak ditemukan di sistem." >&2
    exit 1
fi
RUN_GROUP="$(id -gn "$RUN_USER")"

# ── Tentukan binary server (absolut) ────────────────────────────────────────
arch="$(uname -m)"
case "$arch" in
    x86_64|amd64) arch="amd64" ;;
    aarch64|arm64) arch="arm64" ;;
esac

if [[ -n "$BIN_OVERRIDE" ]]; then
    BIN="$(cd "$(dirname "$BIN_OVERRIDE")" && pwd)/$(basename "$BIN_OVERRIDE")"
    [[ -f "$BIN" ]] || { echo "Binary tidak ditemukan: $BIN"; exit 1; }
else
    BIN="$APP_DIR/tmp/simarc-server"
    if [[ ! -f "$BIN" ]]; then
        rel="$APP_DIR/dist/simarc-server-linux-${arch}"
        if [[ -f "$rel" ]]; then
            echo "Memakai binary rilis: $rel"
            mkdir -p "$APP_DIR/tmp"
            cp -f "$rel" "$BIN"
            chmod +x "$BIN"
            if [[ $EUID -eq 0 ]]; then
                chown "$RUN_USER:$RUN_GROUP" "$BIN"
            fi
        elif command -v go >/dev/null 2>&1; then
            echo "Build binary server..."
            ( cd "$APP_DIR" && mkdir -p tmp && \
              CGO_ENABLED=0 go build -buildvcs=false -ldflags="-s -w" -o "$BIN" ./cmd/server )
            if [[ $EUID -eq 0 ]]; then
                chown "$RUN_USER:$RUN_GROUP" "$BIN"
            fi
        else
            echo "Binary belum ada dan Go tidak terinstall."
            echo "Jalankan ./scripts/build-release.sh di mesin lain, lalu salin folder dist/."
            exit 1
        fi
    fi
fi

# ── Buat isi unit ───────────────────────────────────────────────────────────
tmp_unit="$(mktemp)"
trap 'rm -f "$tmp_unit"' EXIT
sed -e "s|__APP_DIR__|$APP_DIR|g" \
    -e "s|__USER__|$RUN_USER|g" \
    -e "s|__GROUP__|$RUN_GROUP|g" \
    -e "s|__BIN__|$BIN|g" \
    "$TEMPLATE" >"$tmp_unit"

if [[ $DRY_RUN -eq 1 ]]; then
    cat "$tmp_unit"
    exit 0
fi

# ── Pasang ──────────────────────────────────────────────────────────────────
if [[ $EUID -ne 0 ]]; then
    echo "Instalasi service butuh hak root. Jalankan:"
    echo "    sudo $0 $*"
    exit 1
fi

if [[ ! -f "$APP_DIR/.env" ]]; then
    echo "PERINGATAN: $APP_DIR/.env tidak ditemukan — database mungkin gagal tersambung."
fi

install -m 0644 "$tmp_unit" "$UNIT_DST"
systemctl daemon-reload
systemctl enable --now "${UNIT_NAME}.service"
sleep 1
systemctl --no-pager --full status "${UNIT_NAME}.service" || true

echo
echo "Service '${UNIT_NAME}' aktif dan akan menyala otomatis saat komputer dinyalakan."
echo "  Lihat log : journalctl -u ${UNIT_NAME} -f"
echo "  Stop      : sudo systemctl stop ${UNIT_NAME}"
echo "  Nonaktif  : ./scripts/uninstall-systemd.sh"
