package main

import (
	"embed"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/logger"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
	"github.com/wailsapp/wails/v2/pkg/runtime"

	"reclaimed/internal/art"
	"reclaimed/internal/paths"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	l, err := paths.Resolve()
	if err != nil {
		println(err.Error())
		os.Exit(1)
	}
	os.MkdirAll(l.Data, 0o755)
	app := NewApp(l)

	err = wails.Run(&options.App{
		Title:            "Reclaimed",
		Width:            1180,
		Height:           740,
		MinWidth:         960,
		MinHeight:        620,
		Frameless:        true,
		BackgroundColour: &options.RGBA{R: 8, G: 11, B: 16, A: 255},
		AssetServer:      &assetserver.Options{Assets: assets, Handler: art.Handler(l.Art)},
		OnStartup:        app.startup,
		OnBeforeClose:    app.beforeClose,
		Bind:             []interface{}{app},
		Logger:           logger.NewFileLogger(filepath.Join(l.Data, "launcher.log")),
		LogLevel:         logger.INFO,
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId: "reclaimed-launcher-5f0c2a",
			OnSecondInstanceLaunch: func(options.SecondInstanceData) {
				runtime.WindowUnminimise(app.ctx)
				runtime.Show(app.ctx)
			},
		},
		Windows: &windows.Options{
			Theme: windows.Dark,
			// Keep WebView2's cache in the launcher's folder instead of %APPDATA%.
			WebviewUserDataPath: filepath.Join(l.Data, "webview"),
		},
	})
	if err != nil {
		println("Error:", err.Error())
	}
}
