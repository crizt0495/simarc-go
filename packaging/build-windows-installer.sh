#!/usr/bin/env bash
# ===========================================================================
#  SIMARC — Build ONE-FILE Windows installer & keep dist/ clean.
#
#  Hasil:  dist/SIMARC-Setup.exe            (satu file, amd64)
#  Opsional ARM64:
#          SIMARC_BUILD_ARM64=1 bash packaging/build-windows-installer.sh
#          -> dist/SIMARC-Setup-arm64.exe
#
#  Semua biner & perantara dibangun di folder sementara lalu dibersihkan,
#  dan dist/ dirapikan sehingga HANYA berisi installer .exe. Jadi cukup
#  1 file untuk disalin ke komputer lain.
#
#  Pemakaian:  bash packaging/build-windows-installer.sh
#  (butuh Go; TIDAK butuh Inno Setup; bisa dijalankan dari Linux.)
# ===========================================================================
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

VERSION="$(git describe --tags --always 2>/dev/null | sed 's/^v//' || echo 1.0.0)"
VERSION_CLEAN="$(printf '%s' "$VERSION" | tr -c 'A-Za-z0-9._-' '_')"
echo "==> Versi: $VERSION_CLEAN"

WORK="$(mktemp -d /tmp/opencode/simarc-winbuild-XXXXXX)"
trap 'rm -rf "$WORK"' EXIT
PAY="$WORK/payload"
mkdir -p "$PAY/web" "$PAY/docs" "$PAY/dist" "$PAY/packaging"

# ── 1) Biner server Windows (ke folder kerja — TIDAK mengotori dist/) ───────
echo "==> Build biner server Windows amd64..."
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -buildvcs=false -ldflags="-s -w" \
  -o "$PAY/dist/simarc-server-windows-amd64.exe" ./cmd/server
if [ "${SIMARC_BUILD_ARM64:-0}" = "1" ]; then
  echo "==> Build biner server Windows arm64..."
  GOOS=windows GOARCH=arm64 CGO_ENABLED=0 go build -buildvcs=false -ldflags="-s -w" \
    -o "$PAY/dist/simarc-server-windows-arm64.exe" ./cmd/server
fi

# ── 2) Susun payload aplikasi ───────────────────────────────────────────────
rsync -a --exclude '.min.map' web/ "$PAY/web/"
rsync -a docs/ "$PAY/docs/"
cp -f run.bat "$PAY/run.bat"
cp -f .env.example "$PAY/.env.example"
cp -f packaging/windows/SIMARC-Setup.ps1     "$PAY/"
cp -f packaging/windows/SIMARC-Installer-GUI.ps1 "$PAY/"
cp -f packaging/windows/Install-SIMARC.bat   "$PAY/"
cp -f packaging/windows/Uninstall-SIMARC.bat "$PAY/"
cp -f packaging/windows/Uninstall-SIMARC.ps1 "$PAY/"
cp -f packaging/windows/README-WINDOWS.md    "$PAY/README-WINDOWS.md"
cp -f packaging/windows/simarc-install.txt.example "$PAY/simarc-install.txt.example"
cp -f packaging/windows/simarc.ico           "$PAY/simarc.ico"
cp -f packaging/install-desktop.ps1          "$PAY/packaging/"
cp -f packaging/uninstall-desktop.ps1        "$PAY/packaging/"
printf '%s' "$VERSION_CLEAN" > "$PAY/VERSION"

# ── 2b) MariaDB portabel (mode "offline mandiri") ───────────────────────────
#  Default: disertakan, sehingga komputer klien cukup memasang satu file dan
#  langsung punya database sendiri (tanpa server pusat, tanpa install MySQL).
#  Matikan dengan SIMARC_BUNDLE_DB=0.
if [ "${SIMARC_BUNDLE_DB:-1}" = "1" ]; then
  echo "==> Menyertakan MariaDB portabel (offline mandiri)..."
  bash packaging/windows/build-mariadb-bundle.sh
  cp -f packaging/cache/mariadb-winx64.zip "$PAY/mariadb-winx64.zip"
else
  echo "==> MariaDB portabel TIDAK disertakan (SIMARC_BUNDLE_DB=0)."
fi

# ── 3) Normalisasi CRLF untuk skrip Windows ─────────────────────────────────
while IFS= read -r -d '' f; do
  sed -i 's/\r$//' "$f"
  sed -i 's/$/\r/' "$f"
done < <(find "$PAY" -type f \( -name '*.bat' -o -name '*.cmd' -o -name '*.ps1' \) -print0)

# ── 4) Build installer SATU FILE (self-extracting) ──────────────────────────
build_installer() {
  local arch="$1" out="$2"
  local bdir; bdir="$(mktemp -d /tmp/opencode/simarc-inst-XXXXXX)"
  cp -f packaging/windows/installer/go.mod  "$bdir/"
  cp -f packaging/windows/installer/main.go "$bdir/"
  # Windows icon resource (.syso) so the installer .exe carries the SIMARC icon.
  if [ -f "packaging/windows/installer/rsrc_windows_${arch}.syso" ]; then
    cp -f "packaging/windows/installer/rsrc_windows_${arch}.syso" "$bdir/"
  fi
  mkdir -p "$bdir/payload"
  cp -a "$PAY/." "$bdir/payload/"
  # Tiap installer hanya memuat biner yang sesuai arsitekturnya.
  if [ "$arch" = "amd64" ]; then
    rm -f "$bdir/payload/dist/simarc-server-windows-arm64.exe"
  else
    rm -f "$bdir/payload/dist/simarc-server-windows-amd64.exe"
  fi
  ( cd "$bdir" && GOFLAGS= GOWORK=off GO111MODULE=on GOOS=windows GOARCH="$arch" CGO_ENABLED=0 \
      go build -trimpath -ldflags="-s -w -H=windowsgui" -o "$out" . )
  rm -rf "$bdir"
}

mkdir -p dist
echo "==> Build installer satu-file (amd64)..."
build_installer amd64 "$WORK/SIMARC-Setup.exe"
mv -f "$WORK/SIMARC-Setup.exe" "$ROOT/dist/SIMARC-Setup.exe"

if [ "${SIMARC_BUILD_ARM64:-0}" = "1" ]; then
  echo "==> Build installer satu-file (arm64)..."
  build_installer arm64 "$WORK/SIMARC-Setup-arm64.exe"
  mv -f "$WORK/SIMARC-Setup-arm64.exe" "$ROOT/dist/SIMARC-Setup-arm64.exe"
fi

# ── 5) Rapikan dist/ → hanya installer .exe ─────────────────────────────────
#  Lewati dengan: SIMARC_KEEP_DIST=1
if [ "${SIMARC_KEEP_DIST:-0}" != "1" ]; then
  echo "==> Membersihkan dist/ (sisakan installer .exe saja)..."
  find dist -maxdepth 1 -type f \
    ! -name 'SIMARC-Setup.exe' ! -name 'SIMARC-Setup-arm64.exe' \
    -print -delete
fi

echo "==> SELESAI. dist/ sekarang:"
ls -lh dist/
echo "==> Satu file untuk disalin:"
sha256sum "$ROOT/dist/SIMARC-Setup.exe"
