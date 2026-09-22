#!/usr/bin/env bash
# ═══════════════════════════════════════════════════════════════════════════════
# SIMARC — Backup Database Otomatis (aman & terjadwal)
#
# Kegunaan:
#   • Dump database (mysqldump) dengan opsi sama seperti fitur /backup di aplikasi
#   • Simpan ke storage/app/backups/database/  (mode 0600, dir 0700)
#   • Daftarkan di tabel backup_logs  →  otomatis muncul & bisa diunduh di UI /backup
#   • Rotasi: pertahankan N dump terbaru (variabel BACKUP_KEEP, default 14)
#   • Opsional mirror: salin dump ke lokasi lain/eksternal (BACKUP_MIRROR)
#   • Catat log ke storage/logs/backup.log
#
# Kredensial DB dibaca dari .env (TIDAK di-commit, mode 600).
# Aman dipakai via cron, misalnya:
#   15 2 * * * cd /home/chris/Documents/aplikasi/simarc && bash scripts/simarc-backup.sh
# ═══════════════════════════════════════════════════════════════════════════════
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(dirname "$SCRIPT_DIR")"
cd "$ROOT" || exit 1

ENV_FILE="$ROOT/.env"
if [[ ! -f "$ENV_FILE" ]]; then
  echo "[ERROR] $ENV_FILE tidak ditemukan" >&2
  exit 1
fi

# ── baca variabel dari .env (hanya yang dibutuhkan; aman, tidak di-eval) ──────
get_env() { grep -E "^$1=" "$ENV_FILE" 2>/dev/null | tail -1 | cut -d= -f2- | tr -d '"'; }

DB_HOST="${DB_HOST:-$(get_env DB_HOST)}";   DB_HOST="${DB_HOST:-127.0.0.1}"
DB_PORT="${DB_PORT:-$(get_env DB_PORT)}";   DB_PORT="${DB_PORT:-3306}"
DB_DATABASE="${DB_DATABASE:-$(get_env DB_DATABASE)}"; DB_DATABASE="${DB_DATABASE:-simarc}"
DB_USERNAME="${DB_USERNAME:-$(get_env DB_USERNAME)}"; DB_USERNAME="${DB_USERNAME:-root}"
DB_PASSWORD="${DB_PASSWORD:-$(get_env DB_PASSWORD)}"
BACKUP_KEEP="${BACKUP_KEEP:-$(get_env BACKUP_KEEP)}"; BACKUP_KEEP="${BACKUP_KEEP:-14}"
BACKUP_MIRROR="${BACKUP_MIRROR:-$(get_env BACKUP_MIRROR)}"

BUP_DIR="$ROOT/storage/app/backups/database"
LOG_DIR="$ROOT/storage/logs"
LOG_FILE="$LOG_DIR/backup.log"
TS="$(date '+%Y-%m-%d %H:%M:%S')"
FNAME="backup_$(date '+%Y-%m-%d_%H%M%S').sql"
FPATH="$BUP_DIR/$FNAME"

mkdir -p "$BUP_DIR" "$LOG_DIR"
chmod 700 "$BUP_DIR"
touch "$LOG_FILE"

log() { echo "[$TS] $*" >>"$LOG_FILE"; }
fail() { log "GAGAL: $*"; echo "[ERROR] $*" >&2; exit 1; }

# ── pastikan klien db tersedia ─────────────────────────────────────────────────
command -v mysqldump >/dev/null 2>&1 || fail "mysqldump tidak ditemukan di PATH"

# ── dump (MYSQL_PWD: password tidak tampil di ps/argv) ─────────────────────────
log "Dump dimulai: host=$DB_HOST:$DB_PORT db=$DB_DATABASE user=$DB_USERNAME"
export MYSQL_PWD="$DB_PASSWORD"
if : >"$FPATH"; then chmod 600 "$FPATH"; fi   # buat file dulu dgn perms terkunci

if ! mysqldump \
  --host="$DB_HOST" --port="$DB_PORT" --user="$DB_USERNAME" \
  --no-tablespaces --single-transaction --routines --triggers --events \
  "$DB_DATABASE" >"$FPATH" 2>"$FPATH.err"; then
  rm -f "$FPATH" "$FPATH.err"
  fail "mysqldump gagal — baca storage/logs/backup.log"
fi
rm -f "$FPATH.err"

SIZE="$(stat -c%s "$FPATH" 2>/dev/null || echo 0)"
if [[ "$SIZE" -lt 1000 ]]; then
  rm -f "$FPATH"
  fail "dump kosong/terlalu kecil (${SIZE} byte) — kemungkinan kredensial salah"
fi
if command -v gzip >/dev/null 2>&1; then
  gzip -9 -f "$FPATH" && FNAME="$FNAME.gz" && FPATH="$FPATH.gz"
  SIZE="$(stat -c%s "$FPATH")"
fi

log "OK dump: $FNAME (${SIZE} byte)"

# ── daftarkan ke backup_logs agar tampil di UI /backup ─────────────────────────
if command -v mysql >/dev/null 2>&1; then
  REL="storage/app/backups/database/$FNAME"
  mysql --host="$DB_HOST" --port="$DB_PORT" --user="$DB_USERNAME" "$DB_DATABASE" \
    -e "INSERT INTO backup_logs
        (id, filename, file_path, file_size, backup_type, status, notes, created_at, completed_at)
        VALUES (UUID(), '$FNAME', '$REL', $SIZE, 'database', 'success',
                'backup terjadwal (cron)', NOW(), NOW());" \
    >>"$LOG_FILE" 2>&1 \
    || log "PERINGATAN: gagal mencatat ke backup_logs (backup file tetap tersimpan)"
fi

# ── rotasi: sisakan N backup terbaru ───────────────────────────────────────────
mapfile -t OLD < <(ls -1t "$BUP_DIR"/backup_*.sql* 2>/dev/null | tail -n "+$((BACKUP_KEEP + 1))")
if [[ "${#OLD[@]}" -gt 0 ]]; then
  for f in "${OLD[@]}"; do rm -f "$f"; done
  log "Rotasi: hapus ${#OLD[@]} dump lama (> ${BACKUP_KEEP} terbaru)"
fi

# ── mirror opsional ke lokasi eksternal (mis. USB/HDD lain/cloud mount) ────────
if [[ -n "$BACKUP_MIRROR" ]]; then
  if mkdir -p "$BACKUP_MIRROR" && cp -p "$FPATH" "$BACKUP_MIRROR/" 2>/dev/null; then
    chmod 600 "$BACKUP_MIRROR/$FNAME" 2>/dev/null || true
    log "Mirror OK: $BACKUP_MIRROR/$FNAME"
    mapfile -t MOLD < <(ls -1t "$BACKUP_MIRROR"/backup_*.sql* 2>/dev/null | tail -n "+$((BACKUP_KEEP + 1))")
    for f in "${MOLD[@]}"; do rm -f "$f"; done
  else
    log "PERINGATAN: mirror gagal ($BACKUP_MIRROR) — lewati"
  fi
fi

echo "OK: $FNAME (${SIZE} byte)"
exit 0