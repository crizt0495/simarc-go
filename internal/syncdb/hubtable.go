package syncdb

import (
	"context"
	"fmt"
	"sync"

	"arsippro/internal/models"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// hubTools menyimpan koneksi GORM ke hub dan hasil migrasi skema hub. Dipisah
// dari Engine agar berkas engine.go tidak perlu mengimpor GORM.
type hubTools struct {
	dsn      string
	gormDB   *gorm.DB
	gormOnce sync.Once
	gormErr  error

	schemaOnce   sync.Once
	schemaErr    error
	schemaReady  bool
	schemaFailed bool
}

// ensureHubTable membuat tabel di hub bila belum ada.
//
// Dua jalur:
//   - dialek sama (MySQL->MySQL atau SQLite->SQLite): DDL disalin apa adanya
//     sehingga kolom persis sama dengan sisi lokal.
//   - dialek berbeda (SQLite lokal -> MySQL hub): DDL teks tidak bisa
//     disalin mentah (tipe kolom MySQL tidak pernah sama dengan SQLite), jadi
//     skema dibuat ulang dari model GORM. Hasilnya sama dengan instalasi
//     MySQL: id char(36), FK, indeks, dan setelan InnoDB.
func ensureHubTable(ctx context.Context, e *Engine, table string) error {
	if e.localDriver == e.hubDriver {
		return createTableLike(ctx, e.local, e.hub, table)
	}

	model := models.ForTable(table)
	if model == nil {
		return fmt.Errorf("tabel %s belum ada di hub dan tidak punya model GORM yang dikenal", table)
	}
	return e.migrateHubSchema(ctx)
}

// migrateHubSchema menjalankan AutoMigrate terhadap hub memakai seluruh model
// aplikasi. Dipanggil satu kali per engine, hanya saat ada tabel yang hilang.
func (e *Engine) migrateHubSchema(ctx context.Context) error {
	e.schemaOnce.Do(func() {
		gdb, err := e.hubGormDB()
		if err != nil {
			e.schemaErr = err
			e.schemaFailed = true
			return
		}
		if err := gdb.WithContext(ctx).AutoMigrate(models.AllModels()...); err != nil {
			e.schemaErr = err
			e.schemaFailed = true
			return
		}
		e.schemaReady = true
	})
	if e.schemaErr != nil {
		return e.schemaErr
	}
	if !e.schemaReady {
		return fmt.Errorf("migrasi skema hub gagal")
	}
	return nil
}

// hubGormDB membuka koneksi GORM ke hub (dialek mengikuti hubDriver) untuk
// keperluan pembuatan tabel.
func (e *Engine) hubGormDB() (*gorm.DB, error) {
	e.gormOnce.Do(func() {
		dialector := gorm.Dialector(mysql.Open(e.dsn))
		db, err := gorm.Open(dialector, &gorm.Config{
			Logger: logger.Default.LogMode(logger.Error),
		})
		if err != nil {
			e.gormErr = err
			return
		}
		e.gormDB = db
	})
	return e.gormDB, e.gormErr
}

// closeHubGorm menutup koneksi GORM hub bila pernah dibuat.
func (e *Engine) closeHubGorm() {
	if e.gormDB != nil {
		if sqlDB, err := e.gormDB.DB(); err == nil {
			sqlDB.Close()
		}
		e.gormDB = nil
	}
}
