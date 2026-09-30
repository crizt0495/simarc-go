//go:build desktop

// Command desktop runs SIMARC as a native desktop application.
//
// It reuses the existing web application wholesale: app.Init() builds the same
// gin engine cmd/server serves, and Wails' asset server hands every request
// from the window straight to it. Nothing in internal/ is re-plumbed and no
// frontend build step is involved — the shell only changes one thing, the
// navigation, by switching the template layout mode to the horizontal top bar
// (the way recent POS apps present their modules) instead of the left sidebar.
//
// Build with the Wails CLI, from this directory, so the packaging assets in
// desktop/build/ are picked up:
//
//	cd desktop && wails build -tags "desktop,webkit2_41" -skipbindings
//
// Plain `go build` works too but must pass the tags Wails itself relies on,
// and `production` is not optional — without it wails.Run refuses to start:
//
//	Linux   go build -tags "desktop webkit2_41 production" -o simarc ./desktop
//	macOS   go build -tags "desktop production" -o simarc ./desktop
//	Windows go build -tags "desktop production" -o simarc.exe ./desktop
//
// `desktop` and `production` both carry the `//go:build desktop` constraint on
// this file, which keeps it and the Wails dependency out of ordinary builds:
// `go build ./...` and the Vercel build, which compiles only ./cmd/server, are
// unaffected.
package main

import (
	"context"
	"log"
	"os"

	"arsippro/internal/app"
	"arsippro/internal/config"
	"arsippro/internal/handlers"

	"github.com/gin-gonic/gin"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

// newEngine builds the gin engine the desktop window serves.
//
// It is deliberately a separate function: the shell's whole contract with the
// rest of the application is "the normal engine, in top-navigation mode", and
// newEngine is where that contract is defined, so a test can exercise exactly
// what the window gets.
func newEngine() (*gin.Engine, error) {
	// The one behavioural difference from the browser build: the top bar.
	// Set before the first request so every page renders the desktop shell.
	handlers.SetLayoutMode(handlers.LayoutModeTopbar)

	prepareWorkingDir()

	// Same bootstrap as cmd/server — config, database, migrations, sessions,
	// queue workers, templates, routes.
	return app.Init()
}

// prepareWorkingDir gives the app a predictable working directory.
//
// Launched from a development checkout, the app runs from the checkout root so
// templates and .env are read from disk and an edit shows up on the next
// start. Launched as an installed application — where the working directory is
// whatever the desktop environment happened to choose — it runs from the
// per-user data directory instead, which is where config and runtime files
// belong and what makes backups, uploads and logs land in the same place on
// every run. Templates then come from the ones embedded in the binary.
func prepareWorkingDir() {
	wd, err := os.Getwd()
	if err != nil {
		log.Printf("[desktop] tidak bisa membaca working directory: %v", err)
		return
	}

	// Development: use the checkout root, wherever in it we were started.
	if root := config.SourceRoot(wd); root != "" {
		if root == wd {
			return
		}
		if err := os.Chdir(root); err != nil {
			log.Printf("[desktop] gagal pindah ke %s: %v", root, err)
		}
		return
	}

	// Installed: the per-user data directory.
	dir := config.DataDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Printf("[desktop] tidak bisa membuat %s: %v — memakai %s", dir, err, wd)
		return
	}
	if err := os.Chdir(dir); err != nil {
		log.Printf("[desktop] gagal pindah ke %s: %v — memakai %s", dir, err, wd)
	}
}

func main() {
	r, err := newEngine()
	if err != nil {
		log.Fatalf("Gagal menginisialisasi aplikasi: %v", err)
	}

	// Optional diagnostics (SIMARC_DESKTOP_LOG / SIMARC_DESKTOP_DUMP). With no
	// environment set this is the bare engine, so a normal session pays nothing.
	handler := instrument(r)

	// Opt-in hit-test probe (SIMARC_DESKTOP_PROBE). It answers questions that
	// only the real WebKitGTK engine can answer — such as what is actually
	// painted over the top navigation — from inside the window itself.
	handler = withProbe(handler)

	// Serve the app on a real loopback origin. Wails' own origin is the custom
	// scheme wails://wails/ on Linux and macOS, and WebKit keeps no cookies for
	// a custom scheme — so the session never survived the login POST. See
	// loopback.go for the full story.
	lb, err := serveOnLoopback(handler)
	if err != nil {
		log.Fatalf("Gagal menyiapkan server lokal: %v", err)
	}
	defer lb.close()
	log.Printf("[desktop] aplikasi dilayani di %s", lb.url)

	if err := wails.Run(&options.App{
		Title:     config.App.AppName,
		Width:     1440,
		Height:    900,
		MinWidth:  1024,
		MinHeight: 640,

		// The asset server only hands the window a bootstrap page whose sole job
		// is to navigate to the loopback origin; every real request is served by
		// the gin engine over plain HTTP from there on. There is no separate
		// frontend bundle to keep in sync.
		AssetServer: &assetserver.Options{
			Handler: bootstrapPage(lb.url),
		},

		OnStartup: func(ctx context.Context) {
			log.Println("[desktop] wails startup complete")
		},
		OnDomReady: func(ctx context.Context) {
			log.Println("[desktop] frontend DOM ready")
		},
		OnShutdown: func(ctx context.Context) {
			log.Println("[desktop] wails shutdown")
		},

		// SIMARC is a multi-page server-rendered app: never cache the shell, and
		// let the application decide when to open external links.
		StartHidden: false,
	}); err != nil {
		log.Printf("Desktop runtime error: %v", err)
		os.Exit(1)
	}
}
