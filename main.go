package main

import (
	"bufio"
	"embed"
	"os"
	"path/filepath"
	"strings"

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
	app := NewApp()

	window := strings.ToLower(readDotEnv("SIMARC_WINDOW", "fullscreen"))

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
		Linux: &linux.Options{
			WindowIsTranslucent: false,
		},
		Windows: &windows.Options{
			WebviewIsTransparent: false,
		},
		Mac: &mac.Options{},
		WindowStartState: windowOpts(window).startState,
		Frameless:        windowOpts(window).frameless,
		DisableResize:    windowOpts(window).disallowResize,
	}

	if err := wails.Run(opts); err != nil {
		println("Error:", err.Error())
	} else {
		println("wails.Run selesai (jendela ditutup)")
	}
}

type windowFlags struct {
	startState     options.WindowStartState
	frameless      bool
	disallowResize bool
}

func windowOpts(mode string) windowFlags {
	switch mode {
	case "maximized":
		return windowFlags{startState: options.Maximised}
	case "normal":
		return windowFlags{startState: options.Normal}
	case "kiosk":
		return windowFlags{startState: options.Fullscreen, frameless: true, disallowResize: true}
	case "fullscreen":
		return windowFlags{startState: options.Fullscreen}
	}
	return windowFlags{startState: options.Fullscreen}
}

func readDotEnv(key, fb string) string {
	paths := []string{".env"}
	if exe, err := os.Executable(); err == nil {
		paths = append(paths, filepath.Join(filepath.Dir(exe), ".env"))
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		paths = append(paths, filepath.Join(home, ".env"))
	}
	for _, p := range paths {
		f, err := os.Open(p)
		if err != nil {
			continue
		}
		defer f.Close()
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			if eq := strings.Index(line, "="); eq > 0 && strings.TrimSpace(line[:eq]) == key {
				return strings.Trim(strings.TrimSpace(line[eq+1:]), `"'`)
			}
		}
	}
	return fb
}
