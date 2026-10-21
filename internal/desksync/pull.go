package desksync

import (
	"errors"
	"log"
	"reflect"
	"time"

	"arsippro/internal/database"
	"arsippro/internal/models"

	"gorm.io/gorm"
)

// ── PULL: tarik perubahan pusat → lokal (LWW) ────────────────────────────────

// pullAll menarik seluruh tabel syncable dari pusat (induk dulu).
func (e *Engine) pullAll(local, remote *gorm.DB, cycleStart time.Time) (int, error) {
	total := 0
	var firstErr error

	// Join table permission_role ditangani khusus (ganti penuh dari pusat).
	if n, err := e.pullPermissionRole(local, remote); err != nil {
		log.Printf("[SYNC][pull permission_role] %v", err)
		if firstErr == nil {
			firstErr = err
		}
	} else {
		total += n
	}

	for _, spec := range specs() {
		n, err := e.pullTable(local, remote, spec, cycleStart)
		total += n
		if err != nil {
			log.Printf("[SYNC][pull %s] %v", spec.Table, err)
			if firstErr == nil {
				firstErr = err
			}
		}
	}

	// Tandai watermark pull per tabel (dilakukan setelah seluruh tabel
	// ditarik agar baris timestamp ≈ cycleStart tetap masuk).
	for _, spec := range specs() {
		database.SaveSyncState(spec.Table, nil, &cycleStart)
	}
	return total, firstErr
}

// pullTable menarik baris pusat yang berubah (ts > watermark lalu deleted_at)
// dan menerapkan LWW di lokal.
func (e *Engine) pullTable(local, remote *gorm.DB, spec TableSpec, cycleStart time.Time) (int, error) {
	st := database.GetSyncState(spec.Table)
	wm := st.LastPullAt

	rowsPtr := newSlice(spec.Model)
	var q *gorm.DB
	if spec.Soft {
		q = remote.Unscoped().Where(spec.TSCol+" > ? OR deleted_at > ?", wm, wm)
	} else {
		q = remote.Unscoped().Where(spec.TSCol+" > ?", wm)
	}
	if err := q.Model(spec.Model).Find(rowsPtr).Error; err != nil {
		return 0, err
	}

	applied := 0
	slice := reflect.ValueOf(rowsPtr).Elem()
	for i := 0; i < slice.Len(); i++ {
		rowPtr := slice.Index(i).Addr().Interface()
		did, err := e.pullRow(local, remote, spec, rowPtr)
		if err != nil {
			log.Printf("[SYNC][pull %s] baris dilewati: %v", spec.Table, err)
			continue
		}
		if did {
			applied++
		}
	}
	return applied, nil
}

// pullRow menerapkan satu baris pusat ke lokal — LWW: pusat lebih baru menang;
// lokal lebih baru pendapatan (hanya di-bypass saat sama timestamp → local).
func (e *Engine) pullRow(local, remote *gorm.DB, spec TableSpec, rowPtr any) (bool, error) {
	rv := reflect.ValueOf(rowPtr)
	key := fieldStrOf(rv, spec.KeyProp)
	if key == "" {
		return false, nil
	}

	if spec.StripPK {
		return e.pullRowWithAutoPK(local, spec, rowPtr, key)
	}

	localSample := newSample(spec.Model)
	err := local.Unscoped().Where(spec.KeyCol+" = ?", key).Take(localSample).Error
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return false, err
		}
		// Baris baru di pusat → tarik lokal.
		if err := e.insertRowLocal(local, remote, spec, rowPtr); err != nil {
			return false, err
		}
		e.postProcess(local, spec, rowPtr)
		return true, nil
	}
	lts := timeOf(localSample, spec.TSProp)
	rts := timeOf(rowPtr, spec.TSProp)
	lDelT, lDel := deletedAtOf(localSample)
	rDelT, rDel := deletedAtOf(rowPtr)
	if !shouldApplyRowRemote(spec, lts, rts, lDel, lDelT, rDel, rDelT) {
		return false, nil // lokal lebih baru / setara / tombstone lokal lebih baru
	}
	// replace lokal: hapus (hard) + insert versi pusat.
	if err := local.Unscoped().Where(spec.KeyCol+" = ?", key).Delete(newSample(spec.Model)).Error; err != nil {
		return false, err
	}
	if err := e.insertRowLocal(local, remote, spec, rowPtr); err != nil {
		return false, err
	}
	e.postProcess(local, spec, rowPtr)
	return true, nil
}

