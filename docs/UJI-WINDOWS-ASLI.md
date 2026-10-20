# Uji Instalasi di Windows Asli — Checklist

Panduan ini dipakai untuk membuktikan `SIMARC-Setup.exe` berjalan **tanpa error**
di komputer Windows sungguhan (bukan Wine).

File yang diuji: `SIMARC-Setup.exe`
SHA-256: `50dd15c034aa65ee5b06417da30fc93aee2697564f7d090e10d8a351a5ab3b23`

---

## 0. Prasyarat

- Windows **10/11 64-bit** (x64). ARM64 belum didukung.
- Hak **Administrator** (untuk membuat shortcut & firewall).
- Ruang disk kosong **± 500 MB**.
- Salin `SIMARC-Setup.exe` ke komputer, misalnya ke Desktop.

> Nama file **harus** tepat `SIMARC-Setup.exe`. Jangan diganti — installer mencari
> payload di dalam dirinya sendiri.

---

## 1. Jalankan installer

1. Klik-dua-kali `SIMARC-Setup.exe`.
2. Jika muncul **"Windows protected your PC"** (SmartScreen):
   - klik **More info** → **Run anyway**.
   - Ini normal karena file **belum ditandatangani** (unsigned).

Yang **seharusnya** tidak muncul: tidak ada jendela CMD hitam. Ini installer GUI.

---

## 2. Wizard yang harus Anda lihat

> **Pemasangan massal (diam-diam):** bila berkas `simarc-install.txt` diletakkan
> **di folder yang sama** dengan `SIMARC-Setup.exe`, wizard **tidak muncul** —
> installer langsung memakai mode + alamat hub dari berkas itu (mode `offline`,
> hub database Aiven, berkas LAN). Inilah cara pemasangan di banyak komputer.

1. **Dialog pilih mode**:
   - Judul: `SIMARC Installer`
   - Pertanyaan: `Pilih mode instalasi SIMARC:`
   - Pilihan: **Offline (Mandiri)** *(terpilih)* dan **Klien Jaringan**
   - → untuk komputer mandiri, biarkan **Offline (Mandiri)**, klik OK.

   *(Jika Anda memilih Klien Jaringan, akan muncul kotak alamat HUB.)*

2. **Dialog progres**: `Menginstal SIMARC...` — tunggu 1–3 menit.

3. **Dialog selesai**:
   - `SIMARC berhasil diinstal.` → `Buka SIMARC sekarang?`
   - Tombol: **Buka SIMARC** / **Nanti**.

Jika ada kesalahan, justru inilah yang muncul: dialog `SIMARC - Kesalahan`
berisi teks errornya. **Screenshot dialog itu dan kirim ke saya.**

---

## 3. Firewall (hanya sekali)

Windows akan bertanya: *"Allow SIMARC to communicate on..."*.
Centang **Private networks** → **Allow access**.

(Kalau tidak, komputer lain tidak bisa mengakses `:8080` lewat LAN.)

---

## 4. Verifikasi aplikasi

1. Cek **Desktop** → ada shortcut **SIMARC**.
2. Cek **Start Menu** → ada **SIMARC**.
3. Klik shortcut **SIMARC**.
4. Browser otomatis terbuka ke `http://127.0.0.1:8080`
   → harus mengarah ke halaman **Direktori Arsip - SIMARC** (`/viewer`).
5. **Cek tampilan (CSS)** — halaman harus tampil **rapi ber-gaya**: ada warna,
   logo, dan tombol/tombol berwarna, bukan teks polos di latar putih.
   Uji cepat bila tampak polos: buka `http://127.0.0.1:8080/css/enterprise.min.css`
   di browser. Harus tampil **teks CSS** — **bukan** halaman `403 Forbidden` /
   "Access denied". (Ini pernah jadi bug: semua aset statis 403 di Windows.)
