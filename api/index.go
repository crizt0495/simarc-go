// Package api — Vercel serverless function adapter for SIMARC.
//
// Vercel's Go runtime compiles each .go file under /api as an independent
// entry point and expects a `Handler(w, r)` signature. All application
// bootstrap (config, database pool, migrations, templates, routes) lives in
// internal/app and is executed once per cold start; warm invocations reuse
// the existing router and database connections.
package main

import (
	"net/http"

	"arsippro/internal/app"
)

// Handler is invoked by Vercel for every incoming HTTP request.
func Handler(w http.ResponseWriter, r *http.Request) {
	app.Handler(w, r)
}
