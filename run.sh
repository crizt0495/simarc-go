#!/usr/bin/env bash
# ===========================================================================
#  SIMARC — Run Script (Linux / macOS / WSL)
#  Auto-detect LAN, build & run with hot-reload (Air)
#  Usage: chmod +x run.sh && ./run.sh
# ===========================================================================
set -euo pipefail
APP_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$APP_DIR"

R='\033[0;31m'; G='\033[0;32m'; Y='\033[1;33m'; C='\033[0;36m'; B='\033[1m'; N='\033[0m'
info()  { echo -e "${C}${B}[INFO]${N}  $*"; }
ok()    { echo -e "${G}${B}[OK]${N}    $*"; }
warn()  { echo -e "${Y}${B}[WARN]${N}  $*"; }
fail()  { echo -e "${R}${B}[FAIL]${N}  $*"; }

echo -e "\n  ${C}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${N}"
echo -e "  ${C}${B}   SIMARC — Arsip Record Center${N}"
echo -e "  ${C}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${N}\n"

# ── 0. Auto-detect LAN IP (cross-platform) ────────────────────────────────
detect_lan_ip() {
    local ip=""
    # Linux: ip command
    if command -v ip &>/dev/null; then
        ip=$(ip -4 addr show | grep -oE 'inet [0-9]+\.[0-9]+\.[0-9]+\.[0-9]+' | grep -v '127.0.0.1' | head -1 | awk '{print $2}')
    fi
    # macOS: ifconfig
    if [[ -z "$ip" ]] && command -v ifconfig &>/dev/null; then
        ip=$(ifconfig | grep -oE 'inet [0-9]+\.[0-9]+\.[0-9]+\.[0-9]+' | grep -v '127.0.0.1' | head -1 | awk '{print $2}')
    fi
    # Fallback: hostname (Linux)
    if [[ -z "$ip" ]] && command -v hostname &>/dev/null; then
        ip=$(hostname -I 2>/dev/null | awk '{print $1}')
    fi
    echo "${ip:-127.0.0.1}"
}
LAN_IP=$(detect_lan_ip)

# ── 1. Check Prerequisites ──────────────────────────────────────────────────
OK=true

# Go
GO_CMD=""
for cmd in go /usr/local/go/bin/go; do
    if command -v "$cmd" &>/dev/null; then
        GO_CMD="$cmd"
        break
    fi
done
if [[ -n "$GO_CMD" ]]; then
    ok "Go $($GO_CMD version | grep -oE 'go[0-9]+\.[0-9]+(\.[0-9]+)?' | tr -d 'go')"
else
    fail "Go belum terinstall."
    echo "  Install: https://go.dev/dl/"
    OK=false
fi

# Air (hot reload)
AIR_CMD=""
for cmd in air "$HOME/go/bin/air"; do
    if command -v "$cmd" &>/dev/null; then
        AIR_CMD="$cmd"
        break
    fi
done
if [[ -n "$AIR_CMD" ]]; then
    ok "Air (auto-reload) tersedia"
else
    warn "Air belum terinstall. Fallback ke build manual."
    warn "  Install: go install github.com/air-verse/air@latest"
fi

# ── 2. Environment ──────────────────────────────────────────────────────────
ENV_FILE=".env"
if [[ ! -f "$ENV_FILE" ]]; then
    if [[ -f .env.example ]]; then
        cp .env.example "$ENV_FILE"
        ok ".env dibuat dari .env.example"
    else
        warn "Membuat .env default..."
        cat > "$ENV_FILE" << 'EOF'
APP_NAME="SIMARC-Arsip Record Center"
APP_URL=http://localhost:8080
APP_PORT=8080
APP_DEBUG=true
APP_TIMEZONE=Asia/Jakarta
DB_HOST=127.0.0.1
DB_PORT=3306
DB_DATABASE=simarc_db
DB_USERNAME=root
DB_PASSWORD=
SESSION_KEY=simarc-default-key
# Backup disimpan di storage/app/backups/database/
EOF
        ok ".env default dibuat"
    fi
fi
set -a; source "$ENV_FILE"; set +a

# ── 3. Database ─────────────────────────────────────────────────────────────
DB_USER="${DB_USERNAME:-root}"
DB_PASS="${DB_PASSWORD:-}"
DB_HOST="${DB_HOST:-127.0.0.1}"
DB_PORT="${DB_PORT:-3306}"
DB_NAME="${DB_DATABASE:-simarc_db}"

# Try mysql then mariadb CLI
MYSQL_CLI=""
for cmd in mysql mariadb; do
    if command -v "$cmd" &>/dev/null; then
        MYSQL_CLI="$cmd"
        break
    fi
done

if [[ -n "$MYSQL_CLI" ]]; then
    CONN="$MYSQL_CLI -u $DB_USER -h $DB_HOST -P $DB_PORT"
    [[ -n "$DB_PASS" ]] && CONN="$CONN -p$DB_PASS"
    if echo "SELECT 1" | timeout 5 $CONN &>/dev/null 2>&1; then
        ok "Database terhubung ($DB_HOST:$DB_PORT)"
        if ! echo "USE \`$DB_NAME\`" | $CONN 2>/dev/null; then
            warn "Database '$DB_NAME' belum ada, membuat..."
            echo "CREATE DATABASE IF NOT EXISTS \`$DB_NAME\` DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;" | $CONN
            ok "Database '$DB_NAME' dibuat"
        fi
    else
        warn "Database tidak terhubung ke $DB_HOST:$DB_PORT"
        warn "  Jalankan: sudo systemctl start mariadb  (Linux)"
        warn "  Atau: brew services start mysql          (macOS)"
    fi
else
    warn "mysql/mariadb CLI tidak ditemukan"
    warn "  Install: sudo apt install mariadb-client   (Linux)"
    warn "  Atau: brew install mysql-client            (macOS)"
fi

# ── 4. Tampilkan Info Sebelum Run ──────────────────────────────────────────
echo ""
PORT="${APP_PORT:-8080}"
echo -e "  ${G}┌────────────────────────────────────────────┐${N}"
echo -e "  ${G}│  ${B}SIMARC SIAP DIGUNAKAN!${N}                    ${G}│${N}"
echo -e "  ${G}│                                            │${N}"
echo -e "  ${G}│  📍  Lokal    ${C}http://localhost:${PORT}${N}           ${G}│${N}"
echo -e "  ${G}│                                            │${N}"
if [[ -n "$AIR_CMD" ]]; then
echo -e "  ${G}│  🔄  Auto-reload aktif                        ${G}│${N}"
fi
echo -e "  ${G}│  ⎈  ${B}Ctrl+C${N} untuk berhenti                     ${G}│${N}"
echo -e "  ${G}└────────────────────────────────────────────┘${N}"
echo ""

# ── 5. Run ──────────────────────────────────────────────────────────────────
if [[ -n "$AIR_CMD" ]]; then
    exec "$AIR_CMD"
else
    info "Build aplikasi..."
    $GO_CMD mod tidy 2>/dev/null
    mkdir -p tmp
    CGO_ENABLED=0 $GO_CMD build -buildvcs=false -ldflags="-s -w" -o ./tmp/simarc-server ./cmd/server/main.go
    ok "Build selesai. Menjalankan server..."
    APP_DEBUG="${APP_DEBUG:-false}" exec ./tmp/simarc-server
fi
