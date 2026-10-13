# SIMARC — Banyak Komputer dengan Satu Database & Satu Penyimpanan Berkas

Panduan menjalankan SIMARC di **beberapa komputer sekaligus** (mis. 1 komputer
server + beberapa komputer petugas) dengan satu database dan satu penyimpanan
berkas bersama.

> **Butuh tiap komputer tetap jalan tanpa jaringan?** Lihat
> [`docs/OFFLINE-SYNC.md`](OFFLINE-SYNC.md) — setiap komputer memakai database
> sendiri (offline penuh) lalu menyatu otomatis lewat satu "hub". Mode klien di
> dokumen ini adalah alternatifnya (satu server pusat + klien tipis).

---

## 1. Kenapa harus "server pusat + klien"?

SIMARC menyimpan **berkas arsip di disk** — database hanya mencatat *lokasi*
berkas. Karena itu, model "setiap komputer pakai database yang sama tapi server
sendiri" **akan bermasalah**: berkas yang di-upload di komputer A tidak akan
muncul di komputer B (karena berkasnya ada di disk A, bukan B).

| Cara | Database terhubung? | Berkas ikut tersalin? | Disarankan? |
|------|:-------------------:|:---------------------:|:-----------:|
| Semua konek langsung ke DB yang sama | Ya | ❌ Tidak | ❌ Tidak |
| Satu **server pusat**, sisanya **klien** | Ya (di server) | ✅ Ya | ✅ **Ya** |

Solusi yang benar: jalankan **satu server pusat**, lalu komputer lain menjadi
**klien** yang hanya membuka jendela aplikasi ke server pusat.

> Mode klien **tidak** menjalankan server lokal dan **tidak** menyambung ke
> database. Semua data dan berkas datang dari server pusat.

---

## 2. Menyiapkan SERVER PUSAT (satu komputer / VPS)

Server pusat memegang database (MySQL/MariaDB lokal **atau** Aiven) dan seluruh
berkas arsip. Pada komputer ini isi `.env` seperti biasa (bagian `DB_*`), lalu
jalankan server.

Pastikan alamat jaringannya, mis. `http://192.168.1.10:8080`. Alamat ini
ditampilkan di banner saat server dijalankan.

```bash
# cara cepat (tanpa membuka browser di server)
SIMARC_NO_BROWSER=1 ./tmp/simarc-server
```

**Agar server pusat otomatis menyala saat komputer dinyalakan** (systemd):

```bash
sudo ./systemd/install-systemd.sh            # pasang & aktifkan
sudo ./systemd/install-systemd.sh --user chris
```

Buka **port server** (mis. 8080) di firewall LAN/komputer server.

---

## 3. Menyiapkan KOMPUTER KLIEN (satu perintah)

Di setiap komputer klien, cukup arahkan ke alamat server pusat. Tidak perlu
MySQL, tidak perlu konfigurasi database.

### Linux / macOS / WSL

```bash
./setup-client.sh http://192.168.1.10:8080
# atau interaktif:
./setup-client.sh
```

Skrip akan menulis `SIMARC_SERVER_URL` ke `.env` dan menguji koneksi ke server.

### Windows

```powershell
powershell -ExecutionPolicy Bypass -File .\setup-client.ps1 -ServerUrl http://192.168.1.10:8080
# atau interaktif:
powershell -ExecutionPolicy Bypass -File .\setup-client.ps1
```

### Manual (alternatif)

Buat/sunting `.env` dan isi satu baris berikut, lalu simpan:

```bash
SIMARC_SERVER_URL=http://192.168.1.10:8080
```

Contoh `.env` klien minimal ada di `.env.client.example`.

### Menjalankan

```bash
./run-desktop.sh          # Linux / macOS
```

Windows: klik dua kali **run.bat** (atau ikon **SIMARC** di menu).

Hasilnya: semua komputer memakai **database dan berkas yang sama**, tanpa
menyalin data apa pun.

---

## 4. Mengembalikan ke mode server lokal

Kosongkan nilai `SIMARC_SERVER_URL` di `.env`:

```bash
SIMARC_SERVER_URL=
```

Aplikasi kembali menjalankan server lokal (butuh MySQL/MariaDB lokal dan
konfigurasi `DB_*`).

---

## 5. Troubleshooting

| Gejala | Penyebab umum | Solusi |
|--------|---------------|--------|
| Halaman "tidak dapat dihubungi / connection refused" di klien | Server pusat mati atau alamat salah | Nyalakan server pusat, cek `SIMARC_SERVER_URL` |
| Klien jalan tapi **blank / terus memuat** | Port diblokir firewall, atau beda jaringan | Buka port (mis. 8080) di firewall server |
| Muncul halaman **Konfigurasi Database** | Klien masih mode server lokal (tanpa DB lokal) | Isi `SIMARC_SERVER_URL` di `.env` klien (mode klien) |
| Login berhasil tapi **berkas tidak muncul** | Bukan mode klien (tiap komputer server sendiri) | Pakai mode klien ke server pusat |
| Dari **luar kantor** tidak bisa diakses | Hanya jaringan LAN | Gunakan domain + reverse proxy (HTTPS) atau VPN |
| Klien Windows error "Go belum terinstall" | `run.ps1` lama dipakai di mode server | Gunakan `run.bat` untuk mode klien, atau `setup-client.ps1` |

---

## 6. Catatan keamanan

- Saat database **tidak** terhubung, aplikasi menampilkan halaman pemulihan
  (`/database-setup`) yang dapat diakses tanpa login. Pastikan server pusat
  hanya dijangkau dari jaringan tepercaya (LAN/VPN). Lihat peringatan di
  `README.md`.
- Untuk akses internet, **jangan** buka MySQL langsung ke publik. Cukup buka
  port aplikasi (mis. 8080) melalui HTTPS/reverse proxy atau VPN.
- Jangan pernah meng-commit file `.env` (berisi kredensial) ke git.
