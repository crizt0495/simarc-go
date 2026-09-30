# SIMARC Desktop

Versi desktop dari SIMARC — jendela native di atas **webview sistem**, dengan
navigasi **top bar** di bagian atas (gaya iPOS) menggantikan sidebar kiri yang
dipakai versi web.

## Kenapa Wails

Aplikasi ini bukan SPA. Seluruh antarmukanya template Go yang dirender server,
CSS dan JavaScript statis, dan MariaDB/MySQL. Jadi tidak ada gunanya memaketkan
Chromium di dalam aplikasi (+150 MB) hanya untuk menampilkan HTML yang sudah
dimiliki server.

Wails (Go + webview sistem) memberi kita:

- **Binary kecil** — ±34 MB, bukan ±180 MB.
- **Sisi server yang sama persis** — `desktop/` hanya menjadi *"window yang
  membuka engine yang sudah ada"*. Tidak ada handler, route, atau template yang
  diduplikasi.
- **Installer asli** untuk Windows, macOS, dan Linux.

## Cara kerja

`desktop/main.go` mendefinisikan satu-satunya kontrak dengan aplikasi lain:

```go
func newEngine() (*gin.Engine, error) {
    handlers.SetLayoutMode(handlers.LayoutModeTopbar)  // satu-satunya perbedaan
    prepareWorkingDir()                                // dev vs. instalasi
    return app.Init()                                  // engine yang sama seperti cmd/server
}
```

 lalu engine itu diserahkan ke Wails sebagai asset server:

```go
AssetServer: &assetserver.Options{Handler: r}
```

Karena `Assets` tidak di-set, **semua** request dari window — termasuk
non-GET seperti `POST /login` — diteruskan ke engine. Cookie sesi, CSRF, dan
middleware berjalan persis seperti di browser. Tidak ada bundle frontend
 terpisah yang harus dijaga sinkron.

## Layout mode

`internal/handlers` memiliki dua mode:

| Mode | Pemakaian | Navigasi |
|---|---|---|
| `sidebar` (default) | `cmd/server`, Vercel | sidebar kiri + bottom nav mobile |
| `topbar` | binary desktop | 5 grup dropdown + "Lainnya" |

Yang **tidak** berubah di mode topbar: search bar, tombol tema, menu user,
dan semua konten halaman. Hanya kerangka navigasinya./css aturan barunya
 semuanya di-scope dengan `body.layout-topbar`, sehingga cascade sidebar
yang sudah ada tidak tersentuh.

`/profil` sengaja tidak ada di topnav — tetap bisa lewat menu user di header,
supaya top bar tidak Penuh.

## Direktori kerja

Aplikasi yang terpasang dijalankan oleh desktop environment tanpa kendali atas
working directory, jadi `prepareWorkingDir()` memilihnya:

- **Development checkout** → folder root repo. Template dan `.env` dibaca dari
  disk, jadi mengedit CSS cukup *restart*, tidak perlu rebuild.
- **Instalasi** → direktori data per pengguna. Template diambil dari yang
  *embedded* di dalam binary.

| OS | Lokasi data |
|---|---|
| Linux | `~/.local/share/simarc` |
| macOS | `~/Library/Application Support/SIMARC` |
| Windows | `%APPDATA%\SIMARC` |

Override dengan `SIMARC_DATA_DIR`.

### `.env` di aplikasi terpasang

`config.Load()` mencari `.env` di: working directory → folder binary →
direktori data. Variabel lingkungan yang sudah ada **selalu menang**.

Jadi setelah memasang, taruh konfigurasi database di
`~/.local/share/simarc/.env` (Linux/macOS) atau `%APPDATA%\SIMARC\.env`
(Windows):

```
DB_HOST=127.0.0.1
DB_PORT=3306
DB_DATABASE=defaultdb
DB_USERNAME=avnadmin
DB_PASSWORD=...
SESSION_KEY=<acak-panjang>
```

Tanpa ini, halaman akan menampilkan 503 karena tidak bisa konek database.

## Build

