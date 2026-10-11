#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")" && pwd)"
cd "$ROOT"
mkdir -p dist

echo "==> Building Go binaries (cross-compile)..."
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -buildvcs=false -ldflags="-s -w" -o dist/simarc-server-linux-amd64 ./cmd/server
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -buildvcs=false -ldflags="-s -w" -o dist/simarc-server-linux-arm64 ./cmd/server
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -buildvcs=false -ldflags="-s -w" -o dist/simarc-server-windows-amd64.exe ./cmd/server
GOOS=windows GOARCH=arm64 CGO_ENABLED=0 go build -buildvcs=false -ldflags="-s -w" -o dist/simarc-server-windows-arm64.exe ./cmd/server
GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 go build -buildvcs=false -ldflags="-s -w" -o dist/simarc-server-darwin-amd64 ./cmd/server
GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -buildvcs=false -ldflags="-s -w" -o dist/simarc-server-darwin-arm64 ./cmd/server

echo "==> Building .deb (Linux amd64)..."
bash build-linux-deb.sh

echo "==> Done. Artifacts in dist/:"
ls -lh dist/ | awk '{print "  " $9 "  (" $5 ")"}'
