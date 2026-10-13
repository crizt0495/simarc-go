#!/usr/bin/env bash
# ===========================================================================
#  SIMARC — Build Standalone Windows Installer Package (ZIP)
#
#  Menghasilkan  dist/SIMARC-Windows-<versi>.zip  yang berisi:
#    - Install-SIMARC.bat  (klik dua kali untuk install/update)
#    - SIMARC-Setup.ps1    (installer/updater idempoten, tanpa admin)
#    - run.bat             (peluncur aplikasi desktop)
#    - web/  docs/  dist/  (biner server amd64 + arm64)  .env.example  VERSION
#
#  UPDATE berarti: ekstrak zip versi baru, jalankan Install-SIMARC.bat lagi.
#  .env dan storage\ milik komputer TIDAK pernah disentuh oleh updater.
#
#  Pemakaian:  bash packaging/make-windows-package.sh
# ===========================================================================
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

VERSION="$(git describe --tags --always 2>/dev/null | sed 's/^v//' || echo 1.0.0)"
VERSION_CLEAN="$(printf '%s' "$VERSION" | tr -c 'A-Za-z0-9._-' '_')"
PKG="SIMARC-Windows-${VERSION_CLEAN}"
STAGE="$(mktemp -d /tmp/opencode/simarc-win-XXXXXX)"
trap 'rm -rf "$STAGE"' EXIT

echo "==> Versi paket: $VERSION_CLEAN"
mkdir -p "$STAGE/web" "$STAGE/docs" "$STAGE/dist" "$STAGE/packaging"

# ── Runtime & asset ─────────────────────────────────────────────────────────
rsync -a --exclude '.min.map' web/ "$STAGE/web/"
rsync -a docs/ "$STAGE/docs/"
cp -f run.bat "$STAGE/run.bat"
cp -f .env.example "$STAGE/.env.example"

# ── Biner server Windows (harus ada hasil build-all.sh) ─────────────────────
[ -f dist/simarc-server-windows-amd64.exe ] || { echo "[ERR] dist/simarc-server-windows-amd64.exe tidak ada — jalankan packaging/build-all.sh"; exit 1; }
cp -f dist/simarc-server-windows-amd64.exe "$STAGE/dist/"
if [ -f dist/simarc-server-windows-arm64.exe ]; then
  cp -f dist/simarc-server-windows-arm64.exe "$STAGE/dist/"
fi

# ── Installer / updater ─────────────────────────────────────────────────────
cp -f packaging/windows/SIMARC-Setup.ps1          "$STAGE/"
cp -f packaging/windows/Install-SIMARC.bat        "$STAGE/"
cp -f packaging/windows/Uninstall-SIMARC.bat      "$STAGE/"
cp -f packaging/windows/Uninstall-SIMARC.ps1      "$STAGE/"
cp -f packaging/windows/README-WINDOWS.md         "$STAGE/README-WINDOWS.md"
cp -f packaging/install-desktop.ps1               "$STAGE/packaging/"
cp -f packaging/uninstall-desktop.ps1             "$STAGE/packaging/"
printf '%s' "$VERSION_CLEAN" > "$STAGE/VERSION"

# Normalisasi line-ending CRLF untuk skrip Windows (cmd.exe & PowerShell).
while IFS= read -r -d '' f; do
  sed -i 's/\r$//' "$f"
  sed -i 's/$/\r/' "$f"
done < <(find "$STAGE" -type f \( -name '*.bat' -o -name '*.cmd' -o -name '*.ps1' \) -print0)

# ── Zip ─────────────────────────────────────────────────────────────────────
mkdir -p dist
cd "$STAGE"
OUT="$ROOT/dist/$PKG.zip"
rm -f "$OUT"
# Catatan: pakai "." (bukan "./*") supaya berkas titik seperti .env.example ikut.
zip -qr "$OUT" .
cd "$ROOT"

echo "==> OK: $OUT ($(du -h "$OUT" | cut -f1))"
echo "    Isi:"; unzip -l "$OUT" | grep -E "run.bat|simarc-server-windows|SIMARC-Setup|Install-SIMARC|\.env\.example|VERSION" | head -12
echo "    Pastikan TIDAK ada .env maupun storage/ di dalam paket:"; unzip -l "$OUT" | grep -E "(^|/)(\.env|storage)/" | grep -v "\.env\.example" || echo "    aman — tidak ada (.env maupun storage)."

# ── Installer SATU FILE (.exe self-extracting) ──────────────────────────────
#  Satu .exe memuat seluruh payload (biner server + web/ + skrip) lalu
#  menjalankan SIMARC-Setup.ps1. Cukup SALIN 1 file ke komputer lain.
build_installer() {
  local arch="$1" out="$2"
  local bdir; bdir="$(mktemp -d /tmp/opencode/simarc-inst-XXXXXX)"
  cp -f packaging/windows/installer/go.mod  "$bdir/"
  cp -f packaging/windows/installer/main.go "$bdir/"
  mkdir -p "$bdir/payload"
  cp -a "$STAGE/." "$bdir/payload/"
  # Tiap installer hanya memuat biner yang sesuai arsitekturnya (hemat ukuran).
  if [ "$arch" = "amd64" ]; then
    rm -f "$bdir/payload/dist/simarc-server-windows-arm64.exe"
  else
    rm -f "$bdir/payload/dist/simarc-server-windows-amd64.exe"
  fi
  ( cd "$bdir" && GOFLAGS= GOWORK=off GO111MODULE=on GOOS=windows GOARCH="$arch" CGO_ENABLED=0 \
      go build -trimpath -ldflags="-s -w" -o "$out" . )
  rm -rf "$bdir"
}

echo "==> Membangun installer SATU FILE (Windows .exe)..."
EXE_AMD="$ROOT/dist/SIMARC-Setup-${VERSION_CLEAN}.exe"
build_installer amd64 "$EXE_AMD"
echo "==> OK: $EXE_AMD ($(du -h "$EXE_AMD" | cut -f1))"
if [ -f dist/simarc-server-windows-arm64.exe ]; then
  EXE_ARM="$ROOT/dist/SIMARC-Setup-${VERSION_CLEAN}-arm64.exe"
  build_installer arm64 "$EXE_ARM"
  echo "==> OK: $EXE_ARM ($(du -h "$EXE_ARM" | cut -f1))"
fi
echo "    Cukup SALIN: dist/SIMARC-Setup-${VERSION_CLEAN}.exe  (satu file) ke komputer lain, lalu klik dua kali."