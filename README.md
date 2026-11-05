⚙️ SIMARC — Aplikasi Desktop (Wails + Golang + React)

Aplikasi desktop standalone untuk manajemen arsip (offline-first).

  • Backend   : Golang (Wails) — seluruh proses bisnis & akses DB.
  • Frontend  : React + TypeScript + Tailwind, di-embed ke executable.
  • Database  : MySQL / MariaDB lokal (per komputer).
  • Jendela   : WebView bawaan OS — tidak membuka browser eksternal.

Catatan importante:
  • Mode production wajib pakai `wails build` (bukan `wails dev`).
  • Tidak ada server web/node di produksi:
      - `frontend/dist` di-embed (`//go:embed all:frontend/dist`).
      - `migrations/` di-embed dan diterapkan otomatis saat aplikasi start.
  • Aplikasi tetap berjalan tanpa internet; sinkronisasi ke hub opsional
    (SYNC_ENABLED=true + SYNC_HUB_URL=…).

Build (Linux):
  export PATH=$PATH:$HOME/go/bin
  wails build -tags webkit2_41      # binary: build/bin/SIMARC
  # Atau pakai packaging script:
  packaging/linux/build-deb.sh      # menghasilkan dist/simarc_1.0.0-1_amd64.deb

Build (Windows — cross-compile dari Linux):
  export PATH=$PATH:$HOME/go/bin:$HOME/toolchain/llvm-mingw*/bin
  export CC=x86_64-w64-mingw32-gcc CXX=x86_64-w64-mingw32-g++
  wails build -platform windows/amd64 -clean
  # Installer Windows: ikuti instruksi di packaging/simarc.iss (ISCC di Windows).

Konfigurasi (file `.env` di samping aplikasi):
  SIMARC_WINDOW=fullscreen|maximized|normal|kiosk
  DB_HOST, DB_PORT, DB_DATABASE, DB_USERNAME, DB_PASSWORD, DB_TLS
  SYNC_ENABLED, SYNC_NODE_ID, SYNC_HUB_URL, SYNC_HUB_TOKEN, SYNC_INTERVAL_MINUTES

Seluruh konfigurasi bisa diubah dari UI: menu "Pengaturan → Simpan"
(menyunting file .env tanpa mengubah komentar).
