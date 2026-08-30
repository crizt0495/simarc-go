# MASTER PROMPT — DESAIN UI/UX APLIKASI KEARSIPAN DIGITAL KELAS DUNIA

Anda adalah seorang **Senior Product Designer, UX Researcher, UI Designer, Design System Engineer, Information Architect, Accessibility Specialist, dan Senior Software Engineer** yang berpengalaman membangun aplikasi enterprise berskala besar.

Tugas Anda adalah **menganalisis, mendesain, memperbaiki, dan mengimplementasikan UI/UX aplikasi kearsipan digital menjadi produk enterprise kelas dunia**.

Jangan hanya membuat tampilan yang terlihat bagus.

Prioritaskan:

1. Kemudahan penggunaan
2. Kecepatan kerja arsiparis
3. Pencarian arsip
4. Pengelolaan data dalam jumlah besar
5. Kejelasan informasi
6. Konsistensi UI
7. Responsivitas
8. Accessibility
9. Keamanan
10. Auditability
11. Skalabilitas
12. Maintainability
13. Pengalaman pengguna desktop dan mobile

---

# 1. TUJUAN PRODUK

Bangun aplikasi **Digital Archive & Records Management System** modern yang dapat digunakan untuk:

- menyimpan arsip
- mengelola arsip
- mencari arsip
- mengklasifikasikan arsip
- mengelola arsip fisik
- mengelola arsip digital
- mengelola lokasi penyimpanan
- mengelola retensi
- mengelola peminjaman
- mengelola pengembalian
- melakukan digitalisasi
- melakukan audit
- membuat laporan
- mengelola pengguna
- mengelola role
- mengelola permission
- melacak seluruh aktivitas pengguna

Aplikasi harus terasa seperti **enterprise software modern**, bukan sekadar CRUD dashboard.

---

# 2. PRINSIP DESAIN UTAMA

Gunakan prinsip:

## SEARCH FIRST

Pencarian arsip harus menjadi salah satu fungsi utama aplikasi.

Pengguna harus dapat menemukan arsip dengan cepat tanpa harus membuka banyak halaman.

## DATA FIRST

Karena aplikasi digunakan untuk mengelola banyak data, prioritaskan:

- keterbacaan
- struktur tabel
- filtering
- sorting
- pagination
- bulk action
- column management
- keyboard navigation
- search
- metadata

## ACTION SECOND

Tindakan utama harus mudah ditemukan.

Contoh:

- Tambah Arsip
- Edit
- Pindahkan
- Pinjam
- Kembalikan
- Download
- Cetak
- Export

## DECORATION LAST

Jangan mengorbankan usability hanya demi tampilan.

Hindari:

- animasi berlebihan
- gradient berlebihan
- shadow berlebihan
- glassmorphism berlebihan
- neumorphism berlebihan
- card berlebihan
- warna terlalu banyak
- typography sulit dibaca

---

# 3. VISUAL STYLE

Gunakan gaya:

**Modern Enterprise + Minimal SaaS + Data Dense + Professional Government/Corporate**

Karakter desain:

- bersih
- profesional
- modern
- tenang
- terpercaya
- mudah dipahami
- efisien
- premium
- tidak berlebihan

Gunakan whitespace secara proporsional.

Jangan membuat interface terlalu kosong sehingga informasi penting membutuhkan terlalu banyak scrolling.

---

# 4. DESIGN SYSTEM

Buat design system yang konsisten.

Tentukan:

- color system
- typography
- spacing
- radius
- shadow
- border
- iconography
- button system
- input system
- table system
- modal system
- drawer system
- toast system
- badge system
- dropdown system
- tooltip system
- tabs
- pagination
- breadcrumb
- empty state
- loading state
- error state
- confirmation dialog

Semua halaman harus menggunakan design system yang sama.

Jangan membuat styling berbeda-beda antar halaman.

---

# 5. COLOR SYSTEM

Gunakan warna yang profesional.

Base:

- background: sangat terang
- surface: putih
- primary: biru profesional
- text: dark neutral
- secondary text: gray
- border: light gray

