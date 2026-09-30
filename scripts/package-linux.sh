#!/usr/bin/env bash
#
# Package the SIMARC desktop app for Linux.
#
# Produces, in desktop/build/dist/:
#   simarc_<version>_amd64.deb   Debian/Ubuntu/Mint — install with
#                                 `sudo apt install ./simarc_….deb`
#   SIMARC-<version>-linux-amd64.tar.gz
#                                 Everywhere else — untar and run ./install.sh
#   SIMARC-<version>-x86_64.AppImage
#                                 Only when appimagetool is installed.
#
# The .deb is assembled by hand with dpkg-deb so the script needs no extra
# tooling beyond a Go toolchain and the webkit2gtk-4.1 development headers.
# Run it from the repository root:  ./scripts/package-linux.sh
set -euo pipefail

VERSION="${SIMARC_VERSION:-1.0.0}"
ARCH="$(dpkg --print-architecture 2>/dev/null || uname -m)"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DESKTOP="$ROOT/desktop"
DIST="$DESKTOP/build/dist"
STAGE="$DESKTOP/build/.stage"
BIN="$DESKTOP/build/bin/SIMARC"

info() { printf '\033[36m==>\033[0m %s\n' "$*"; }
die()  { printf '\033[31mERROR:\033[0m %s\n' "$*" >&2; exit 1; }

# ── 1. Build the binary ──────────────────────────────────────────────────────
info "Building the desktop binary (linux/amd64)"
cd "$DESKTOP"
wails build -tags "desktop,webkit2_41" -skipbindings -platform linux/amd64

[ -x "$BIN" ] || die "binary not produced at $BIN"
info "Binary: $(du -h "$BIN" | cut -f1)"

mkdir -p "$DIST"
rm -rf "$STAGE"

# ── 2. Assemble the .deb tree ────────────────────────────────────────────────
info "Assembling the .deb"
install -d "$STAGE/DEBIAN"
install -d "$STAGE/usr/bin"
install -d "$STAGE/usr/share/applications"
install -d "$STAGE/usr/share/icons/hicolor/512x512/apps"
install -d "$STAGE/usr/share/doc/simarc"

install -m 0755 "$BIN"                 "$STAGE/usr/bin/SIMARC"
install -m 0644 "$ROOT/.desktop/simarc.desktop" \
                                      "$STAGE/usr/share/applications/simarc.desktop"
install -m 0644 "$DESKTOP/build/appicon.png" \
                                      "$STAGE/usr/share/icons/hicolor/512x512/apps/simarc.png"

# Exec=SIMARC %u resolves on PATH to /usr/bin/SIMARC, so the .desktop file
# works unchanged from a system-wide install.

cat > "$STAGE/DEBIAN/control" <<EOF
Package: simarc
Version: $VERSION
Section: utils
Priority: optional
Architecture: $ARCH
Maintainer: SIMARC <simarc@example.com>
Depends: libwebkit2gtk-4.1-0, libgtk-3-0
Installed-Size: $(du -ks "$STAGE/usr" | cut -f1)
Description: Sistem Informasi Manajemen Arsip Record Center
 SIMARC adalah aplikasi manajemen arsip dengan navigasi top bar.
 Ini adalah versi desktop: jendela native (Wails) di atas webview sistem,
 dengan database MySQL lokal.
EOF

cat > "$STAGE/usr/share/doc/simarc/README" <<'EOF'
SIMARC — Sistem Informasi Manajemen Arsip Record Center
======================================================

Mulai dari menu aplikasi, atau jalankan dari terminal:

    simarc

Konfigurasi database dibaca dari variabel lingkungan / berkas .env di
direktori kerja. Lihat README.md repositori untuk rincian.
EOF

DEB="$DIST/simarc_${VERSION}_${ARCH}.deb"
dpkg-deb --root-owner-group --build "$STAGE" "$DEB" >/dev/null
info "Deb: $(basename "$DEB") ($(du -h "$DEB" | cut -f1))"

# ── 3. Sanity-check the .deb ─────────────────────────────────────────────────
info "Verifying the package"
if ! dpkg-deb --info "$DEB" >/dev/null; then
	die "the generated .deb is unreadable"
