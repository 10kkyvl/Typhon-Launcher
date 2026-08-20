package main

import (
	"embed"
	"log"
	"log/slog"
	"os"

	"typhon/internal/app"
	"typhon/internal/download"
	"typhon/internal/library"
	"typhon/internal/settings"

	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed all:frontend/dist
var assets embed.FS

func init() {
	application.RegisterEvent[settings.Settings]("settings:updated")
	application.RegisterEvent[[]library.Game]("library:updated")
	application.RegisterEvent[library.SessionEvent]("game:started")
	application.RegisterEvent[library.SessionEvent]("game:stopped")
	application.RegisterEvent[download.Download]("download:added")
	application.RegisterEvent[download.Download]("download:updated")
	application.RegisterEvent[download.Download]("download:completed")
	application.RegisterEvent[download.Download]("download:failed")
	application.RegisterEvent[download.RemovedEvent]("download:removed")
}

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))

	settingsService := settings.NewService()
	appService := app.NewService(settingsService)
	libraryService := library.NewService()
	downloadManager := download.NewManager(settingsService)

	wails := application.New(application.Options{
		Name:        "Typhon",
		Description: "Typhon game launcher",
		Services: []application.Service{
			application.NewService(appService),
			application.NewService(settingsService),
			application.NewService(libraryService),
			application.NewService(downloadManager),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	wails.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            "Typhon",
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

	slog.Info("typhon starting", "version", app.Version)
	if err := wails.Run(); err != nil {
		log.Fatal(err)
	}
}