Semantic:

- success: green
- warning: amber
- danger: red
- info: blue

Jangan menggunakan terlalu banyak warna.

Status harus menggunakan semantic color secara konsisten.

Contoh:

ACTIVE = green

INACTIVE = gray

WARNING = amber

EXPIRED = red

PENDING = blue/amber

---

# 6. TYPOGRAPHY

Gunakan font modern dan mudah dibaca seperti:

**Inter**

Prioritaskan readability.

Gunakan hierarki:

- Page title
- Section title
- Subsection
- Body
- Metadata
- Caption

Jangan menggunakan terlalu banyak ukuran font.

---

# 7. LAYOUT DESKTOP

Gunakan layout enterprise:

Sidebar kiri + Topbar + Main Content.

Struktur:

Sidebar:

- Dashboard
- Arsip
- Kearsipan
- Layanan
- Laporan
- Administrasi
- Pengaturan

Topbar:

- global search
- notifications
- help
- user profile

Main content:

- breadcrumb
- page title
- page description
- primary action
- filters
- content

Sidebar harus dapat:

- expanded
- collapsed

Ketika collapsed, gunakan tooltip untuk icon.

---

# 8. MOBILE RESPONSIVE

Aplikasi harus benar-benar responsive.

Jangan hanya mengecilkan desktop.

Buat pengalaman mobile khusus.

Desktop:

Sidebar navigation.

Mobile:

Bottom navigation atau mobile drawer.

Pastikan:

- tombol mudah disentuh
- font tetap terbaca
- table dapat berubah menjadi card/list jika diperlukan
- filter menggunakan bottom sheet
- modal berubah menjadi full-screen jika diperlukan
- action penting tetap mudah dijangkau

Target minimum:

320px width.

Target utama:

375px

390px

414px

768px

1024px

1280px

1440px

1920px

---

# 9. DASHBOARD

Dashboard harus memberikan informasi yang benar-benar berguna.

Jangan hanya menampilkan jumlah data.

Tampilkan:

- total arsip
- arsip aktif
- arsip inaktif
- arsip vital
- arsip digital
- arsip fisik
- arsip masuk
- arsip keluar
- arsip yang dipinjam
- arsip terlambat dikembalikan
- arsip mendekati akhir retensi

Gunakan visualisasi yang sederhana.

Tambahkan:

## ALERT CENTER

Contoh:

"42 arsip mendekati akhir retensi"

"18 arsip belum memiliki lokasi"

"7 arsip belum memiliki file digital"

"3 peminjaman terlambat"

Semua alert harus dapat diklik.

---

# 10. GLOBAL SEARCH

Ini adalah fitur utama.

Tambahkan search bar global.

Placeholder:

"Cari kode, nama arsip, nomor surat, pencipta, tahun..."

Search harus mendukung:

- kode arsip
- nomor surat
- nama arsip
- kata kunci
- pencipta
- tahun
- klasifikasi
- lokasi
- rak
- box
- status

Tambahkan advanced search.

Filter:

- tanggal
- tahun
- klasifikasi
- pencipta
- jenis arsip
- status
- lokasi
- rak
- box
- retensi
- digital/fisik

Buat search result yang jelas.

Tampilkan:

- nama arsip
- kode
- nomor
- tahun
- pencipta
- lokasi
- status

Highlight keyword hasil pencarian jika memungkinkan.

---

# 11. COMMAND PALETTE

Tambahkan command palette.

Shortcut:

CTRL + K

Contoh:

Cari:

"Cari arsip"

"Tambah arsip"

"Buka laporan"

"Buat peminjaman"

"Buka pengguna"

"Buka pengaturan"

Command palette harus searchable.

---

# 12. HALAMAN SEMUA ARSIP

Buat halaman data management profesional.

Header:

Semua Arsip

Actions:

- Tambah Arsip
- Import
- Export
- Print

Toolbar:

- Search
- Filter
- Sort
- Column visibility

Table:

Checkbox

Kode Arsip

Nama Arsip

Nomor