// pullRowWithAutoPK untuk tabel ber-PK auto-increment via uuid:
// bila sudah ada baris uuid sama di lokal → update field (id lokal tetap);
// bila belum → insert baru (id lokal auto).
func (e *Engine) pullRowWithAutoPK(local *gorm.DB, spec TableSpec, rowPtr any, key string) (bool, error) {
	localSample := newSample(spec.Model)
	err := local.Unscoped().Where(spec.KeyCol+" = ?", key).Take(localSample).Error
	if err == nil {
		lts := timeOf(localSample, spec.TSProp)
		rts := timeOf(rowPtr, spec.TSProp)
		lDelT, lDel := deletedAtOf(localSample)
		rDelT, rDel := deletedAtOf(rowPtr)
		if !shouldApplyRowRemote(spec, lts, rts, lDel, lDelT, rDel, rDelT) {
			// tetap catat peta bila tabel jenis (FK penting)
			if spec.Table == "jenis_arsip" {
				localID := uint(fieldUintOf(reflect.ValueOf(localSample), "ID"))
				remoteID := uint(fieldUintOf(reflect.ValueOf(rowPtr), "ID"))
				upsertIDMap(local, spec.Table, localID, remoteID, key)
			}
			return false, nil
		}
		copyFieldsPreservingID(localSample, rowPtr)
		if err := local.Save(localSample).Error; err != nil {
			return false, err
		}
		if spec.Table == "jenis_arsip" {
			localID := uint(fieldUintOf(reflect.ValueOf(localSample), "ID"))
			remoteID := uint(fieldUintOf(reflect.ValueOf(rowPtr), "ID"))
			upsertIDMap(local, spec.Table, localID, remoteID, key)
		}
		return true, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return false, err
	}
	remoteCopy := newSample(spec.Model)
	copyAllFields(remoteCopy, rowPtr)
	if err := local.Omit("id").Create(remoteCopy).Error; err != nil {
		return false, err
	}
	e.fixDeletedAt(local, spec, remoteCopy)
	if spec.Table == "jenis_arsip" {
		localID := uint(fieldUintOf(reflect.ValueOf(remoteCopy), "ID"))
		remoteID := uint(fieldUintOf(reflect.ValueOf(rowPtr), "ID"))
		upsertIDMap(local, spec.Table, localID, remoteID, key)
	}
	e.postProcess(local, spec, remoteCopy)
	return true, nil
}

// insertRowLocal memasukkan baris pusat ke lokal; bila tabel punya FK
// jenisArsip (arsip) → remap dulu ke id lokal. Tombstone (deleted_at)
// dipastikan ikut tertulis.
func (e *Engine) insertRowLocal(local, remote *gorm.DB, spec TableSpec, rowPtr any) error {
	if spec.Table == "arsip" {
		row := rowPtr.(*models.Arsip)
		if row.JenisArsipID != nil && *row.JenisArsipID > 0 {
			lid, err := e.ensureLocalJenisForRemoteID(remote, local, *row.JenisArsipID)
			if err != nil {
				return err
			}
			row.JenisArsipID = &lid
		}
	}
	if err := local.Create(rowPtr).Error; err != nil {
		return err
	}
	e.fixDeletedAt(local, spec, rowPtr)
	return nil
}

// fixDeletedAt memastikan nilai tombstone ikut tersimpan pada insert.
func (e *Engine) fixDeletedAt(local *gorm.DB, spec TableSpec, rowPtr any) {
	if !spec.Soft {
		return
	}
	v := fvalue(reflect.ValueOf(rowPtr), "DeletedAt")
	if !v.IsValid() {
		return
	}
	da, ok := v.Interface().(gorm.DeletedAt)
	if !ok || !da.Valid {
		return
	}
	key := fieldStrOf(reflect.ValueOf(rowPtr), spec.KeyProp)
	if key == "" {
		return
	}
	local.Unscoped().Model(spec.Model).Where(spec.KeyCol+" = ?", key).Update("deleted_at", da.Time)
}

