#!/usr/bin/env bash
# ===========================================================================
#  SIMARC — Desktop Uninstaller (Linux)
#  Menghapus instalasi desktop (menu aplikasi, ikon, shortcut, perintah simarc)
# ===========================================================================
set -euo pipefail

APP_NAME="simarc"
DESKTOP_ENTRY="simarc.desktop"

remove_if_exists() {
    [[ -e "$1" ]] || [[ -L "$1" ]] && rm -f "$1"
}

# ── Entri menu aplikasi ─────────────────────────────────────────────────────
APPS_DIR="${XDG_DATA_HOME:-$HOME/.local/share}/applications"
remove_if_exists "$APPS_DIR/$DESKTOP_ENTRY"

# ── Ikon aplikasi (hicolor) ─────────────────────────────────────────────────
ICONS_ROOT="${XDG_DATA_HOME:-$HOME/.local/share}/icons/hicolor"
for s in 16 24 32 48 64 128 192 256 512; do
    remove_if_exists "$ICONS_ROOT/${s}x${s}/apps/${APP_NAME}.png"
done
remove_if_exists "$ICONS_ROOT/scalable/apps/${APP_NAME}.svg"

# ── Pixmaps ─────────────────────────────────────────────────────────────────
PIXMAPS="${XDG_DATA_HOME:-$HOME/.local/share}/pixmaps"
mkdir -p "$PIXMAPS" 2>/dev/null || true
remove_if_exists "$PIXMAPS/${APP_NAME}.ico"

# ── Shortcut Desktop ───────────────────────────────────────────────────────
if command -v xdg-user-dir >/dev/null 2>&1; then
    DESKTOP_DIR="$(xdg-user-dir DESKTOP || true)"
    if [[ -n "$DESKTOP_DIR" && -d "$DESKTOP_DIR" ]]; then
        remove_if_exists "$DESKTOP_DIR/$DESKTOP_ENTRY"
    fi
fi

# ── Perintah terminal "simarc" ──────────────────────────────────────────────
remove_if_exists "$HOME/.local/bin/simarc"

# ── Cache ikon ──────────────────────────────────────────────────────────────
if command -v gtk-update-icon-cache >/dev/null 2>&1; then
    gtk-update-icon-cache -f -t "$ICONS_ROOT" >/dev/null 2>&1 || true
fi
if command -v update-desktop-database >/dev/null 2>&1; then
    update-desktop-database "$APPS_DIR" >/dev/null 2>&1 || true
fi

echo "SIMARC berhasil di-uninstall (desktop integration dihapus)."
echo "Catatan: File aplikasi di direktori ini tidak dihapus. Jalankan 'rm -rf $(pwd)' jika ingin menghapus seluruh folder aplikasi."