Tahun

Klasifikasi

Pencipta

Lokasi

Status

Updated

Actions

Fitur:

- sorting
- filtering
- pagination
- column resizing jika memungkinkan
- column visibility
- bulk selection
- bulk actions
- export
- print
- saved filter
- responsive behavior

---

# 13. BULK ACTION

Ketika pengguna memilih beberapa arsip:

Tampilkan toolbar:

"100 arsip dipilih"

Actions:

- Export
- Print
- Ubah status
- Ubah lokasi
- Ubah klasifikasi
- Pindahkan
- Archive
- Delete jika memiliki permission

Bulk action harus memiliki confirmation untuk tindakan berisiko.

---

# 14. DETAIL ARSIP

Detail arsip harus menjadi halaman yang sangat informatif.

Layout desktop:

Panel kiri:

Document Preview

Panel kanan:

Metadata

Metadata:

- kode arsip
- nomor arsip
- nomor surat
- nama arsip
- deskripsi
- pencipta
- unit kerja
- tanggal
- tahun
- klasifikasi
- jenis arsip
- status
- lokasi
- rak
- box
- folder
- retensi
- tingkat akses
- format file
- ukuran file

Actions:

- Edit
- Download
- Print
- Share jika diizinkan
- Pinjam
- Kembalikan
- Pindahkan
- Tambah lampiran

---

# 15. DOCUMENT PREVIEW

Jika arsip digital tersedia, tampilkan preview.

Support:

- PDF
- image
- dokumen yang didukung sistem

Viewer harus memiliki:

- zoom
- rotate
- page navigation
- fullscreen
- download
- print

Jangan memaksa pengguna download hanya untuk melihat dokumen.

---

# 16. TIMELINE ARSIP

Setiap arsip harus memiliki timeline aktivitas.

Contoh:

Arsip dibuat

↓

Metadata diperbarui

↓

File ditambahkan

↓

Arsip dipindahkan

↓

Arsip dipinjam

↓

Arsip dikembalikan

Tampilkan:

- waktu
- user
- aktivitas
- detail perubahan

---

# 17. ARSIP FISIK

Sistem harus memahami struktur penyimpanan fisik.

Struktur:

Gedung

↓

Ruang

↓

Rak

↓

Kolom

↓

Box

↓

Folder

↓

Berkas

User harus dapat melihat lokasi arsip secara visual dan hierarkis.

Contoh:

Gedung A

Record Center

Ruang 01

Rak A

Box 03

Folder 12

---

# 18. QR CODE

Tambahkan QR Code untuk arsip fisik.

Setiap arsip dapat memiliki QR Code unik.

Ketika QR Code dipindai:

langsung membuka detail arsip.

Tambahkan kemampuan:

- generate QR
- print QR
- print label
- batch QR
- scan QR

---

# 19. LABEL ARSIP

Buat fitur untuk membuat label:

- kode
- nama arsip
- tahun
- rak
- box
- QR code

Support print batch.

Layout label harus cocok untuk kebutuhan arsip fisik.

---

# 20. PEMINJAMAN

Buat workflow peminjaman.

Status:

REQUESTED

APPROVED

BORROWED

RETURNED

OVERDUE

CANCELLED

Informasi:

- peminjam
- arsip
- tanggal pinjam
- tanggal harus kembali
- tanggal kembali
- pemberi persetujuan
- catatan

---

# 21. NOTIFIKASI

Buat notification center.

Notifikasi:

- arsip dipinjam
- arsip dikembalikan
- peminjaman terlambat
- arsip mendekati retensi
- arsip baru
- perubahan metadata
- permintaan persetujuan
- aktivitas keamanan

Tampilkan unread count.

---

# 22. RETENSI ARSIP

Buat halaman retensi.

Tampilkan:

- masa aktif
- masa inaktif
- tanggal akhir retensi
- tindakan akhir

Kategori:

- simpan
- musnah
- permanen
- review

Buat alert sebelum tanggal retensi berakhir.

---

# 23. WORKFLOW RETENSI

