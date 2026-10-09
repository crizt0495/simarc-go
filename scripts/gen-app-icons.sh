#!/usr/bin/env bash
# ===========================================================================
#  SIMARC — Generator Ikon Aplikasi
#  Mengubah web/static/images/logo-icon.svg menjadi:
#    - icon-<ukuran>.png (16..512, tema hicolor)
#    - apple-touch-icon.png (180)
#    - icon-maskable-192/512.png (PWA maskable)
#    - favicon.ico (multi-ukuran)
#
#  Prasyarat: Chrome/Chromium + python3 + Pillow.
#  Pemakaian:  ./scripts/gen-app-icons.sh   (atau set CHROME=/path/ke/chrome)
# ===========================================================================
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
IMG="$ROOT/web/static/images"
MASTER="$IMG/logo-icon.svg"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

CHROME="${CHROME:-$(command -v google-chrome || command -v chromium || command -v chromium-browser || true)}"
if [[ -z "$CHROME" ]]; then
    echo "Chrome/Chromium tidak ditemukan. Set env CHROME=/path/ke/chrome" >&2
    exit 1
fi

# Varian maskable: background memenuhi seluruh kanvas (aman untuk safe-zone).
python3 - "$MASTER" "$TMP/maskable.svg" <<'PY'
import sys
src = open(sys.argv[1]).read()
old = '''  <rect x="10" y="10" width="180" height="180" rx="40" fill="url(#bg)"/>
  <rect x="10" y="10" width="180" height="180" rx="40" fill="url(#glass)"/>
  <rect x="11.5" y="11.5" width="177" height="177" rx="38.5" fill="none" stroke="#FFFFFF" stroke-opacity="0.22" stroke-width="1.5"/>'''
new = '''  <rect x="0" y="0" width="200" height="200" fill="url(#bg)"/>
  <rect x="0" y="0" width="200" height="200" fill="url(#glass)"/>'''
if old not in src:
    raise SystemExit("Pola background pada logo-icon.svg tidak ditemukan")
open(sys.argv[2], "w").write(src.replace(old, new))
PY

render() { # <svg> <out-png>
    local html="$TMP/render.html"
    cat > "$html" <<HTML
<!doctype html><html><head><meta charset="utf-8">
<style>html,body{margin:0;padding:0;background:transparent;overflow:hidden}img{display:block;width:512px;height:512px}</style>
</head><body><img src="file://$1"></body></html>
HTML
    "$CHROME" --headless=new --disable-gpu --no-sandbox --hide-scrollbars \
        --default-background-color=00000000 --user-data-dir="$TMP/profile" \
        --window-size=512,512 --screenshot="$2" "file://$html" >/dev/null 2>&1
}

echo "Merender SVG…"
render "$MASTER" "$TMP/master512.png"
render "$TMP/maskable.svg" "$TMP/mask512.png"

python3 - "$TMP" "$IMG" <<'PY'
import sys
from PIL import Image
tmp, img = sys.argv[1], sys.argv[2]
master = Image.open(f"{tmp}/master512.png").convert("RGBA")
mask = Image.open(f"{tmp}/mask512.png").convert("RGBA")
for s in (16, 24, 32, 48, 64, 128, 192, 256, 512):
    master.resize((s, s), Image.LANCZOS).save(f"{img}/icon-{s}.png")
master.resize((180, 180), Image.LANCZOS).save(f"{img}/apple-touch-icon.png")
for s in (192, 512):
    mask.resize((s, s), Image.LANCZOS).save(f"{img}/icon-maskable-{s}.png")
master.resize((256, 256), Image.LANCZOS).save(
    f"{img}/favicon.ico", sizes=[(16, 16), (24, 24), (32, 32), (48, 48), (64, 64)]
)
print("Ikon aplikasi diperbarui di", img)
PY
