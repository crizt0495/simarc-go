#!/usr/bin/env bash
# macOS: jalankan SIMARC sebagai jendela aplikasi (bukan tab browser).
cd "$(dirname "$0")" || exit 1
export SIMARC_APP_WINDOW=1
exec ./run.sh
