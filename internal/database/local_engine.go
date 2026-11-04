package database

import (
	"database/sql"
	"fmt"

	_ "github.com/go-sql-driver/mysql"
)

// LocalEngine manages MySQL 8.4 local database per desktop client.
// All business transactions happen here first (offline-first).
type LocalEngine struct {
	db  *sql.DB
	dsn string
}

func NewLocalEngine(dsn string) (*LocalEngine, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("local engine open: %w", err)
	}
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(0)
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("local db ping failed: %w", err)
	}
	return &LocalEngine{db: db, dsn: dsn}, nil
}

func (e *LocalEngine) DB() *sql.DB { return e.db }

func (e *LocalEngine) HealthCheck() error {
	var one int
	return e.db.QueryRow("SELECT 1").Scan(&one)
}

func (e *LocalEngine) Close() error {
	if e.db != nil {
		return e.db.Close()
	}
	return nil
}

func (e *LocalEngine) Migrate(dir string) error { return nil }
func (e *LocalEngine) Backup(path string) error { return nil }
func (e *LocalEngine) Restore(path string) error { return nil }
