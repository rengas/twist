package main

import (
	"embed"

	"github.com/rengas/twist/pkg"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	app := pkg.NewApp()

	err := wails.Run(&options.App{
		Title:            "twist",
		Width:            1400,
		Height:           900,
		MinWidth:         1000,
		MinHeight:        700,
		WindowStartState: options.Maximised,
		BackgroundColour: &options.RGBA{R: 15, G: 23, B: 42, A: 255}, // #0f172a
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup: app.Startup,
		Bind: []interface{}{
			app,
		},
		Mac: &mac.Options{
			TitleBar: mac.TitleBarHiddenInset(),
			About: &mac.AboutInfo{
				Title:   "twist",
				Message: "Spec-driven, approval-gated Kanban workflow powered by Claude",
			},
		},
	})
	if err != nil {
		println("Error:", err.Error())
	}
}
