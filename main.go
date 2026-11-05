// Study: main SIMARC — aplikasi desktop standalone (Wails + React).
// Tidak ada dev server, tidak ada browser eksternal: seluruh UI React
// di-embed ke executable dan dirender WebView bawaan sistem operasi.
package main

import (
	"embed"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/linux"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	app := NewApp(assets)

	opts := &options.App{
		Title:     "SIMARC — Arsip Record Center",
		Width:     1280,
		Height:    800,
		MinWidth:  1180,
		MinHeight: 700,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 243, G: 245, B: 249, A: 255},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		Bind: []interface{}{
			app,
		},
		Linux:  &linux.Options{WindowIsTranslucent: false},
		Windows: &windows.Options{WebviewIsTransparent: false},
		Mac:    &mac.Options{},
	}
	if err := wails.Run(opts); err != nil {
		println("Error:", err.Error())
	} else {
		println("wails.Run selesai (jendela ditutup)")
	}
	println("App exited")
}
