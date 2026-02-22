package main

import (
	"embed"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	app := NewApp()

	err := wails.Run(&options.App{
		Title:     "Direct Connector",
		Width:     960,
		Height:    680,
		MinWidth:  820,
		MinHeight: 560,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		// gray-900 background — matches the Tailwind theme so there is no
		// flash of white during initial load.
		BackgroundColour: &options.RGBA{R: 17, G: 24, B: 39, A: 255},
		OnStartup:        app.startup,
		Bind:             []interface{}{app},
		Mac: &mac.Options{
			TitleBar: mac.TitleBarDefault(),
			About: &mac.AboutInfo{
				Title:   "Direct Connector",
				Message: "SSH Tunnel & P2P Manager\n\nVersion 1.0",
			},
		},
		Windows: &windows.Options{
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
			DisableWindowIcon:    false,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
