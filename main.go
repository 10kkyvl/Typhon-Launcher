package main

import (
	"embed"
	"log"
	"log/slog"
	"os"

	"aurora/internal/app"
	"aurora/internal/settings"

	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed all:frontend/dist
var assets embed.FS

func init() {
	application.RegisterEvent[settings.Settings]("settings:updated")
}

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))

	settingsService := settings.NewService()
	appService := app.NewService(settingsService)

	wails := application.New(application.Options{
		Name:        "Aurora",
		Description: "Aurora game launcher",
		Services: []application.Service{
			application.NewService(appService),
			application.NewService(settingsService),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	wails.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            "Aurora",
		Width:            1440,
		Height:           900,
		MinWidth:         1000,
		MinHeight:        680,
		Frameless:        true,
		BackgroundColour: application.NewRGB(11, 16, 22),
		Mac: application.MacWindow{
			InvisibleTitleBarHeight: 50,
			Backdrop:                application.MacBackdropTranslucent,
			TitleBar:                application.MacTitleBarHiddenInset,
		},
		URL: "/",
	})

	slog.Info("aurora starting", "version", app.Version)
	if err := wails.Run(); err != nil {
		log.Fatal(err)
	}
}
