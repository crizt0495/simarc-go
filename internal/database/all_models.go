package database

import "arsippro/internal/models"

// allModels mengembalikan seluruh model GORM yang perlu dibuatkan tabel.
// Dipakai jalur MySQL (AutoMigrate) maupun SQLite. Daftar tunggalnya berada di
// internal/models supaya syncdb bisa memakai model yang sama untuk membuat
// tabel yang belum ada di hub.
func allModels() []interface{} {
	return models.AllModels()
}
