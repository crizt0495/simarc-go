#!/usr/bin/env bash
# ===========================================================================
#  SIMARC — Setup Komputer KLIEN (Linux / macOS / WSL)
#
#  Mengatur komputer ini sebagai KLIEN yang memakai SERVER PUSAT:
#  menulis SIMARC_SERVER_URL ke .env sehingga aplikasi hanya membuka jendela
#  ke server pusat — tanpa menjalankan server dan tanpa database lokal.
#
#  Pemakaian:
#     ./setup-client.sh                              # interaktif (ditanya alamat)
#     ./setup-client.sh http://192.168.1.10:8080     # langsung
# ===========================================================================
set -euo pipefail

APP_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$APP_DIR"

G='\033[0;32m'; Y='\033[1;33m'; R='\033[0;31m'; C='\033[0;36m'; B='\033[1m'; N='\033[0m'
ok()   { echo -e "${G}${B}[OK]${N}    $*"; }
warn() { echo -e "${Y}${B}[WARN]${N}  $*"; }
fail() { echo -e "${R}${B}[FAIL]${N}  $*"; }
info() { echo -e "${C}${B}[INFO]${N}  $*"; }

echo
echo -e "  ${C}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${N}"
echo -e "  ${C}${B}   SIMARC — Setup Komputer Klien${N}"
echo -e "  ${C}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${N}"
echo

# ── 1. Minta / validasi alamat server pusat ─────────────────────────────────
SERVER_URL="${1:-}"
if [[ -z "$SERVER_URL" ]]; then
    read -rp "  Alamat server pusat (contoh http://192.168.1.10:8080): " SERVER_URL
fi
# Buang spasi dan garis miring di akhir.
SERVER_URL="${SERVER_URL#"${SERVER_URL%%[![:space:]]*}"}"
SERVER_URL="${SERVER_URL%"${SERVER_URL##*[![:space:]]}"}"
SERVER_URL="${SERVER_URL%/}"

if [[ ! "$SERVER_URL" =~ ^https?://[^[:space:]]+ ]]; then
    fail "Alamat tidak valid: '$SERVER_URL' (harus dimulai http:// atau https://)."
    exit 1
fi

# ── 2. Buat .env dari template klien bila belum ada ─────────────────────────
if [[ ! -f .env ]]; then
    if [[ -f .env.client.example ]]; then
        cp .env.client.example .env
        ok ".env dibuat dari .env.client.example"
    else
        : > .env
        warn ".env baru dibuat (kosong)."
    fi
fi

# ── 3. Tulis / perbarui SIMARC_SERVER_URL di .env ───────────────────────────
if grep -qE '^[[:space:]]*SIMARC_SERVER_URL=' .env; then
    tmp="$(mktemp)"
    awk -v u="$SERVER_URL" '
        BEGIN { done = 0 }
        /^[[:space:]]*SIMARC_SERVER_URL=/ { print "SIMARC_SERVER_URL=" u; done = 1; next }
        { print }
        END { if (!done) print "SIMARC_SERVER_URL=" u }
    ' .env >"$tmp"
    mv "$tmp" .env
    ok "SIMARC_SERVER_URL diperbarui di .env"
else
    printf '\n# Mode klien (multi-komputer)\nSIMARC_SERVER_URL=%s\n' "$SERVER_URL" >>.env
    ok "SIMARC_SERVER_URL ditambahkan ke .env"
fi

# ── 4. Uji koneksi ke server pusat (tidak menggagalkan setup) ───────────────
info "Menguji koneksi ke ${SERVER_URL} ..."
if command -v curl >/dev/null 2>&1; then
    if curl -fsS --max-time 8 "${SERVER_URL}/ping" >/dev/null 2>&1; then
        ok "Server pusat dapat dihubungi ✔"
    else
        warn "Belum bisa menghubungi ${SERVER_URL}/ping"
        warn "  • Pastikan server pusat menyala dan port-nya (mis. 8080) terbuka di firewall"
        warn "  • Pastikan alamat benar dan sejaringan (LAN), atau melalui VPN/HTTPS"
    fi
else
    warn "curl tidak tersedia; uji koneksi dilewati."
fi

echo
ok "Selesai — komputer ini kini berjalan sebagai KLIEN."
echo "     Jalankan aplikasi:  ./run-desktop.sh   (atau ikon SIMARC)"
echo
