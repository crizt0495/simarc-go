#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")" && pwd)"
TMP="$ROOT/build-deb-tmp"
rm -rf "$TMP"
mkdir -p "$TMP/DEBIAN"
mkdir -p "$TMP/usr/share/applications"
mkdir -p "$TMP/usr/share/icons/hicolor/16x16/apps" "$TMP/usr/share/icons/hicolor/24x24/apps" "$TMP/usr/share/icons/hicolor/32x32/apps" "$TMP/usr/share/icons/hicolor/48x48/apps" "$TMP/usr/share/icons/hicolor/64x64/apps" "$TMP/usr/share/icons/hicolor/128x128/apps" "$TMP/usr/share/icons/hicolor/192x192/apps" "$TMP/usr/share/icons/hicolor/256x256/apps" "$TMP/usr/share/icons/hicolor/512x512/apps" "$TMP/usr/share/icons/hicolor/scalable/apps"
mkdir -p "$TMP/usr/share/pixmaps"
mkdir -p "$TMP/usr/bin"
mkdir -p "$TMP/opt/simarc"

cat > "$TMP/DEBIAN/control" <<'EOF'
Package: simarc
Version: 1.0.0
Section: utils
Priority: optional
Architecture: amd64
Depends: chromium-browser | google-chrome | chromium | firefox (>=78)
Maintainer: Bakesbangpol Kota Probolinggo <info@bakesbangpol.probolinggo.go.id>
Description: SIMARC - Sistem Informasi Manajemen Arsip Record Center
 Aplikasi desktop SIMARC untuk manajemen arsip record center.
EOF
cat > "$TMP/DEBIAN/postinst" <<'EOF'
#!/bin/bash
set -e
update-desktop-database /usr/share/applications >/dev/null 2>&1 || true
gtk-update-icon-cache -f -t /usr/share/icons/hicolor >/dev/null 2>&1 || true
EOF
cat > "$TMP/DEBIAN/prerm" <<'EOF'
#!/bin/bash
set -e
EOF
cat > "$TMP/DEBIAN/postrm" <<'EOF'
#!/bin/bash
set -e
update-desktop-database /usr/share/applications >/dev/null 2>&1 || true
gtk-update-icon-cache -f -t /usr/share/icons/hicolor >/dev/null 2>&1 || true
EOF
chmod 0755 "$TMP/DEBIAN/postinst" "$TMP/DEBIAN/prerm" "$TMP/DEBIAN/postrm"

cp "$ROOT/.desktop/simarc.desktop" "$TMP/usr/share/applications/simarc.desktop"
cat > "$TMP/usr/bin/simarc" <<'EOF'
#!/usr/bin/env bash
exec /opt/simarc/run-desktop.sh "$@"
EOF
chmod 0755 "$TMP/usr/bin/simarc"

rsync -a --exclude='.git/' --exclude='tmp/' --exclude='dist/' --exclude='build-deb*/' --exclude='node_modules/' --exclude='*.AppImage' "$ROOT/" "$TMP/opt/simarc/"

for s in 16 24 32 48 64 128 192 256 512; do
  [[ -f "$ROOT/web/static/images/icon-${s}.png" ]] && cp "$ROOT/web/static/images/icon-${s}.png" "$TMP/usr/share/icons/hicolor/${s}x${s}/apps/simarc.png"
done
[[ -f "$ROOT/web/static/images/logo-icon.svg" ]] && cp "$ROOT/web/static/images/logo-icon.svg" "$TMP/usr/share/icons/hicolor/scalable/apps/simarc.svg"
[[ -f "$ROOT/web/static/images/favicon.ico" ]] && cp "$ROOT/web/static/images/favicon.ico" "$TMP/usr/share/pixmaps/simarc.ico"

find "$TMP/opt/simarc" -name '*.sh' -o -name '*.command' | xargs chmod +x 2>/dev/null || true

mkdir -p "$ROOT/dist"
fakeroot dpkg-deb --build "$TMP" "$ROOT/dist/simarc_1.0.0_amd64.deb"
rm -rf "$TMP"
echo "DONE: dist/simarc_1.0.0_amd64.deb"
