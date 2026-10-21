package desksync

import (
	"reflect"
	"time"

	"gorm.io/gorm"
)

// ── Refleksi util ────────────────────────────────────────────────────────────
// Operasi generik atas baris model (pointer struct) lintas tabel syncable.

// elemType mengembalikan tipe struct dari pointer model.
func elemType(model any) reflect.Type {
	t := reflect.TypeOf(model)
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t
}

// newSample membuat pointer struct barkosong dengan tipe model.
func newSample(model any) any {
	return reflect.New(elemType(model)).Interface()
}

// newSlice membuat pointer ke slice baris ([]T) dari tipe model.
func newSlice(model any) any {
	return reflect.New(reflect.SliceOf(elemType(model))).Interface()
}

// fvalue mengambil refleksi field nama tertentu dari value struct/pointer.
func fvalue(v reflect.Value, name string) reflect.Value {
	for v.Kind() == reflect.Pointer {
		v = v.Elem()
	}
	return v.FieldByName(name)
}

// fieldStrOf mengambil field string dari struct baris.
func fieldStrOf(v reflect.Value, name string) string {
	f := fvalue(v, name)
	if f.IsValid() && f.Kind() == reflect.String {
		return f.String()
	}
	return ""
}

// fieldUintOf mengambil field integer unsigned dari struct baris.
func fieldUintOf(v reflect.Value, name string) uint64 {
	f := fvalue(v, name)
	switch f.Kind() {
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return f.Uint()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return uint64(f.Int())
	default:
		return 0
	}
}

// timeOf mengambil field timestamp (time.Time) dari struct baris.
func timeOf(row any, propName string) time.Time {
	f := fvalue(reflect.ValueOf(row), propName)
	if f.IsValid() && f.Type() == reflect.TypeOf(time.Time{}) {
		return f.Interface().(time.Time)
	}
	return time.Time{}
}

// deletedAtOf mengambil tombstone (time.Time, valid) dari struct baris bila
// model soft-delete. Return (zero,false) untuk model non-soft / baris hidup.
func deletedAtOf(row any) (time.Time, bool) {
	f := fvalue(reflect.ValueOf(row), "DeletedAt")
	if !f.IsValid() {
		return time.Time{}, false
	}
	da, ok := f.Interface().(gorm.DeletedAt)
	if !ok {
		return time.Time{}, false
	}
	if !da.Valid {
		return time.Time{}, false
	}
	return da.Time, true
}

// shouldApplyRowRemote (pull) — keputusan menerapkan baris pusat ke lokal:
//
//	tombstone pusat menang atas baris hidup lokal dan atas tombstone yang
//	lebih tua; selain itu LWW updated_at biasa (setara → lewati).
func shouldApplyRowRemote(spec TableSpec, lts, rts time.Time, lDel bool, lDelT time.Time, rDel bool, rDelT time.Time) bool {
	if spec.Soft {
		switch {
		case rDel:
			if !lDel || rDelT.After(lDelT) {
				return true
			}
			return false
		case lDel:
			return rts.After(lts) // pusat memulihkan baris (undel-ede)
		}
	}
	return rts.After(lts)
}

// shouldWriteRowRemote (push) — keputusan menulis baris lokal ke pusat:
//
//	tombstone lokal menang atas baris hidup pusat / tombstone lebih tua;
//	selain itu LWW updated_at biasa (setara → skip).
func shouldWriteRowRemote(spec TableSpec, lts, rts time.Time, lDel bool, lDelT time.Time, rDel bool, rDelT time.Time) bool {
	if spec.Soft {
		switch {
		case lDel:
			if !rDel || lDelT.After(rDelT) {
				return true
			}
			return false
		case rDel:
			return lts.After(rts) // lokal memulihkan baris
		}
	}
	return lts.After(rts)
}

// timestamp. Field relasi (slice) yang kosong di sumber DILEWATI.
func copyAllFields(dstAny, srcAny any) {
	dst := reflect.ValueOf(dstAny).Elem()
	src := reflect.ValueOf(srcAny).Elem()
	for i := 0; i < dst.NumField(); i++ {
		df, sf := dst.Field(i), src.Field(i)
		if !df.CanSet() || df.Type() != sf.Type() {
			continue
		}
		if sf.Kind() == reflect.Slice && sf.Len() == 0 {
			continue // relasi nil/kosong — jangan tumpukkan
		}
		df.Set(sf)
	}
}

// copyFieldsPreservingID seperti copyAllFields, tetapi mencegah ID ditulis —
// dipakai saat mengganti isi baris yang PK-nya milik node tempatkan.
func copyFieldsPreservingID(dstAny, srcAny any) {
	dst := reflect.ValueOf(dstAny).Elem()
	src := reflect.ValueOf(srcAny).Elem()
	for i := 0; i < dst.NumField(); i++ {
		name := dst.Type().Field(i).Name
		df, sf := dst.Field(i), src.Field(i)
		if name == "ID" || !df.CanSet() || df.Type() != sf.Type() {
			continue
		}
		if sf.Kind() == reflect.Slice && sf.Len() == 0 {
			continue
		}
		df.Set(sf)
	}
}
