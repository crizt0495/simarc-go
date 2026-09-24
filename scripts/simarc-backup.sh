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

# ── AIVEN: impor dump ke MySQL Aiven (backup offsite / DR target) ──────────────
# Aktif otomatis bila AIVEN_HOST & AIVEN_PASSWORD terisi di .env.
# Strategi: DROP + CREATE + import penuh → salinan selalu utuh & idempoten.
# Koneksi memakai --ssl (TLS terenkripsi). Catat ke backup.log; gagal → exit 1
# (agar cron/monitoring terlihat), backup lokal sudah aman tersimpan.
AIVEN_HOST="${AIVEN_HOST:-$(get_env AIVEN_HOST)}"
AIVEN_PORT="${AIVEN_PORT:-$(get_env AIVEN_PORT)}"
AIVEN_USERNAME="${AIVEN_USERNAME:-$(get_env AIVEN_USERNAME)}"
AIVEN_PASSWORD="${AIVEN_PASSWORD:-$(get_env AIVEN_PASSWORD)}"
AIVEN_DATABASE="${AIVEN_DATABASE:-$(get_env AIVEN_DATABASE)}"
AIVEN_DATABASE="${AIVEN_DATABASE:-simarc_db}"

decompress() { case "$1" in *.gz) gzip -dc "$1" ;; *) cat "$1" ;; esac; }
# MySQL 8.4 menolak `DEFAULT uuid()` tanpa kurung (dump MariaDB) → normalisasi.
normalize_mysql8() { sed -e 's/DEFAULT uuid()/DEFAULT (uuid())/g'; }