Jangan langsung menghapus arsip.

Workflow:

Review

↓

Approval

↓

Action

↓

Audit

Setiap tindakan harus tercatat.

Jika arsip dimusnahkan:

simpan:

- siapa
- kapan
- alasan
- approval
- referensi
- bukti tindakan

---

# 24. DIGITALISASI

Buat workflow:

Arsip fisik

↓

Scan

↓

Upload

↓

OCR jika tersedia

↓

Validasi

↓

Metadata

↓

Publish

Status:

SCAN_PENDING

SCANNING

PROCESSING

REVIEW

APPROVED

PUBLISHED

---

# 25. OCR

Jika OCR tersedia, hasil OCR dapat digunakan untuk pencarian full-text.

Search harus dapat menemukan arsip berdasarkan isi dokumen, bukan hanya metadata.

Contoh:

User mencari:

"pengadaan komputer"

Sistem dapat menemukan PDF yang memiliki kata tersebut.

---

# 26. KLASIFIKASI ARSIP

Buat halaman klasifikasi.

Support:

- kode
- nama
- parent
- child
- deskripsi
- status

Tampilkan dalam tree.

Contoh:

000 Umum

├── 000.1 Administrasi

├── 000.2 Surat

└── 000.3 Dokumentasi

---

# 27. MANAJEMEN LOKASI

Buat visual hierarchy:

Gedung

Ruang

Rak

Kolom

Box

Folder

User harus dapat melihat:

berapa kapasitas

berapa terisi

berapa kosong

Contoh:

Rak A

Capacity: 100 box

Used: 72

Available: 28

---

# 28. LAPORAN

Buat report center.

Kategori:

- laporan jumlah arsip
- laporan arsip aktif
- laporan arsip inaktif
- laporan arsip vital
- laporan arsip digital
- laporan arsip fisik
- laporan peminjaman
- laporan keterlambatan
- laporan retensi
- laporan aktivitas
- laporan pengguna

Support:

- filter
- date range
- export PDF
- export Excel
- print

---

# 29. AUDIT LOG

Audit log sangat penting.

Catat:

- login
- logout
- create
- update
- delete
- download
- print
- view sensitive document
- borrow
- return
- move
- permission change

Tampilkan:

timestamp

user

action

resource

IP jika tersedia

result

detail

---

# 30. USER MANAGEMENT

Buat user management enterprise.

Tampilkan:

- nama
- username
- email
- role
- status
- last login

Actions:

- tambah
- edit
- disable
- reset access
- view activity

---

# 31. ROLE & PERMISSION

Jangan hanya gunakan role sederhana.

Buat permission granular.

Contoh:

archive.view

archive.create

archive.update

archive.delete

archive.download

archive.print

archive.borrow

archive.return

archive.move

archive.export

archive.manage_retention

user.manage

role.manage

audit.view

report.view

Permission harus dapat dikontrol berdasarkan role.

---

# 32. SECURITY UX

Tampilkan security state secara jelas.

Untuk tindakan sensitif:

gunakan confirmation.

Contoh:

"Apakah Anda yakin ingin menghapus arsip ini?"

Untuk operasi kritis:

minta konfirmasi tambahan.

Jangan menggunakan confirmation dialog untuk setiap tindakan kecil.

---

# 33. ACCESSIBILITY

Target minimal:

WCAG 2.2 AA.

Pastikan:

- contrast cukup
- keyboard accessible
- focus state jelas
- screen reader friendly
- semantic HTML
- aria label
- tooltip tidak menjadi satu-satunya sumber informasi
- error message jelas
- form label jelas

Jangan hanya mengandalkan warna untuk menunjukkan status.

Contoh:

bukan hanya warna hijau.

Gunakan:

"AKTIF"

dengan badge.

---

# 34. FORM UX

Form arsip harus mudah digunakan.

Kelompokkan:

INFORMASI UTAMA

IDENTITAS ARSIP

KLASIFIKASI

LOKASI

RETENSI

DOKUMEN

AKSES

CATATAN

Jangan membuat satu form panjang tanpa struktur.

