#!/usr/bin/env bash
# ─────────────────────────────────────────────────────────────────
# Build paket .deb produksi untuk SIMARC (Wails desktop).
# Menjalankan `wails build` (PRODUCTION, bukan wails dev) lalu
# membungkus hasilnya jadi .deb: executable + shortcut menu + ikon.
# Hasil: dist/simarc_1.0.0-1_amd64.deb
# ─────────────────────────────────────────────────────────────────
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"
export PATH="$PATH:$HOME/go/bin"

VERSION=1.0.0-1
DEB=dist/simarc_${VERSION}_amd64
STAGE="${DEB}"

echo "▸ Wails build (production)…"
wails build -tags webkit2_41 -clean
BINARY=build/bin/SIMARC
[ -x "$BINARY" ] || { echo "✗ binary tidak ditemukan"; exit 1; }

echo "▸ Menyiapkan pohon paket…"
rm -rf "$STAGE"
mkdir -p "$STAGE"/DEBIAN \
         "$STAGE"/usr/bin \
         "$STAGE"/usr/share/applications \
         "$STAGE"/usr/share/icons/hicolor/512x512/apps \
         "$STAGE"/usr/share/doc/simarc

install -m 0755 "$BINARY" "$STAGE/usr/bin/SIMARC"
install -m 0644 packaging/linux/simarc.desktop "$STAGE/usr/share/applications/simarc.desktop"
install -m 0644 build/appicon.png "$STAGE/usr/share/icons/hicolor/512x512/apps/simarc.png"
{
  echo "SIMARC — Arsip Record Center (desktop, offline-first)"
  echo
  echo "Dibangun dengan Wails; frontend React di-embed."
  echo "Konfigurasi: siapkan file ~/.env dari contoh yang disertakan"
  echo "  cp /usr/share/doc/simarc/.env.example ~/.env"
  echo
  echo "Aplikasi membaca .env berurutan dari:"
  echo "  1. env runtime proses (mis. dari launcher)"
  echo "  2. HOME pengguna: ~/.env"
  echo "Database MySQL/MariaDB lokal pada DB_* di atas."
} > "$STAGE/usr/share/doc/simarc/README"
install -m 0644 .env.example "$STAGE/usr/share/doc/simarc/.env.example"

N=$(du -sk "$STAGE" | cut -f1)
cat > "$STAGE/DEBIAN/control" <<EOF
Package: simarc
Source: simarc
Version: ${VERSION}
Section: Office
Priority: optional
Architecture: amd64
Installed-Size: ${N}
Maintainer: SIMARC <dev@simarc.local>
Depends: libgtk-3-0, libwebkit2gtk-4.1-0
Description: SIMARC — Arsip Record Center (desktop)
 Aplikasi desktop mandiri untuk manajemen arsip offline-first.
 Backend Golang + database MySQL/MariaDB lokal.
EOF

echo "▸ Menulis .deb…"
dpkg-deb --build "$STAGE" dist/simarc_${VERSION}_amd64.deb
rm -rf "$STAGE"
echo "✓ dist/simarc_${VERSION}_amd64.deb siap."
