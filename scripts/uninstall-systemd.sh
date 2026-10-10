#!/usr/bin/env bash
# ===========================================================================
#  SIMARC — Lepas systemd service (server pusat)
#  Pemakaian: sudo ./scripts/uninstall-systemd.sh
# ===========================================================================
set -euo pipefail

UNIT_NAME="simarc"
UNIT_DST="/etc/systemd/system/${UNIT_NAME}.service"

if [[ $EUID -ne 0 ]]; then
    echo "Jalankan dengan sudo: sudo $0"
    exit 1
fi

systemctl disable --now "${UNIT_NAME}.service" 2>/dev/null || true
rm -f "$UNIT_DST"
systemctl daemon-reload
systemctl reset-failed "${UNIT_NAME}.service" 2>/dev/null || true
echo "Service '${UNIT_NAME}' dilepas. (Data & .env tidak dihapus.)"
