#!/usr/bin/env bash
# ===========================================================================
#  SIMARC — Build binary rilis (cross-compile) untuk dipasang di banyak PC.
#
#  Menghasilkan binary siap-pakai di folder dist/ sehingga komputer klien
#  TIDAK perlu meng-install Go. Launcher (run-desktop.sh / run.sh / run.bat)
#  otomatis memakai binary di dist/ bila ada.
#
#  Pemakaian:  ./scripts/build-release.sh
# ===========================================================================
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
OUT="$ROOT/dist"
mkdir -p "$OUT"

build() {
    local os="$1" arch="$2" name="$3"
    printf '  %-8s %-6s -> %s\n' "$os" "$arch" "$name"
    GOOS="$os" GOARCH="$arch" CGO_ENABLED=0 \
        go build -buildvcs=false -ldflags="-s -w" -o "$OUT/$name" ./cmd/server
}

echo "Membangun binary rilis SIMARC..."
build linux   amd64 "simarc-server-linux-amd64"
build linux   arm64 "simarc-server-linux-arm64"
build windows amd64 "simarc-server-windows-amd64.exe"
build windows arm64 "simarc-server-windows-arm64.exe"
build darwin  amd64 "simarc-server-darwin-amd64"
build darwin  arm64 "simarc-server-darwin-arm64"

echo
echo "Selesai. Binary tersedia di: $OUT"
ls -lh "$OUT"