Gunakan:

- section
- tabs
- progressive disclosure jika diperlukan

Validasi harus realtime tetapi tidak mengganggu.

Error harus menjelaskan cara memperbaikinya.

---

# 35. EMPTY STATE

Jangan tampilkan halaman kosong.

Contoh:

"Belum ada arsip"

"Mulai dengan menambahkan arsip pertama."

Button:

"Tambah Arsip"

---

# 36. LOADING STATE

Gunakan skeleton loading untuk halaman besar.

Jangan menampilkan blank screen.

Untuk action:

gunakan loading indicator pada button.

Contoh:

"Menyimpan..."

bukan hanya spinner tanpa konteks.

---

# 37. ERROR STATE

Error harus manusiawi.

Jangan menampilkan:

"SQLSTATE 23505"

kepada user biasa.

Tampilkan:

"Gagal menyimpan arsip karena data dengan kode tersebut sudah digunakan."

Sediakan:

- retry
- detail jika diperlukan
- support/debug information untuk administrator

---

# 38. TOAST

Gunakan toast untuk feedback singkat.

Success:

"Arsip berhasil disimpan."

Error:

"Arsip gagal disimpan."

Warning:

"Arsip mendekati akhir retensi."

Jangan membuat toast terlalu lama.

---

# 39. MODAL

Gunakan modal hanya untuk:

- confirmation
- quick action
- focused task

Untuk form kompleks gunakan page atau drawer.

Pastikan:

- modal dapat ditutup
- ESC berfungsi
- focus trap
- mobile friendly

---

# 40. TABLE UX

Table harus menjadi salah satu komponen terbaik dalam aplikasi.

Support:

- sticky header
- horizontal scrolling
- sorting
- filtering
- pagination
- column visibility
- row selection
- bulk action
- responsive mode

Jika layar mobile terlalu sempit, ubah table menjadi list/card yang tetap mempertahankan informasi penting.

---

# 41. PERFORMANCE UX

Jangan membuat interface berat.

Untuk data besar gunakan:

- server-side pagination
- server-side filtering
- lazy loading
- virtualization jika diperlukan
- debounced search
- caching
- optimistic update jika aman

Hindari mengambil ribuan record sekaligus ke browser.

---

# 42. DARK MODE

Sediakan dark mode jika arsitektur aplikasi mendukungnya.

Pastikan dark mode bukan sekadar membalik warna.

Perhatikan:

- contrast
- borders
- status colors
- document preview
- modal
- table
- charts

---

# 43. RESPONSIVE DESIGN RULE

Semua komponen harus memiliki breakpoint yang jelas.

Jangan menggunakan:

"nanti otomatis responsive."

Tentukan behavior masing-masing komponen pada:

Mobile

Tablet

Desktop

Wide desktop

---

# 44. MICRO INTERACTION

Gunakan animasi sangat halus.

Contoh:

- button hover
- dropdown
- modal transition
- sidebar collapse
- toast
- row selection

Durasi pendek.

Jangan menggunakan animasi yang mengganggu pekerjaan.

---

# 45. NAVIGATION UX

Gunakan breadcrumb.

Contoh:

Dashboard

>

Arsip

>

Arsip Aktif

>

Detail Arsip

User harus selalu tahu sedang berada di mana.

---

# 46. QUICK ACTION

Tambahkan quick action.

Contoh:

Floating action atau command menu:

Tambah Arsip

Peminjaman

Upload Dokumen

Scan QR

Buat Laporan

Gunakan hanya jika benar-benar membantu.

---

# 47. NOTIFICATION CENTER

Buat notification drawer.

Tampilkan:

Unread

Today

Earlier

Setiap notification dapat diklik dan membawa user ke sumbernya.

Tambahkan:

Mark as read

Mark all as read

---

# 48. PROFILE MENU

Profile menu:

Nama pengguna

Role

Status

Last login

Profile

Security

Settings

Logout

---

# 49. SETTINGS

Settings harus terstruktur.

Kategori:

General

Archive

Classification

Retention

