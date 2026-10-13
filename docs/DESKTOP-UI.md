# Tampilan Desktop — SIMARC Modern Layer

Dokumen ini menjelaskan lapisan UI **"desktop modern"** yang ditambahkan di
atas design system SIMARC. Tujuannya memberi *rasa aplikasi desktop kelas
dunia*: cepat, keyboard-first, tenang, dan rapi — tanpa mengubah struktur
halaman yang sudah ada.

## Referensi (riset 2026)

Pola yang diadopsi berasal dari aplikasi desktop/aplikasi web terbaik:

| Referensi | Pola yang diambil |
|---|---|
| **Linear** | *Soft chrome* (topbar tenang), densitas navigasi, keyboard-first |
| **Raycast** | Command palette (⌘K / Ctrl+K) sebagai tulang navigasi |
| **Notion** | Palet netral, tipografi tenang, satu warna aksen |
| **Vercel** | Topbar translusen (glass) + border halus |
| **Apple HIG** | Focus ring konsisten, motion halus, dukungan `prefers-reduced-motion` |

## Yang diimplementasikan

### 1. Command palette (⌘K / Ctrl+K)
- Berkas: `web/templates/components/command-palette.html`,
  `web/static/js/command-palette.js`.
- Buka dengan **Ctrl+K** (Windows/Linux) atau **⌘K** (macOS), tombol
  **command bar** di topbar, atau tombol **`/`**.
- Berisi: **Terakhir** (riwayat pilihan, disimpan di `localStorage`),
  **Navigasi** (semua halaman), **Aksi** (ganti tema, pintasan keyboard,
  backup, muat ulang, keluar), dan **Pencarian** — mengetik bebas lalu
  Enter akan membuka `/search?q=…`.
- Navigasi keyboard penuh: `↑`/`↓`, `Home`/`End`, `Enter`, `Esc`, `Tab`
  terperangkap di dalam dialog.
- Aksesibel: `role="dialog"` + `aria-modal`, input `role="combobox"`,
  hasil `role="listbox"`, item `role="option"` + `aria-selected`.

### 2. Command bar di topbar
- Menggantikan kolom pencarian lama (`.nav-search`) dengan tombol
  `.cmdk-trigger` yang menampilkan label + petunjuk `Ctrl K`.
- Di layar 992–1199px otomatis menciut menjadi ikon saja agar topbar tetap
  lega. Di bawah 992px disembunyikan (pencarian mobile tetap dipakai).

### 3. Lapisan poles (`desktop-modern.css`)
- Tipografi: fitur optis Inter (`cv02/cv03/cv04/cv11/ss01`), `antialiased`,
  *tracking* negatif pada judul.
- Topbar translusen (*glass*) dengan `backdrop-filter`, plus fallback aman.
- Satu **focus ring** aksesibel untuk semua kontrol (`:focus-visible`).
- Scrollbar kustom tipis, warna seleksi teks, transisi tombol/kartu halus.
- Menghormati `prefers-reduced-motion`.

## Berkas yang terlibat

| Berkas | Peran |
|---|---|
| `web/static/css/desktop-modern.css` | Sumber lapisan modern + gaya palette |
| `web/static/css/desktop-modern.min.css` | Versi produksi (di-*link* layout) |
| `web/static/js/command-palette.js` | Logika palette (data, filter, keyboard) |
| `web/templates/components/command-palette.html` | Markup palette |
| `web/templates/layouts/app.html` | *Link* CSS, trigger topbar, include palette, skrip, pintasan |

## Alur build & deploy

Karena template & aset **di-embed** ke biner (`go:embed web/templates web/static`),
setiap perubahan wajib di-*rebuild* agar berlaku:

```bash
# 1) (bila CSS berubah) minifikasi CSS
npm run css:minify

# 2) hentikan layanan, build, jalankan kembali
echo 04 | sudo -S systemctl stop simarc
CGO_ENABLED=0 go build -buildvcs=false -ldflags="-s -w" -o tmp/simarc-server ./cmd/server
echo 04 | sudo -S systemctl start simarc
```

> `AssetVersion` adalah *hash* isi `web/static`, jadi begitu biner baru jalan
> `?v=<hash>` berubah otomatis — cache browser/CDN ter-*bust* tanpa aksi manual.
> Aplikasi desktop cukup **muat ulang** (Ctrl+R / F5).

## Cara memakai

- **Ctrl+K** / **⌘K** — buka command palette di halaman mana pun.
- Ketik untuk mencari halaman; panah + Enter untuk membuka.
- Ketik nama arsip lalu **Enter** untuk mencari di `/search`.
- Atau klik **"Cari atau jalankan perintah…"** di topbar.

## Menambah item ke palette

Tambahkan objek ke `NAV_ITEMS` (punya `href`) atau `ACTION_ITEMS` (punya
`action`) di `web/static/js/command-palette.js`:

```js
{ id: 'nav:/contoh', label: 'Contoh Halaman', href: '/contoh',
  icon: 'bi-star', section: 'Navigasi', keywords: 'kata kunci pencarian' }
```

`keywords` membantu pencocokan (mis. "backup" menemukan "Backup & Restore").

## Catatan

- `dark-mode.css`, `enterprise-premium.css`, `mobile-material.css` adalah
  berkas lama yang **tidak** di-*link* oleh layout manapun (design system
  aktif ada di `enterprise.min.css`). Lapisan baru ini sengaja ditaruh di
  berkas terpisah dan dimuat **setelah** `enterprise.min.css`.
- Perubahan bersifat aditif — tidak menghapus komponen lama, sehingga
  aman untuk semua halaman.
