# IMPLEMENTATION PLAN — SIMARC Offline-First Desktop

## Target
Aplikasi desktop Golang (Wails + React/TS) dengan:
- MySQL lokal per klien (offline-first, selalu bekerja)
- Sinkronisasi dua arah otomatis ke server pusat
- Tahan terhadap server mati / LAN terputus / internet mati
- Tidak kehilangan data / tidak duplikasi
- Windows + Linux

## Langkah 1: Analisis Repo (SELESAI)
Repo `simarc-go` sudah ada dengan struktur `cmd/`, `internal/`, `packaging/`. Ada dokumen `OFFLINE-SYNC.md`, `MULTI-KOMPUTER.md`, `DESKTOP-UI.md`.

## Langkah 2: Struktur Direktori (DALAM PROSES)
Buat:
```
desktop/          → Wails frontend (React + TS)
server/           → Gin REST API + MySQL pusat
cmd/desktop/      → Entry desktop
cmd/server/       → Entry server
internal/
  app/            → Business logic
  database/       → Local + Central DB init, connection pool
  sync/           → Push/pull engine, queue, conflict, cursor
  models/         → UUID v7 entities
  auth/           → JWT + Argon2id
  config/         → Env/config loader
migrations/
  central/        → Server schema
  local/          → Client local schema
pkg/
  syncprotocol/   → Batch, idempotency, cursor protocol
```

## Langkah 3: Database Design (DALAM PROSES)
- InnoDB, utf8mb4, UUID PK, DECIMAL(19,4), DATETIME(3), FK, index
- Soft delete (`deleted_at`), versioning (`version` / `row_version`)
- Tabel sistem: devices, sync_queue, sync_batches, sync_inbox, sync_cursors, sync_conflicts, change_log, sync_logs, audit_logs, schema_versions
- Tabel bisnis: arsip, pemberkasan, pengguna, perangkat, dll.

## Langkah 4: Local Engine (DALAM PROSES)
- MySQL 8.4 lokal, listen 127.0.0.1:3306
- Auto migration (`golang-migrate`)
- Connection pool (`sql.DB`)
- Health check (`SELECT 1`)
- Backup/restore (mysqldump / file copy)
- Semua transaksi lokal dulu; sync_queue ditulis dalam transaksi yang sama (transactional outbox)

## Langkah 5: Central Server (DALAM PROSES)
- Gin REST API (port 8080)
- MySQL pusat (port 3306, tidak dibuka publik)
- Endpoints: auth, device/register, sync/status, sync/push, sync/pull, sync/ack, bootstrap, schema/version, health
- JWT middleware, rate limit, RBAC, audit log
- Idempotency (operation_id), sequence server

## Langkah 6: Sync Engine (DALAM PROSES)
- PUSH: baca sync_queue → batch → REST → server ACK → tandai sukses
- PULL: ambil change_log berdasarkan cursor → terapkan lokal → simpan cursor atomik
- Retry: exponential backoff + jitter
- Crash recovery: redrive dari sync_queue
- Conflict: tombstone / optimistic lock / append-only / UUID-merge sesuai jenis data
- Tidak pakai timestamp sebagai satu-satunya cursor

## Langkah 7: Desktop UI (Wails) (DALAM PROSES)
- React + TypeScript
- Dashboard, login, modul bisnis, pengaturan, status DB lokal, status server, status sync, antrean, konflik, backup/restore
- Responsive, modern (Fluent Design), full-screen option

## Langkah 8: Installer (DALAM PROSES)
- Windows: EXE/MSI + MySQL lokal otomatis + provisioning
- Linux: AppImage / DEB + MySQL lokal otomatis
- Shutdown bersih, uninstall bersih (data tetap di storage/)

## Langkah 9: Testing (MENUNGGU)
- Server mati, LAN mati, internet mati, restart, duplicate, concurrent, push/pull fail, conflict, recovery, backup, Windows/Linux build

## Langkah 10: Dokumentasi & Build (MENUNGGU)
- `go doc`, `go test`, `go build`, `wails build`
- `README.md`, `docs/`, diagram arsitektur
