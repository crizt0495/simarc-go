package syncdb

import (
	"context"
	"fmt"
	"time"

	"arsippro/internal/config"
)

// HubHasUsers reports whether the configured hub database already contains user
// rows. A freshly installed computer uses this to avoid seeding its own baseline
// (default roles, admin user, default location): pushing those rows to an
// already-populated hub would duplicate them, and every other computer would
// then pull the duplicates. When the hub is populated the new node pulls the
// hub's baseline instead (see internal/app.Init).
func HubHasUsers(ctx context.Context, cfg config.SyncConfig) (bool, error) {
	if !cfg.HubReady() {
		return false, fmt.Errorf("hub database belum dikonfigurasi (SYNC_HUB_DB_HOST/NAME/USER)")
	}
	db, err := openPool(dsn(cfg.HubDBHost, cfg.HubDBPort, cfg.HubDBName, cfg.HubDBUser, cfg.HubDBPass, cfg.HubDBTLS))
	if err != nil {
		return false, err
	}
	defer db.Close()

	cctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	var n int64
	if err := db.QueryRowContext(cctx, "SELECT COUNT(1) FROM `users`").Scan(&n); err != nil {
		return false, err
	}
	return n > 0, nil
}
