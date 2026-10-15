#!/usr/bin/env bash
# ===========================================================================
#  SIMARC — Bundel MariaDB portabel untuk Windows (mode "offline mandiri").
#
#  Mengunduh paket resmi MariaDB winx64, memangkasnya ke berkas yang benar-benar
#  dipakai server lokal, lalu membungkusnya menjadi satu zip kecil:
#
#      packaging/cache/mariadb-winx64.zip      (~14 MB)
#
#  Zip inilah yang ikut disertakan ke installer (lihat build-windows-installer.sh
#  dengan SIMARC_BUNDLE_DB=1). Hasil ekstraksi di Windows: bin/, lib/, share/.
#
#  Versi 10.11 (LTS) dipilih karena biner Windows-nya MANDIRI: server.dll tidak
#  membutuhkan DLL eksternal (ICU, OpenSSL, dsb.), berbeda dengan 11.x yang
#  server.dll-nya mengimpor icuuc.dll/icuin.dll yang tidak ikut dalam paket.
#
#  Pemakaian:  bash packaging/windows/build-mariadb-bundle.sh
#  Ulang-undang tidak mengunduh bila zip sudah ada (hapus untuk memaksa).
# ===========================================================================
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
CACHE="$ROOT/packaging/cache"
OUT="$CACHE/mariadb-winx64.zip"

# Versi & sumber resmi (archive.mariadb.org) beserta checksum SHA-256.
VER="10.11.9"
URL="https://archive.mariadb.org/mariadb-${VER}/winx64-packages/mariadb-${VER}-winx64.zip"
SHA="616f22fc1c10a27e8f38d562700d345de96d95c2a30503e2a62edaf30d40682f"

if [ -f "$OUT" ]; then
  echo "==> Bundel sudah ada: $OUT ($(du -h "$OUT" | cut -f1)) — dilewati."
  exit 0
fi

mkdir -p "$CACHE"
FULL="$CACHE/mariadb-${VER}-winx64-full.zip"

if [ ! -f "$FULL" ]; then
  echo "==> Mengunduh MariaDB ${VER} (winx64)..."
  curl -fSL --retry 3 -o "$FULL.part" "$URL"
  mv -f "$FULL.part" "$FULL"
fi

echo "==> Memeriksa checksum..."
echo "${SHA}  ${FULL}" | sha256sum -c -

TMP="$(mktemp -d /tmp/opencode/simarc-mdb-XXXXXX)"
trap 'rm -rf "$TMP"' EXIT

echo "==> Mengekstrak..."
( cd "$TMP" && unzip -q "$FULL" )
SRC="$TMP/mariadb-${VER}-winx64"
[ -d "$SRC" ] || { echo "[ERR] Struktur paket tidak dikenal di $SRC"; exit 1; }

echo "==> Memangkas ke berkas runtime minimal..."
MIN="$TMP/min"
mkdir -p "$MIN/bin" "$MIN/lib" "$MIN/share"
# Biner inti: server (mariadbd + mysqld shim + server.dll), pembuat datadir,
# klien, admin, dump. mariadb-install-db.exe memanggil "mysqld.exe" saat
# bootstrap, jadi KEDUA shim harus ada.
cp "$SRC"/bin/mariadbd.exe \
   "$SRC"/bin/mysqld.exe \
   "$SRC"/bin/server.dll \
   "$SRC"/bin/mariadb-install-db.exe \
   "$SRC"/bin/mariadb.exe \
   "$SRC"/bin/mariadb-admin.exe \
   "$SRC"/bin/mariadb-dump.exe \
   "$SRC"/bin/my_print_defaults.exe \
   "$MIN/bin/"
# Pustaka klien + plugin (auth caching_sha2 dsb).
cp "$SRC"/lib/libmariadb.dll "$MIN/lib/"
cp -a "$SRC"/lib/plugin "$MIN/lib/"
# Data pendukung server: charset, pesan, tabel sistem.
cp -a "$SRC"/share/charsets "$SRC"/share/english "$MIN/share/"
cp "$SRC"/share/*.sql "$MIN/share/"
cp "$SRC"/COPYING "$MIN/" 2>/dev/null || true

echo "==> Membungkus (zip)..."
rm -f "$OUT"
( cd "$MIN" && zip -qr -9 "$OUT" . )

echo "==> OK: $OUT ($(du -h "$OUT" | cut -f1))"
echo "    Isi:"; unzip -l "$OUT" | awk 'NR<=6 || /bin\/|lib\/|share\//' | head -20
