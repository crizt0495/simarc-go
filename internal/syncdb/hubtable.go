package syncdb

import (
	"context"
	"database/sql"
	"fmt"
	"sync"

	"arsippro/internal/models"

	sqlitedrv "github.com/glebarez/sqlite"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// hubTools menyimpan koneksi GORM ke hub dan hasil migrasi skema hub. Dipisah
// dari Engine agar berkas engine.go tidak perlu mengimpor GORM.
type hubTools struct {
	dsn      string
	driver   string
	gormDB   *gorm.DB
	gormOnce sync.Once
	gormErr  error

	schemaOnce   sync.Once
	schemaErr    error
	schemaReady  bool
	schemaFailed bool
}

// ensureMissingHubTables membuat tabel yang siap disinkronkan tapi belum ada di
// hub, sebelum pembatasan kolom (restrictTablesToHub) berjalan. Tanpa tahap ini
// hub yang baru (belum punya tabel apa pun) membuat semua tabel dilewati.
//
//   - dialek sama (MySQL->MySQL atau SQLite->SQLite): DDL disalin apa adanya
//     sehingga kolom persis sama dengan sisi lokal.
//   - dialek berbeda (SQLite lokal -> MySQL hub): DDL teks tidak bisa disalin
//     mentah (tipe kolom berbeda), jadi skema dibuat dari model GORM per tabel.
//
// Tabel tanpa model GORM yang dikenal dibiarkan hilang; dilewati dengan log.
func ensureMissingHubTables(ctx context.Context, local *sql.DB, localDriver string, hub *sql.DB, hubDriver, hubDSN string, tables map[string]*tableInfo, logf func(string, ...any)) error {
	hubTables, err := loadTables(ctx, hub, hubDriver)
	if err != nil {
		return err
	}
	var missing []string
	for name := range tables {
		if _, ok := hubTables[name]; !ok {
			missing = append(missing, name)
		}
	}
	if len(missing) == 0 {
		return nil
	}

	created := 0
	var gdb *gorm.DB
	var gormErr error
	for _, name := range missing {
		if localDriver == hubDriver {
			err := createTableLike(ctx, local, localDriver, hub, hubDriver, name)
			if err != nil {
				if logf != nil {
					logf("[sync] gagal salin DDL tabel %s ke hub: %v", name, err)
				}
				continue
			}
		} else {
			// Dialek berbeda (SQLite lokal -> MySQL hub atau sebaliknya).
			// Buat skema hub dari model GORM sekali saja (semua tabel), agar
			// foreign key antar tabel dibuat benar (butuh referensi kedua sisi).
			if gdb == nil && gormErr == nil {
				gdb, gormErr = hubGormDBFor(hubDriver, hubDSN)
			}
			if gormErr != nil {
				if logf != nil {
					logf("[sync] koneksi GORM hub gagal: %v", gormErr)
				}
				continue
			}
			if err := gdb.WithContext(ctx).AutoMigrate(models.AllModels()...); err != nil {
				if logf != nil {
					logf("[sync] migrasi skema hub: %v", err)
				}
				continue
			}
			// AutoMigrate semua model membuat tabel yang hilang + yang sudah ada
			// (idempoten). Tandai semua missing sebagai dibuat.
			for i := range missing {
				created++
				if logf != nil {
					logf("[sync] tabel %s dibuat di hub (via AutoMigrate)", missing[i])
				}
			}
			break
		}
		created++
		if logf != nil {
			logf("[sync] tabel %s dibuat di hub", name)
		}
	}
	if created == 0 {
		return fmt.Errorf("%d tabel hilang di hub dan semua gagal dibuat", len(missing))
	}
	return nil
}

// hubGormDBFor membuka koneksi GORM ke hub sesuai dialeknya (MySQL atau
// SQLite). Dipakai untuk membuat/memperbaiki skema hub.
func hubGormDBFor(driver, dsn string) (*gorm.DB, error) {
	var dialector gorm.Dialector
	if driver == driverSQLite {
		dialector = sqlitedrv.Open(dsn)
	} else {
		dialector = mysql.Open(dsn)
	}
	return gorm.Open(dialector, &gorm.Config{
		Logger: logger.Default.LogMode(logger.Error),
	})
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
		return createTableLike(ctx, e.local, e.localDriver, e.hub, e.hubDriver, table)
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
		db, err := hubGormDBFor(e.driver, e.dsn)
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

// hubTableExists menanyakan keberadaan tabel di hub, dengan cache per-run.
// Hasil salah tidak di-cache selamanya: setelah ensureHubTable berhasil,
// pemanggil diharuskan memvalidasi kembali (telah ditangani di bawah).
func (e *Engine) hubTableExists(ctx context.Context, table string) (bool, error) {
	if e.hubExists == nil {
		return tableExistsIn(ctx, e.hub, e.hubDriver, table)
	}
	if v, ok := e.hubExists[table]; ok {
		return v, nil
	}
	v, err := tableExistsIn(ctx, e.hub, e.hubDriver, table)
	if err != nil {
		return false, err
	}
	e.hubExists[table] = v
	return v, nil
}

// hubShape mengembalikan bentuk tabel hub (kolom + PK) dengan cache per-run.
func (e *Engine) hubShape(ctx context.Context, table string) (*tableInfo, error) {
	if e.hubShapes != nil {
		if ti, ok := e.hubShapes[table]; ok {
			return ti, nil
		}
	}
	ti, err := loadTableFor(ctx, e.hub, e.hubDriver, table)
	if err != nil {
		return nil, err
	}
	if e.hubShapes == nil {
		e.hubShapes = map[string]*tableInfo{}
	}
	e.hubShapes[table] = ti
	return ti, nil
}

// invalidateHubCache dipanggil setelah ensureHubTable berhasil membuat tabel:
// keberadaan kini benar (true), sedangkan bentuknya harus dibaca ulang bila
// nanti diminta.
func (e *Engine) invalidateHubCache(table string) {
	if e.hubExists != nil {
		e.hubExists[table] = true
	}
	delete(e.hubShapes, table)
}
