package database

import (
	"path/filepath"
	"testing"

	"arsippro/internal/config"
	"arsippro/internal/models"
)

// TestSQLiteMigrateCreatesAllModelTables memastikan mode SQLite membuat tabel
// untuk seluruh model aplikasi. Kalau ada model yang terlewat, tabelnya tidak
// akan pernah ada di node SQLite — dan karena itu data tabel tersebut tidak
// pernah ikut sinkronisasi ke hub.
func TestSQLiteMigrateCreatesAllModelTables(t *testing.T) {
	oldDriver, oldPath := config.App.DBDriver, config.App.DBPath
	t.Cleanup(func() {
		config.App.DBDriver, config.App.DBPath = oldDriver, oldPath
		if DB != nil {
			if sqlDB, err := DB.DB(); err == nil {
				sqlDB.Close()
			}
			DB = nil
		}
	})

	config.App.DBDriver = "sqlite"
	config.App.DBPath = filepath.Join(t.TempDir(), "simarc-test.db")

	if err := Connect(); err != nil {
		t.Fatalf("buka sqlite: %v", err)
	}
	if err := Migrate(); err != nil {
		t.Fatalf("migrasi sqlite: %v", err)
	}

	for _, m := range models.AllModels() {
		table := models.TableNameOf(m)
		if table == "" {
			t.Fatalf("model %T tidak punya nama tabel", m)
		}
		if !DB.Migrator().HasTable(table) {
			t.Errorf("tabel %s (model %T) tidak dibuat di SQLite", table, m)
		}
	}

	// Tabel penghubung pemusnahan dibuat manual (bukan model GORM).
	if !DB.Migrator().HasTable("pemusnahan_arsip_items") {
		t.Error("tabel pemusnahan_arsip_items tidak dibuat di SQLite")
	}
}

// TestModelRegistryCoversTables memastikan setiap tabel yang dipakai sinkronisasi
// punya entri di registry model, sehingga tabel yang hilang di hub bisa dibuat
// ulang dengan skema MySQL yang benar.
func TestModelRegistryCoversTables(t *testing.T) {
	for _, name := range []string{"arsip", "users", "unit_kerja", "kode_klasifikasi", "pemberkasan"} {
		if models.ForTable(name) == nil {
			t.Errorf("registry model tidak memuat tabel %q", name)
		}
	}
	if models.ForTable("tabel_tidak_ada") != nil {
		t.Error("registry model memuat tabel yang tidak dikenal")
	}
}