Wails memakai cgo dan webview platform, jadi **binary hanya bisa dibangun di
OS tempat ia akan berjalan**. Untuk ketiganya sekaligus, pakai CI.

```bash
make desktop-tools   # pasang Wails CLI + header webview
make desktop         # binary untuk OS ini → desktop/build/bin/SIMARC
make desktop-run     # build lalu jalankan
```

### Linux

Butuh header WebKitGTK **4.1** (bukan 4.0 — 4.0 sudah EOL):

```bash
sudo apt install libwebkit2gtk-4.1-dev libgtk-3-dev build-essential pkg-config
make desktop
```

Kalau WebKitGTK di mesin Anda hanya 4.0, ganti tag `webkit2_41` dengan
`webkit2_40`. Kalau GPU tidak tersedia, paksa software rendering:

```bash
WEBKIT_DISABLE_COMPOSITING_MODE=1 WEBKIT_DISABLE_DMABUF_RENDERER=1 \
  LIBGL_ALWAYS_SOFTWARE=1 ./desktop/build/bin/SIMARC
```

### macOS

Butuh Xcode Command Line Tools. Tidak ada tag webview.

```bash
make desktop
```

### Windows

Butuh WebView2 Runtime (sudah ada di Windows 11 dan Windows 10 terbaru).
Tidak ada tag webview.

```powershell
make desktop
```

### Tag yang wajib ada

`production` **tidak opsional**. Tanpa itu `wails.Run` mengembalikan error
dan keluar:

```
Wails applications will not build without the correct build tags.
```

`wails build` menambahkannya sendiri. Kalau membangun dengan `go build`
langsung:

```bash
go build -tags "desktop webkit2_41 production" -ldflags "-w -s" -o SIMARC ./desktop
```

## Installer

| OS | Perintah | Hasil |
|---|---|---|
| Linux | `./scripts/package-linux.sh` | `.deb`, `.tar.gz`, AppImage |
| macOS | `cd desktop && wails build -tags desktop -skipbindings -package` | `.app`, `.dmg` |
| Windows | `cd desktop && wails build -tags desktop -skipbindings -nsis` | `-setup.exe` |

Paket Linux dirakit tangan dengan `dpkg-deb` sehingga tidak butuh tooling
tambahan. AppImage hanya dibuat bila `appimagetool` terpasang.

Untuk semuanya sekaligus: push tag `v1.0.0`, dan
`.github/workflows/desktop-release.yml` membangun ketiganya di runner masing-masing
lalu mempublikasikan release-nya.

## Memasang di mesin ini (Linux)

Tanpa root, ke direktori user:

```bash
make desktop-install
```

Lalu jalankan dari menu aplikasi, atau `~/.local/bin/SIMARC`.

Dengan `.deb` (butuh root):

```bash
sudo apt install ./desktop/build/dist/simarc_<versi>_amd64.deb
```

## Diagnostik

Tidak ada port untuk `curl` — window bicara lewat transport in-process.
Karena itu ada dua variabel lingkungan:

```bash
# satu baris per request yang dilakukan window
SIMARC_DESKTOP_LOG=1 ./SIMARC

# simpan HTML halaman pertama yang dirender webview
SIMARC_DESKTOP_DUMP=/tmp/dom.html ./SIMARC
```

Dump berguna untuk memverifikasi apa yang benar-benar tampil tanpa perlu
screenshot. Keduanya mati secara default, jadi sesi biasa tidak incursi
biaya.

## Test

```bash
make test-desktop
```

`desktop/shell_test.go` menjalankan engine yang **sungguhan** (login sungguhan
terasuk CSRF token), lalu untuk setiap halaman memastikan top bar ada dan
sidebar tidak ada. Ini yang menjaga "desktop selalu topbar, tidak pernah
sidebar" dari regresi.

## Ikon

Aset di `desktop/build/` (PNG, ICO, ICNS) **di-commit** — itulah yang
menghasilkan installer. Buat ulang setelah mengganti warna brand:

```bash
make desktop-icons
```

Skripnya menggambar ulang geometri `web/static/images/logo-icon.svg` dengan
Pillow, jadi tidak ada ketergantungan perender SVG.
