// Command sync performs one database synchronisation pass between this
// computer's local database and the configured hub database.
//
// It is the same engine the running server uses (see the "Sinkronkan" button
// and the background scheduler); the CLI is handy for cron jobs or for running
// a manual sync from the desktop launcher.
//
// Usage (from the project root, where .env lives):
//
//	go run ./cmd/sync
//
// Required .env keys: SYNC_HUB_DB_HOST, SYNC_HUB_DB_NAME, SYNC_HUB_DB_USER
// (plus SYNC_HUB_DB_PASS / SYNC_HUB_DB_PORT / SYNC_HUB_DB_TLS as needed).
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"arsippro/internal/config"
	"arsippro/internal/syncdb"
)

func main() {
	config.Load()
	cfg := config.LoadSync()

	if !cfg.HubReady() {
		fmt.Fprintln(os.Stderr, "Hub belum dikonfigurasi.")
		fmt.Fprintln(os.Stderr, "Isi SYNC_HUB_DB_HOST, SYNC_HUB_DB_NAME, dan SYNC_HUB_DB_USER di .env,")
		fmt.Fprintln(os.Stderr, "lalu jalankan lagi. Contoh:")
		fmt.Fprintln(os.Stderr, "  SYNC_HUB_DB_HOST=192.168.1.86")
		fmt.Fprintln(os.Stderr, "  SYNC_HUB_DB_NAME=simarc")
		fmt.Fprintln(os.Stderr, "  SYNC_HUB_DB_USER=chris")
		os.Exit(2)
	}

	log.Printf("[sync] simpul=%s hub=%s/%s", cfg.NodeID, cfg.HubDBHost, cfg.HubDBName)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	eng, err := syncdb.Open(ctx, cfg, log.Printf)
	if err != nil {
		fmt.Fprintf(os.Stderr, "gagal membuka koneksi: %v\n", err)
		os.Exit(1)
	}
	defer eng.Close()

	res, err := eng.Run(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sinkronisasi gagal: %v\n", err)
		os.Exit(1)
	}

	// Archive files (storage/app/uploads) travel over the hub's HTTP endpoints.
	var files syncdb.FileSyncResult
	if cfg.FilesActive() {
		files, err = syncdb.SyncFiles(ctx, cfg, log.Printf)
		if err != nil {
			log.Printf("[sync] berkas dilewati: %v", err)
		}
	}

	fmt.Printf("Selesai (%s) dalam %s\n", res.Status(), res.FinishedAt.Sub(res.StartedAt).Round(time.Millisecond))
	fmt.Printf("  didorong ke hub : %d baris\n", res.Pushed)
	fmt.Printf("  ditarik dari hub: %d baris\n", res.Pulled)
	fmt.Printf("  tabel gagal     : %d\n", res.Errors)
	if cfg.FilesActive() {
		fmt.Printf("  berkas didorong : %d (%.1f MB)\n", files.Pushed, float64(files.BytesOut)/1048576)
		fmt.Printf("  berkas ditarik  : %d (%.1f MB)\n", files.Pulled, float64(files.BytesIn)/1048576)
	}
	for _, t := range res.Tables {
		if t.Err != "" {
			fmt.Printf("    - %s (%s): %s\n", t.Name, t.Strategy, t.Err)
		}
	}
	if res.Errors > 0 {
		os.Exit(1)
	}
}
