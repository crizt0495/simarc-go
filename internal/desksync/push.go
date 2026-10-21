package desksync

import (
	"errors"
	"log"
	"os"
	"reflect"
	"time"

	"arsippro/internal/database"
	"arsippro/internal/models"

	"gorm.io/gorm"
)

// ── PUSH: kirim perubahan lokal → pusat ──────────────────────────────────────

// pushAll mendorong seluruh tabel syncable ke pusat (induk dulu).
// Return (jumlah baris terkirim, error pertama non-fatal per tabel tidak
// menghentikan tabel berikutnya).
func (e *Engine) pushAll(local, remote *gorm.DB, cycleStart time.Time) (int, error) {
	total := 0
	var firstErr error
	for _, spec := range specs() {
		n, err := e.pushTable(local, remote, spec, cycleStart)
		total += n
		if err != nil {
			log.Printf("[SYNC][push %s] %v", spec.Table, err)
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return total, firstErr
}

// pushTable mengirim baris lokal milik tabel spec yang berubah sejak
// watermark terakhir (outbox + rescan updated_at / deleted_at) ke pusat.
func (e *Engine) pushTable(local, remote *gorm.DB, spec TableSpec, cycleStart time.Time) (int, error) {
	st := database.GetSyncState(spec.Table)
	wm := st.LastPushAt
	// Baris seed bootstrap tidak akan pernah di-push: rescan hanya menyapu
	// perubahan nyata setelah jejak bootstrap.
	if database.LocalBootstrapAt().After(wm) {
		wm = database.LocalBootstrapAt()
	}

	// 1) Rescan watermark: perubahan timestamp-based (create/update).
	rowsPtr := newSlice(spec.Model)
	q := local.Unscoped().Where(spec.TSCol+" > ?", wm)
	if spec.Soft {
		q = local.Unscoped().Where(spec.TSCol+" > ? OR deleted_at > ?", wm, wm)
	}
	if err := q.Model(spec.Model).Find(rowsPtr).Error; err != nil {
		return 0, err
	}

	sent := 0
	sentKeys := map[string]bool{}
	failedKeys := map[string]bool{}
	slice := reflect.ValueOf(rowsPtr).Elem()
	for i := 0; i < slice.Len(); i++ {
		rowPtr := slice.Index(i).Addr().Interface()
		key := fieldStrOf(reflect.ValueOf(rowPtr), spec.KeyProp)
		did, err := e.pushRow(local, remote, spec, rowPtr)
		if err != nil {
			log.Printf("[SYNC][push %s] baris %s dilewati: %v", spec.Table, key, err)
			failedKeys[key] = true
			continue
		}
		if did {
			sent++
			sentKeys[key] = true
		}
	}

	// 2) Union outbox: baris yang dicatat hook (termasuk delete tombstone,
	//    yang tidak bisa disapu scan timestamp).
	var obKeys []string
	if err := local.Model(&models.SyncOutbox{}).Where("table_name = ? AND deleted_at IS NULL", spec.Table).
		Distinct().Pluck("row_key", &obKeys).Error; err == nil {
		for _, key := range obKeys {
			if sentKeys[key] || failedKeys[key] {
				continue
			}
			rowPtr := newSample(spec.Model)
			if err := local.Unscoped().Where(spec.KeyCol+" = ?", key).Take(rowPtr).Error; err != nil {
				// Baris sudah tak ada (hard delete?) → tandai tertangani.
				sentKeys[key] = true
				continue
			}
			did, err := e.pushRow(local, remote, spec, rowPtr)
			if err != nil {
				log.Printf("[SYNC][push %s] baris outbox %s dilewati: %v", spec.Table, key, err)
				failedKeys[key] = true
				continue
			}
			if did {
				sent++
			}
			sentKeys[key] = true
		}
	}

	database.OutboxClearFor(spec.Table, failedKeys)
	database.SaveSyncState(spec.Table, &cycleStart, nil)
	return sent, nil
}

// pushRow mengirim satu baris: LWW dulu (remote lebih baru → lewati),
// kemudian replace baris pusat (hapus hard + insert) agar timestamp lokal
// ikut terbawa apabila menang.
func (e *Engine) pushRow(local, remote *gorm.DB, spec TableSpec, rowPtr any) (bool, error) {
	rv := reflect.ValueOf(rowPtr).Elem()
	key := fieldStrOf(rv, spec.KeyProp)
	if key == "" {
		return false, nil
	}

	// Khusus arsip: remap FK jenis_arsip (local → remote) & kemas file.
	if spec.Table == "arsip" {
		if err := e.packArsipBeforePush(local, remote, rowPtr.(*models.Arsip)); err != nil {
			return false, err
		}
	}

	if spec.StripPK {
		return e.pushRowWithAutoPK(local, remote, spec, rowPtr, key)
	}

	var existValue = reflect.New(elemType(spec.Model)).Interface()
	err := remote.Unscoped().Where(spec.KeyCol+" = ?", key).Take(existValue).Error
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return false, err
		}
		return true, remote.Create(rowPtr).Error
	}
	lts := timeOf(rowPtr, spec.TSProp)
	rts := timeOf(existValue, spec.TSProp)
	lDelT, lDel := deletedAtOf(rowPtr)
	rDelT, rDel := deletedAtOf(existValue)
	if !shouldWriteRowRemote(spec, lts, rts, lDel, lDelT, rDel, rDelT) {
		return false, nil // pusat lebih baru / setara / tombstone pusat lebih baru
	}
	// Ganti baris pusat: hapus (hard), masukkan versi lokal (timestamp pasang).
	if err := remote.Unscoped().Where(spec.KeyCol+" = ?", key).Delete(newSample(spec.Model)).Error; err != nil {
		return false, err
	}
	return true, remote.Create(rowPtr).Error
}

