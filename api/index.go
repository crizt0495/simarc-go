// Package handler — Vercel serverless function adapter untuk SIMARC.
//
// Runtime Go Vercel mengompilasi setiap file di /api dan mensyaratkan
// `package handler` dengan fungsi eksport `Handler(w, r)`. Seluruh bootstrap
// aplikasi (config, database pool, migrasi, template, routes) ada di
// internal/app dan dijalankan sekali per cold start; invokasi hangat
// memakai ulang router dan koneksi database yang sudah ada.
package handler

import (
	"net/http"

	"arsippro/internal/app"
)

// Handler dipanggil Vercel untuk setiap request HTTP masuk.
func Handler(w http.ResponseWriter, r *http.Request) {
	app.Handler(w, r)
}
