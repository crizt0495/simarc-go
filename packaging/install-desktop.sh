#!/usr/bin/env bash
# ===========================================================================
#  SIMARC — Desktop Installer (Linux)
#  Memasang SIMARC sebagai aplikasi desktop:
#    - Ikon aplikasi (tema hicolor, berbagai ukuran)
#    - Entri menu aplikasi (launcher aplikasi)
#    - Shortcut Desktop (opsional)
#    - Perintah terminal "simarc" (bila ~/.local/bin ada di PATH)
#
#  Pemakaian: ./install-desktop.sh
# ===========================================================================
set -euo pipefail

APP_DIR="$(cd "$(dirname "$0")" && pwd)"
IMAGES="$APP_DIR/web/static/images"

if [[ ! -x "$APP_DIR/run.sh" ]]; then
    chmod +x "$APP_DIR/run.sh" 2>/dev/null || true
fi
chmod +x "$APP_DIR/run-desktop.sh" 2>/dev/null || true

# ── Ikon aplikasi (tema hicolor, agar tampil di menu & taskbar) ────────────
ICONS_ROOT="${XDG_DATA_HOME:-$HOME/.local/share}/icons/hicolor"
for s in 16 24 32 48 64 128 192 256 512; do
    src="$IMAGES/icon-${s}.png"
    [[ -f "$src" ]] || continue
    dest="$ICONS_ROOT/${s}x${s}/apps"
    mkdir -p "$dest"
    cp -f "$src" "$dest/simarc.png"
done
if [[ -f "$IMAGES/logo-icon.svg" ]]; then
    mkdir -p "$ICONS_ROOT/scalable/apps"
    cp -f "$IMAGES/logo-icon.svg" "$ICONS_ROOT/scalable/apps/simarc.svg"
fi
if command -v gtk-update-icon-cache >/dev/null 2>&1; then
    gtk-update-icon-cache -f -t "$ICONS_ROOT" >/dev/null 2>&1 || true
fi

# ── Entri menu aplikasi ───────────────────────────────────────────────────
APPS_DIR="${XDG_DATA_HOME:-$HOME/.local/share}/applications"
mkdir -p "$APPS_DIR"
ENTRY="$APPS_DIR/simarc.desktop"

cat > "$ENTRY" <<EOF
[Desktop Entry]
Type=Application
Version=1.0
Name=SIMARC
GenericName=Arsip Record Center
Comment=Sistem Informasi Manajemen Arsip (web & desktop)
Exec=${APP_DIR}/run-desktop.sh
Path=${APP_DIR}
Icon=simarc
Terminal=false
Categories=Office;Database;
Keywords=arsip;archive;simarc;record;center;
StartupNotify=true
StartupWMClass=SIMARC
EOF

if command -v update-desktop-database >/dev/null 2>&1; then
    update-desktop-database "$APPS_DIR" >/dev/null 2>&1 || true
fi

# Perintah terminal "simarc"
if [[ ":$PATH:" == *":$HOME/.local/bin:"* ]]; then
    mkdir -p "$HOME/.local/bin"
    printf '#!/usr/bin/env bash\nexec "%s/run-desktop.sh" "$@"\n' "$APP_DIR" > "$HOME/.local/bin/simarc"
    chmod +x "$HOME/.local/bin/simarc"
fi

# Shortcut Desktop (bila ada folder Desktop)
DESKTOP_DIR=""
if command -v xdg-user-dir >/dev/null 2>&1; then
    DESKTOP_DIR="$(xdg-user-dir DESKTOP || true)"
fi
if [[ -n "$DESKTOP_DIR" && -d "$DESKTOP_DIR" ]]; then
    cp "$ENTRY" "$DESKTOP_DIR/simarc.desktop"
    chmod +x "$DESKTOP_DIR/simarc.desktop"
    if command -v gio >/dev/null 2>&1; then
        gio set "$DESKTOP_DIR/simarc.desktop" metadata::trusted true 2>/dev/null || true
        CK=$(md5sum "$DESKTOP_DIR/simarc.desktop" | awk '{print $1}')
        gio set "$DESKTOP_DIR/simarc.desktop" metadata::xfce-exe-checksum "$CK" 2>/dev/null || true
    fi
fi

echo "SIMARC terpasang sebagai aplikasi desktop:"
echo "  Ikon     : $ICONS_ROOT (simarc.png / simarc.svg)"
echo "  Menu     : $ENTRY"
if [[ -n "${DESKTOP_DIR:-}" && -d "$DESKTOP_DIR" ]]; then
    echo "  Desktop  : $DESKTOP_DIR/simarc.desktop"
fi
if [[ -x "$HOME/.local/bin/simarc" ]]; then
    echo "  Terminal : $HOME/.local/bin/simarc"
fi
