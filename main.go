package main

import (
	"embed"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/logger"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
	"github.com/wailsapp/wails/v2/pkg/runtime"

	"reclaimed/internal/art"
	"reclaimed/internal/paths"
	"reclaimed/internal/proc"
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
	// After a launcher update the new exe is started before the old one has
	// quit; wait for it, or the single-instance lock would hand over to it.
	if len(os.Args) == 3 && os.Args[1] == "--after-update" {
		if pid, err := strconv.Atoi(os.Args[2]); err == nil {
			proc.WaitExit(pid, 20*time.Second)
		}
	}
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
		// Production builds use this level (Wails defaults it to ERROR).
		LogLevelProduction: logger.INFO,
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
