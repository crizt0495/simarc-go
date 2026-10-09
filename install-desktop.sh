#!/usr/bin/env bash
# ===========================================================================
#  SIMARC — Desktop Installer (Linux)
#  Memasang SIMARC sebagai aplikasi desktop:
#    - Entri menu aplikasi (launcher aplikasi)
#   - Shortcut Desktop (opsional)
#    - Perintah terminal "simarc" (bila ~/.local/bin ada di PATH)
#
#  Pemakaian: ./install-desktop.sh
# ===========================================================================
set -euo pipefail

APP_DIR="$(cd "$(dirname "$0")" && pwd)"
LAUNCHER="$APP_DIR/run-desktop.sh"
ICON="$APP_DIR/web/static/images/logo-icon.svg"

if [[ ! -x "$APP_DIR/run.sh" ]]; then
    chmod +x "$APP_DIR/run.sh" 2>/dev/null || true
fi
chmod +x "$APP_DIR/run-desktop.sh" 2>/dev/null || true

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
Icon=${APP_DIR}/web/static/images/logo-icon.svg
Terminal=false
Categories=Office;Database;
Keywords=arsip;archive;simarc;record;center;
StartupNotify=true
StartupWMClass=SIMARC
EOF

if command -v update-desktop-database >/dev/null 2>&1; then
    update-desktop-database "${XDG_DATA_HOME:-$HOME/.local/share}/applications" >/dev/null 2>&1 || true
fi

# Perintah terminal "simarc"
if [[ ":$PATH:" == *":$HOME/.local/bin:"* ]]; then
    mkdir -p "$HOME/.local/bin"
    printf '#!/usr/bin/env bash\nexec "%s/run-desktop.sh" "$@"\n' "$APP_DIR" > "$HOME/.local/bin/simarc"
    chmod +x "$HOME/.local/bin/simarc"
fi

# Shortcut Desktop (bila ada folder Desktop)
DESKTOP_DIR="$(command -v xdg-user-dir >/dev/null 2>&1 && xdg-user-dir DESKTOP || true)"
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
echo "  Menu     : $APPS_DIR/simarc.desktop"
if [[ -n "${DESKTOP_DIR:-}" && -d "$DESKTOP_DIR" ]]; then
    echo "  Desktop  : $DESKTOP_DIR/simarc.desktop"
fi
if [[ -x "$HOME/.local/bin/simarc" ]]; then
    echo "  Terminal : $HOME/.local/bin/simarc"
fi
