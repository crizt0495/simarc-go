#!/usr/bin/env bash
# ===========================================================================
#  SIMARC — Regenerate the Windows icon resources.
#
#  Menghasilkan:
#    packaging/windows/simarc.ico            (multi-resolusi: 16..256)
#    cmd/server/rsrc_windows_amd64.syso      (ikon untuk biner aplikasi)
#    cmd/server/rsrc_windows_arm64.syso
#    packaging/windows/installer/rsrc_windows_amd64.syso   (ikon installer)
#    packaging/windows/installer/rsrc_windows_arm64.syso
#
#  File .syso sudah disertakan di repo, jadi build normal TIDAK butuh skrip ini.
#  Jalankan hanya bila ingin memperbarui ikon. Kebutuhan:
#    - python3 + Pillow  (membuat .ico dari web/static/images/icon-256.png)
#    - rsrc              (go install github.com/akavel/rsrc@latest)
#
#  Pemakaian:  bash packaging/make-windows-icon.sh
# ===========================================================================
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

export PATH="$PATH:$HOME/go/bin"
SRC_PNG="web/static/images/icon-256.png"
ICO="packaging/windows/simarc.ico"

[ -f "$SRC_PNG" ] || { echo "[ERR] $SRC_PNG tidak ada"; exit 1; }
command -v rsrc >/dev/null 2>&1 || { echo "[ERR] rsrc tidak ada — jalankan: go install github.com/akavel/rsrc@latest"; exit 1; }

echo "==> Membuat $ICO dari $SRC_PNG ..."
python3 - "$SRC_PNG" "$ICO" <<'PY'
import sys
from PIL import Image
src, out = sys.argv[1], sys.argv[2]
img = Image.open(src).convert("RGBA")
sizes = [(16,16),(24,24),(32,32),(48,48),(64,64),(128,128),(256,256)]
img.save(out, format="ICO", sizes=sizes)
print("   ukuran:", sorted(Image.open(out).ico.sizes()))
PY

echo "==> Membuat file .syso ..."
for arch in amd64 arm64; do
  rsrc -ico "$ICO" -arch "$arch" -o "cmd/server/rsrc_windows_${arch}.syso"
  rsrc -ico "$ICO" -arch "$arch" -o "packaging/windows/installer/rsrc_windows_${arch}.syso"
  echo "   cmd/server/rsrc_windows_${arch}.syso + packaging/windows/installer/rsrc_windows_${arch}.syso"
done

echo "==> Selesai."