Location

Notifications

Security

Users

Roles

Audit

System

---

# 50. UX CONSISTENCY

Setiap halaman harus memiliki pola yang sama.

Contoh:

Page header

↓

Toolbar

↓

Filter

↓

Content

↓

Pagination

Jangan membuat setiap halaman memiliki layout berbeda tanpa alasan.

---

# 51. DESIGN TOKENS

Gunakan CSS variables/design tokens.

Contoh konsep:

--color-primary

--color-background

--color-surface

--color-border

--color-text

--color-muted

--spacing-xs

--spacing-sm

--spacing-md

--spacing-lg

--radius-sm

--radius-md

--radius-lg

--shadow-sm

--shadow-md

Semua komponen harus menggunakan token.

---

# 52. COMPONENT ARCHITECTURE

Buat reusable components:

AppShell

Sidebar

Topbar

Search

CommandPalette

Button

Input

Select

DatePicker

Badge

Card

Table

Pagination

Modal

Drawer

Tabs

Dropdown

Toast

Tooltip

Breadcrumb

FileUploader

DocumentViewer

Timeline

EmptyState

ErrorState

Skeleton

FilterPanel

ConfirmDialog

QRScanner

QRGenerator

---

# 53. DESIGN REVIEW

Setelah implementasi:

review seluruh aplikasi.

Periksa:

- spacing
- alignment
- typography
- contrast
- responsive
- consistency
- usability
- accessibility
- empty state
- loading
- error
- permission
- table
- form
- modal
- navigation

Jangan hanya memeriksa halaman dashboard.

---

# 54. UX TESTING

Simulasikan pengguna:

## USER 1 — ARSIPARIS

Tugas:

1. mencari arsip
2. menambah arsip
3. upload dokumen
4. menemukan lokasi fisik
5. meminjamkan arsip
6. mengembalikan arsip
7. mencetak label

Pastikan workflow cepat.

## USER 2 — ADMIN

Tugas:

1. membuat user
2. membuat role
3. mengatur permission
4. melihat audit log
5. mengatur klasifikasi

## USER 3 — PIMPINAN

Tugas:

1. melihat dashboard
2. melihat laporan
3. melihat statistik
4. melihat alert
5. melihat aktivitas

---

# 55. ZERO DEAD END

Jangan membuat user masuk ke halaman yang tidak tahu harus melakukan apa.

Setiap halaman harus memiliki:

- tujuan
- primary action
- feedback
- navigation kembali

---

# 56. ZERO CONFUSION

Gunakan bahasa yang jelas.

Hindari istilah teknis seperti:

SQL

API

UUID

Database ID

Internal ID

kepada user biasa.

Gunakan bahasa bisnis/kearsipan.

---

# 57. DATA INTEGRITY UX

Jika kode arsip duplicate:

Tampilkan:

"Kode arsip sudah digunakan."

Bukan error database mentah.

Jika file terlalu besar:

"Ukuran file melebihi batas yang diizinkan."

Jika permission tidak cukup:

"Anda tidak memiliki izin untuk melakukan tindakan ini."

---

# 58. DELETE UX

Jangan langsung menghapus data penting.

Gunakan:

Soft delete jika sesuai.

Tambahkan confirmation.

Untuk data kritis:

minta user mengetik kode/nama jika diperlukan.

---

# 59. MOBILE-FIRST ACTIONS

Pada mobile, prioritaskan:

Search

View archive

Scan QR

Borrow

Return

Upload

Notifications

Jangan membuat user melakukan terlalu banyak navigasi.

---

# 60. FINAL QUALITY STANDARD

Aplikasi harus memiliki karakter:

"Enterprise-grade"

"Production-ready"

"Professional"

"Fast"

"Accessible"

"Responsive"

"Scalable"

"Secure"

"Maintainable"

"Easy to learn"

"Easy to operate"

---

# 61. ATURAN IMPLEMENTASI

Jika project yang diberikan sudah memiliki kode:

JANGAN menghapus fitur yang sudah berjalan hanya karena ingin mengubah desain.