if [[ -n "$AIVEN_HOST" && -n "$AIVEN_PASSWORD" ]]; then
  log "Aiven push dimulai: host=$AIVEN_HOST:$AIVEN_PORT db=$AIVEN_DATABASE"
  MYSQLOPTS=("--ssl" "--host=$AIVEN_HOST" "--port=$AIVEN_PORT" "--user=$AIVEN_USERNAME" "--max-allowed-packet=512M")
  # Pra-cek: lewati bila layanan Aiven read-only (mode standby DR) —
  # hindari banjir log ERROR 1290; beri pesan jelas ke backup.log & stdout.
  AIVEN_RO="$(MYSQL_PWD="$AIVEN_PASSWORD" mysql "${MYSQLOPTS[@]}" -N -e "SELECT @@read_only" 2>>"$LOG_FILE" || echo '?')"
  if [[ "$AIVEN_RO" == "1" ]]; then
    log "Aiven DILEWATI: layanan read-only (@@read_only=1). Matikan read-only di konsol Aiven agar impor aktif."
    echo "Aiven dilewati (read-only; matikan di konsol Aiven untuk mengaktifkan impor)"
  else
    # DROP/CREATE diberi lock_wait_timeout pendek agar TIDAK pernah menggantung
    # selamanya menunggu metadata-lock dari sesi impor/query lain yang macet
    # (fenomena teramati: impor lama yg terkubur memegang lock tabel berjam-jam
    # dan memblokir run berikutnya; dengan timeout ini run GAGAL cepat lalu
    # bisa dicoba ulang/besihkan sesi stale, bukan diam membisu).
    if ! MYSQL_PWD="$AIVEN_PASSWORD" mysql "${MYSQLOPTS[@]}" \
         --connect-timeout=60 \
         --init-command="SET SESSION lock_wait_timeout=60;" \
         -e "DROP DATABASE IF EXISTS \`$AIVEN_DATABASE\`; CREATE DATABASE \`$AIVEN_DATABASE\` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;" >>"$LOG_FILE" 2>&1; then
      log "Aiven GAGAL: drop/create database $AIVEN_DATABASE (mungkin terkunci sesi lain)"
      fail "Aiven: gagal drop/create database $AIVEN_DATABASE"
    fi
    # Dump Aiven dibuat TERPISAH dari $FPATH:
    #   • --skip-extended-insert → dump lokal ANDAL (bukan penyebab lambat).
    #   • --ignore-table=telescope_* → buang log debug internal Laravel.
    #   • Setelah dump: baris INSERT per-baris DIGABUNG menjadi multi-row
    #     (merge_mysql_inserts.py, cap ~800KB/statement). Tanpa penggabungan,
    #     impor TLS Aiven terikat RTT ~150ms/statement → ±100rb statement =
    #     BERJAM-JAM (teramati: 2 jam baru 40%). Dengan gabungan hanya puluhan
    #     statement → seluruh DB (<36MB) selesai < 2 menit. Satu baris raksasa
    #     (>10MB, mis. notes backup_logs) tetap aman karena pipeline lama sudah
    #     terbukti mengangkutnya.
    #   • timeouts & max_allowed_packet besar pada sesi impor (Aiven menutup
    #     koneksi TLS bila impor lama); lock_wait_timeout=60 utk gagal-cepat.
    # Data arsip (arsip, pemberkasan, klasifikasi, dll) tetap 100% ikut.
    AIVEN_TMP="$(mktemp "$BUP_DIR/aiven_push_XXXXXX.sql")"
    # bersihkan aiven_push_* basi (sisa proses yang di-kill; ~36MB/keping)
    find "$BUP_DIR" -maxdepth 1 -name 'aiven_push_*.sql' -mtime +1 -delete 2>/dev/null || true
    # Snapshot arsip lokal TEPAT sebelum dump dibuat (basis verifikasi pasca-impor;
    # bukan count live yg bergerak selama impor berlangsung).
    SNAP_ARSIP="$(MYSQL_PWD="$DB_PASSWORD" mysql --host="$DB_HOST" --port="$DB_PORT" --user="$DB_USERNAME" -N -e "SELECT COUNT(*) FROM \`$DB_DATABASE\`.arsip" 2>/dev/null || echo '?')"
    if ! mysqldump \
        --host="$DB_HOST" --port="$DB_PORT" --user="$DB_USERNAME" \
        --no-tablespaces --single-transaction --skip-extended-insert \
        --ignore-table="$DB_DATABASE.telescope_entries" \
        --ignore-table="$DB_DATABASE.telescope_entries_tags" \
        --ignore-table="$DB_DATABASE.telescope_monitoring" \
        "$DB_DATABASE" 2>>"$LOG_FILE" | normalize_mysql8 >"$AIVEN_TMP"; then
      rm -f "$AIVEN_TMP"
      log "Aiven GAGAL: pembuatan dump ramping Aiven"
      fail "Aiven: gagal membuat dump ramping"
    fi
    if ! command -v python3 >/dev/null 2>&1; then
      rm -f "$AIVEN_TMP"
      log "Aiven GAGAL: python3 tidak tersedia (dibutuhkan utk merge INSERT)"
      fail "Aiven: python3 tidak tersedia"
    fi
    if ! python3 "$SCRIPT_DIR/merge_mysql_inserts.py" "$AIVEN_TMP" "$AIVEN_TMP.merged"; then
      rm -f "$AIVEN_TMP" "$AIVEN_TMP.merged"
      log "Aiven GAGAL: penggabungan INSERT dump ramping"
      fail "Aiven: gagal menggabungkan dump ramping"
    fi
    PUSH_START="$(date +%s)"
    if ! MYSQL_PWD="$AIVEN_PASSWORD" mysql "${MYSQLOPTS[@]}" \
         --connect-timeout=60 \
         --init-command="SET SESSION net_read_timeout=3600; SET SESSION net_write_timeout=3600; SET SESSION max_allowed_packet=1073741824; SET SESSION lock_wait_timeout=60;" \
         "$AIVEN_DATABASE" <"$AIVEN_TMP.merged" >>"$LOG_FILE" 2>&1; then
      rm -f "$AIVEN_TMP" "$AIVEN_TMP.merged"
      log "Aiven GAGAL: impor dump ($FNAME)"
      fail "Aiven: impor dump gagal"
    fi
    PUSH_SECS="$(( $(date +%s) - PUSH_START ))"
    rm -f "$AIVEN_TMP" "$AIVEN_TMP.merged"
    log "Aiven push selesai dalam ${PUSH_SECS} detik"
    # verifikasi: jumlah arsip di salinan Aiven vs SNAPSHOT lokal saat dump dibuat.
    # (TIDAK membandingkan dgn count live — app bisa menambah arsip selama impor,
    #  sehingga "selisih" lama = data baru yg sah, bukan kegagalan impor.)
    AIVEN_CNT="$(MYSQL_PWD="$AIVEN_PASSWORD" mysql "${MYSQLOPTS[@]}" -N -e "SELECT COUNT(*) FROM \`$AIVEN_DATABASE\`.arsip" 2>/dev/null || echo '?')"
    if [[ "$SNAP_ARSIP" == "$AIVEN_CNT" && "$AIVEN_CNT" != "?" ]]; then
      log "Aiven OK: impor berhasil (arsip=$AIVEN_CNT, cocok snapshot dump local)"
      echo "Aiven OK: arsip=$AIVEN_CNT baris (cocok snapshot dump local)"
    else
      SELISIH="$([[ "$SNAP_ARSIP" != "?" && "$AIVEN_CNT" != "?" ]] && echo "$((SNAP_ARSIP - AIVEN_CNT))" || echo '?')"
      log "Aiven PERINGATAN: arsip snapshot=$SNAP_ARSIP vs aiven=$AIVEN_CNT (selisih=$SELISIH; bila positif = arsip baru setelah dump, wajar)"
      echo "Aiven PERINGATAN: arsip snapshot=$SNAP_ARSIP vs aiven=$AIVEN_CNT"
    fi
  fi
else
  log "Aiven dilewati (AIVEN_HOST / AIVEN_PASSWORD belum diisi)"
fi

echo "OK: $FNAME (${SIZE} byte)"
exit 0