fi
# Write the listing to a file first: piping into `grep -q` closes the pipe
# early, which kills dpkg's tar subprocess with SIGPIPE.
LISTING="$(mktemp)"
trap 'rm -f "$LISTING"' EXIT
dpkg-deb --contents "$DEB" > "$LISTING"
for required in 'usr/bin/SIMARC' 'usr/share/applications/simarc.desktop' \
                'usr/share/icons/hicolor/512x512/apps/simarc.png'; do
	if ! grep -qF "$required" "$LISTING"; then
		die "the .deb is missing $required"
	fi
done
info "Package contains $(wc -l < "$LISTING") paths"

# ── 4. Tarball with a self-contained installer ───────────────────────────────
info "Building the tarball"
TARBALL_DIR="$DIST/SIMARC-$VERSION-linux-$ARCH"
rm -rf "$TARBALL_DIR"
mkdir -p "$TARBALL_DIR"
cp "$BIN" "$TARBALL_DIR/SIMARC"
cp "$DESKTOP/build/appicon.png" "$TARBALL_DIR/simarc.png"
cp "$ROOT/.desktop/simarc.desktop" "$TARBALL_DIR/simarc.desktop"

cat > "$TARBALL_DIR/install.sh" <<'EOF'
#!/usr/bin/env bash
# Install SIMARC for the current user (no root required).
set -euo pipefail
here="$(cd "$(dirname "$0")" && pwd)"
bin="${HOME}/.local/bin"
share="${HOME}/.local/share"

install -d "$bin" "$share/applications" "$share/icons/hicolor/512x512/apps"
install -m 0755 "$here/SIMARC" "$bin/SIMARC"
install -m 0644 "$here/simarc.desktop" "$share/applications/simarc.desktop"
install -m 0644 "$here/simarc.png" \
    "$share/icons/hicolor/512x512/apps/simarc.png"
command -v update-desktop-database >/dev/null 2>&1 && \
    update-desktop-database "$share/applications" || true

echo "Installed. Start SIMARC from your application menu, or run: simarc"
EOF
chmod +x "$TARBALL_DIR/install.sh"

tar -czf "$DIST/SIMARC-$VERSION-linux-$ARCH.tar.gz" -C "$DIST" "$(basename "$TARBALL_DIR")"
info "Tarball: SIMARC-$VERSION-linux-$ARCH.tar.gz"

# ── 5. AppImage, only if the tool is present ─────────────────────────────────
if command -v appimagetool >/dev/null 2>&1; then
	info "Building the AppImage"
	APPDIR="$DIST/AppDir"
	rm -rf "$APPDIR"
	mkdir -p "$APPDIR/usr/bin" "$APPDIR/usr/share/applications" \
	         "$APPDIR/usr/share/icons/hicolor/512x512/apps"
	cp "$BIN" "$APPDIR/usr/bin/SIMARC"
	cp "$ROOT/.desktop/simarc.desktop" "$APPDIR/simarc.desktop"
	cp "$DESKTOP/build/appicon.png" "$APPDIR/simarc.png"
	cp "$DESKTOP/build/appicon.png" \
	    "$APPDIR/usr/share/icons/hicolor/512x512/apps/simarc.png"

	cat > "$APPDIR/AppRun" <<'EOF'
#!/usr/bin/env bash
HERE="$(dirname "$(readlink -f "${0}")")"
exec "$HERE/usr/bin/SIMARC" "$@"
EOF
	chmod +x "$APPDIR/AppRun"
	# appimagetool wants the desktop file at the AppDir root, named after the app.
	appimagetool --no-appstream "$APPDIR" \
	    "$DIST/SIMARC-$VERSION-x86_64.AppImage"
	info "AppImage: SIMARC-$VERSION-x86_64.AppImage"
else
	info "appimagetool not found — skipping the AppImage (the tarball covers it)"
fi

rm -rf "$STAGE" "$TARBALL_DIR"

info "Done. Artifacts in desktop/build/dist/:"
ls -1 "$DIST"