Pertahankan:

- business logic
- API
- database
- authentication
- authorization
- existing functionality

kecuali memang ditemukan bug atau arsitektur yang harus diperbaiki.

Sebelum mengubah:

1. inspect project
2. pahami architecture
3. pahami routing
4. pahami components
5. pahami API
6. pahami database
7. pahami authentication
8. pahami authorization

Kemudian lakukan perubahan secara terstruktur.

---

# 62. JANGAN MELAKUKAN INI

Jangan:

- membuat dashboard hanya untuk estetika
- menggunakan terlalu banyak card
- menggunakan terlalu banyak warna
- menggunakan font dekoratif
- membuat sidebar terlalu ramai
- membuat table sulit dibaca
- membuat modal untuk semua hal
- membuat animasi berlebihan
- menghilangkan informasi penting
- mengorbankan usability demi estetika
- membuat mobile version sebagai afterthought
- menampilkan error teknis kepada pengguna biasa
- mengubah business logic tanpa alasan
- membuat duplicate component yang sebenarnya bisa reusable

---

# 63. PRIORITAS JIKA HARUS MEMILIH

Jika terdapat konflik antara:

Visual vs usability

pilih usability.

Jika:

Animation vs performance

pilih performance.

Jika:

Decoration vs readability

pilih readability.

Jika:

Fitur baru vs stability

pilih stability.

Jika:

Complexity vs simplicity

pilih simplicity.

---

# 64. TARGET AKHIR

Hasil akhir harus terasa seperti produk enterprise modern yang dapat digunakan oleh:

- arsiparis
- administrator
- pimpinan
- organisasi
- perusahaan
- instansi pemerintah

Pengguna baru harus dapat memahami interface tanpa membutuhkan tutorial panjang.

Pengguna berpengalaman harus dapat bekerja sangat cepat menggunakan:

- search
- filter
- keyboard shortcut
- command palette
- bulk action
- QR scan
- quick action

---

# 65. FINAL AUDIT

Sebelum menyatakan pekerjaan selesai, lakukan audit menyeluruh terhadap:

UI

UX

Responsive

Accessibility

Performance

Security UX

Navigation

Forms

Tables

Search

Dashboard

Archive detail

Physical archive

Digital archive

Retention

Borrowing

Return

QR

Reports

Notifications

Users

Roles

Permissions

Audit log

Settings

Loading states

Empty states

Error states

Confirmation dialogs

Mobile

Tablet

Desktop

Dark mode jika tersedia

---

# 66. OUTPUT YANG DIHARAPKAN

Jangan hanya mengatakan:

"UI sudah diperbaiki."

Berikan hasil pekerjaan nyata.

Tunjukkan:

1. apa yang diperbaiki
2. halaman yang diperbaiki
3. komponen yang dibuat
4. masalah UX yang ditemukan
5. masalah responsive yang ditemukan
6. masalah accessibility yang ditemukan
7. masalah performance yang ditemukan
8. masalah consistency yang ditemukan
9. fitur yang ditambahkan
10. bug yang diperbaiki

Jika menemukan masalah lain selama proses audit, **perbaiki juga jika aman dan masih berada dalam scope aplikasi**.

---

# 67. GOLDEN RULE

Selalu tanyakan pada setiap halaman:

"Apakah arsiparis dapat menyelesaikan pekerjaannya dengan lebih cepat setelah desain ini diterapkan?"

Jika jawabannya tidak:

PERBAIKI.

Jangan mengejar desain yang hanya terlihat modern.

Bangun aplikasi yang:

**terlihat modern + terasa profesional + sangat cepat digunakan + mudah dipahami + kuat untuk data besar + nyaman di desktop dan mobile + aman + accessible + scalable.**

Target akhir:

# WORLD-CLASS DIGITAL ARCHIVE MANAGEMENT SYSTEM

Bukan sekadar dashboard CRUD.

Buat seluruh pengalaman aplikasi terasa sebagai satu produk yang matang, konsisten, profesional, dan siap digunakan dalam lingkungan enterprise.