// pushRowWithAutoPK untuk tabel ber-PK auto-increment (jenis_arsip & item):
// identitas memakai uuid; baris pusat binasa bila ada, else insert baru lalu
// catat peta local_id ↔ remote_id.
func (e *Engine) pushRowWithAutoPK(local, remote *gorm.DB, spec TableSpec, rowPtr any, key string) (bool, error) {
	exist := newSample(spec.Model)
	err := remote.Unscoped().Where(spec.KeyCol+" = ?", key).Take(exist).Error
	if err == nil {
		lts := timeOf(rowPtr, spec.TSProp)
		rts := timeOf(exist, spec.TSProp)
		lDelT, lDel := deletedAtOf(rowPtr)
		rDelT, rDel := deletedAtOf(exist)
		if !shouldWriteRowRemote(spec, lts, rts, lDel, lDelT, rDel, rDelT) {
			// Pusat setara/lebih baru — pastikan peta pada lokal tetap direkam.
			e.recordLocalIDMap(local, spec, exist, rowPtr)
			return false, nil
		}
		// Ganti isi baris pusat dengan versi lokal, MEMPERTAHANKAN id pusat.
		copyFieldsPreservingID(exist, rowPtr)
		if err := remote.Save(exist).Error; err != nil {
			return false, err
		}
		e.recordLocalIDMap(local, spec, exist, rowPtr)
		return true, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return false, err
	}
	remoteCopy := newSample(spec.Model)
	copyAllFields(remoteCopy, rowPtr)
	if err := remote.Omit("id").Create(remoteCopy).Error; err != nil {
		return false, err
	}
	e.recordLocalIDMap(local, spec, remoteCopy, rowPtr)
	return true, nil
}

// recordLocalIDMap merekam peta local↔remote id (untuk FK jenis_arsip).
func (e *Engine) recordLocalIDMap(local *gorm.DB, spec TableSpec, remoteRowIPv, localRow any) {
	localID := uint(fieldUintOf(reflect.ValueOf(localRow), "ID"))
	remoteID := uint(fieldUintOf(reflect.ValueOf(remoteRowIPv), "ID"))
	if localID == 0 || remoteID == 0 {
		return
	}
	val := fieldStrOf(reflect.ValueOf(localRow), "UUID")
	var m models.SyncIDMap
	if err := local.Where("table_name = ? AND local_id = ? AND remote_id = ?", spec.Table, localID, remoteID).First(&m).Error; err == nil {
		return
	}
	local.Create(&models.SyncIDMap{Table: spec.Table, LocalID: localID, RemoteID: remoteID, UUID: val})
}

// packArsipBeforePush menyiapkan baris arsip sebelum dikirim pusat:
//   - remap FK jenis_arsip dari id lokal ke id pusat (via peta);
//   - isi FileContent dari disk bila kosong (agar lampiran ikut).
func (e *Engine) packArsipBeforePush(local, remote *gorm.DB, row *models.Arsip) error {
	if row.JenisArsipID != nil && *row.JenisArsipID > 0 {
		rid, err := e.ensureRemoteJenisByLocalID(local, remote, *row.JenisArsipID)
		if err != nil {
			return err
		}
		row.JenisArsipID = &rid
	}
	if len(row.FileContent) == 0 && row.FilePath != "" {
		if b, err := os.ReadFile(ResolveUploadPath(row.FilePath)); err == nil && int64(len(b)) <= e.FileMax {
			row.FileContent = b
		}
	}
	return nil
}

// ensureRemoteJenisByLocalID — baris jenis_arsip dengan id lokal diberikan.
func (e *Engine) ensureRemoteJenisByLocalID(local, remote *gorm.DB, localID uint) (uint, error) {
	var m models.SyncIDMap
	if err := local.Where("table_name = ? AND local_id = ?", "jenis_arsip", localID).First(&m).Error; err == nil {
		return m.RemoteID, nil
	}
	var jenis models.JenisArsip
	if err := local.First(&jenis, localID).Error; err != nil {
		return 0, err
	}
	var cur models.JenisArsip
	if err := remote.Where("uuid = ?", jenis.UUID).First(&cur).Error; err == nil {
		upsertIDMap(local, "jenis_arsip", jenis.ID, cur.ID, jenis.UUID)
		return cur.ID, nil
	}
	cop := jenis
	cop.ID = 0
	if err := remote.Omit("id").Create(&cop).Error; err != nil {
		return 0, err
	}
	upsertIDMap(local, "jenis_arsip", jenis.ID, cop.ID, jenis.UUID)
	return cop.ID, nil
}

// upsertIDMap menulis/memastikan peta id lintas-DB (upsert sederhana).
func upsertIDMap(local *gorm.DB, table string, localID, remoteID uint, uuidv string) {
	var m models.SyncIDMap
	if err := local.Where("table_name = ? AND local_id = ? AND remote_id = ?", table, localID, remoteID).First(&m).Error; err == nil {
		return
	}
	local.Create(&models.SyncIDMap{Table: table, LocalID: localID, RemoteID: remoteID, UUID: uuidv})
}
