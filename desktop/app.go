package desktop

import (
	"context"
	"embed"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func InitApp() (*wails.App, error) {
	app := wails.NewApp()
	app.SetAssetServer(assetserver.New(assets))
	app.SetTitle("SIMARC")
	app.SetSize(1280, 800)
	app.SetFullscreen(true)
	return app, nil
}