6. Login:
   - username: `admin`
   - password: `admin`
   - *(ganti password setelah login)*

   > **Penting (hub cloud):** pada instalasi **pertama** di tiap komputer,
   > aplikasi memeriksa hub. Bila hub sudah berisi (komputer ke-2 dst.), data
   > pengguna & peran **ditarik dari hub** — jadi username/password mengikuti
   > data hub, bukan seed bawaan. Selama admin hub belum pernah diganti,
   > login tetap `admin` / `admin`. Jangan mengubah nama pengguna admin di satu
   > komputer saja, karena bisa bentrok dengan hub.

Komponen yang seharusnya ada di `%LocalAppData%\SIMARC`:
```
run.bat
.env
tmp\simarc-server.exe
web\templates\layouts\app.html
mariadb\bin\mariadbd.exe
mariadb\bin\mariadb-install-db.exe
```

Cek port aktif (buka `cmd`):
```
netstat -ano | findstr ":8080 :3307"
```
harus muncul dua listener.

---

## 5. Komputer kedua — lihat data tanpa server

Tujuan: komputer lain bisa melihat arsip **walau komputer server mati**.

Caranya bukan mode klien (mode klien tergantung server), melainkan:

1. Salin **dua berkas** ke komputer kedua: `SIMARC-Setup.exe` **dan**
   `simarc-install.txt` (yang sudah berisi alamat hub Aiven + token berkas).
   Pilih/berlaku mode **Offline (Mandiri)** — installer otomatis karena
   `simarc-install.txt` ada di sebelahnya.
2. Setiap komputer punya database & aplikasinya sendiri → tetap mandiri.
3. Saat pertama dijalankan, aplikasi **menarik seluruh data dari hub Aiven**
   (peran, pengguna, izin, arsip) — lihat `docs/OFFLINE-SYNC.md`. Hub hanya
   dibutuhkan **saat sinkronisasi**, bukan saat membuka arsip. Berkas arsip
   ditarik dari aplikasi hub LAN (`SYNC_HUB_URL`) saat komputer berada di LAN
   yang sama.

---

## 6. Uji update (tidak kehilangan data)

1. Buka `%LocalAppData%\SIMARC\storage` dan buat file penanda, mis. `tes.txt`.
2. Jalankan `SIMARC-Setup.exe` lagi.
3. Setelah selesai: `tes.txt` **masih ada**, dan `.env` **tidak berubah**
   (password DB sama).

---

## 7. Kalau ada error — kirim ini ke saya

Sertakan **satu pesan** berisi:

1. **Versi Windows**: tekan `Win+R` → ketik `winver` → Enter → screenshot.
2. **Screenshot dialog error** (teksnya lengkap).
3. **Log server** (kalau aplikasi yang gagal):
   - buka `cmd`, lalu:
     ```
     cd %LocalAppData%\SIMARC
     run.bat
     ```
   - salin seluruh teks yang muncul di jendela.
4. **Isi folder instalasi**:
   ```
   dir /s /b "%LocalAppData%\SIMARC" > "%USERPROFILE%\Desktop\simarc-files.txt"
   ```
   lampirkan `simarc-files.txt`.
5. **Arsitektur**: `echo %PROCESSOR_ARCHITECTURE%` (harus `AMD64`).

---

## 8. Cara bersihkan total (kalau perlu uji ulang dari nol)

1. Tutup aplikasi & jendela `run.bat`.
2. Hapus folder:
   ```
   rmdir /s /q "%LocalAppData%\SIMARC"
   ```
3. Hapus shortcut **SIMARC** di Desktop & Start Menu.
4. Hapus entri firewall SIMARC (opsional): Control Panel → Windows Defender Firewall
   → Allow an app → hapus SIMARC.
5. Jalankan installer lagi.

---

## Ringkas: tanda "lulus"

- [ ] Installer muncul dengan wizard GUI (tanpa CMD hitam).
- [ ] Progres jalan sampai selesai, muncul "berhasil diinstal".
- [ ] Shortcut Desktop + Start Menu ada.
- [ ] Browser terbuka ke `Direktori Arsip - SIMARC`.
- [ ] Login `admin` / `admin` berhasil.
- [ ] `netstat` menunjukkan port `8080` dan `3307`.
- [ ] Update tidak menghapus `storage` & `.env`.

Kalau semua kotak tercentang → instalasi **terbukti tanpa error di Windows asli**.