// postProcess dimanakan setelah baris lokal disisipkan/diperbarui:
//   - materialisasi lampiran (disk) untuk arsip;
//   - pembuatan ulang file QR bila PNG hilang.
func (e *Engine) postProcess(_ *gorm.DB, spec TableSpec, rowPtr any) {
	switch spec.Table {
	case "arsip":
		if row, ok := rowPtr.(*models.Arsip); ok {
			e.materializeArsipFile(row)
		}
	case "qr_codes":
		if row, ok := rowPtr.(*models.QrCode); ok {
			e.ensureQrFile(row)
		}
	}
}

// ensureLocalJenisForRemoteID membuat baris jenis_arsip lokal (bila belum ada)
// untuk id jenis pusat yang diberikan; mengembalikan id lokal ANDA.
func (e *Engine) ensureLocalJenisForRemoteID(remote, local *gorm.DB, remoteID uint) (uint, error) {
	var m models.SyncIDMap
	if err := local.Where("table_name = ? AND remote_id = ?", "jenis_arsip", remoteID).First(&m).Error; err == nil {
		return m.LocalID, nil
	}
	var jenis models.JenisArsip
	if err := remote.Unscoped().Where("id = ?", remoteID).First(&jenis).Error; err != nil {
		return 0, err
	}
	var cur models.JenisArsip
	if err := local.Where("uuid = ?", jenis.UUID).First(&cur).Error; err == nil {
		upsertIDMap(local, "jenis_arsip", cur.ID, jenis.ID, jenis.UUID)
		return cur.ID, nil
	}
	cop := jenis
	cop.ID = 0
	if err := local.Omit("id").Create(&cop).Error; err != nil {
		return 0, err
	}
	upsertIDMap(local, "jenis_arsip", cop.ID, jenis.ID, jenis.UUID)
	return cop.ID, nil
}

// pullPermissionRole menggantikan gabungan relasi permission_role lokal dari
// pusat (penuh-ganti). Peran lokal yang dibuat offline tetap tersedia;
// penetapan izin pusat selalu menang.
func (e *Engine) pullPermissionRole(local, remote *gorm.DB) (int, error) {
	type joinRow struct {
		RoleID       string
		PermissionID string
	}
	var remoteRows []joinRow
	// Nama kolom di tabel join mengikuti konvensi GORM: role_id, permission_id.
	if err := remote.Table("permission_role").Select("role_id, permission_id").Scan(&remoteRows).Error; err != nil {
		return 0, err
	}
	type pair struct {
		RoleID, PermissionID string
	}
	existing := map[pair]bool{}
	var localRows []joinRow
	if err := local.Table("permission_role").Select("COALESCE(role_id,'') as role_id, COALESCE(permission_id,'') as permission_id").Scan(&localRows).Error; err != nil {
		return 0, err
	}
	for _, r := range localRows {
		existing[pair{RoleID: r.RoleID, PermissionID: r.PermissionID}] = true
	}
	if len(remoteRows) == len(existing) {
		same := true
		for _, r := range remoteRows {
			if !existing[pair{RoleID: r.RoleID, PermissionID: r.PermissionID}] {
				same = false
				break
			}
		}
		if same && len(remoteRows) == len(localRows) {
			return 0, nil
		}
	}
	if err := local.Exec("DELETE FROM permission_role").Error; err != nil {
		return 0, err
	}
	inserted := 0
	for _, r := range remoteRows {
		if r.RoleID == "" || r.PermissionID == "" {
			continue
		}
		if err := local.Exec("INSERT INTO permission_role (role_id, permission_id) VALUES (?, ?)", r.RoleID, r.PermissionID).Error; err != nil {
			log.Printf("[SYNC][permission_role] insert sepotong gagal: %v", err)
			continue
		}
		inserted++
	}
	return inserted, nil
}
