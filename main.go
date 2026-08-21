package main

import (
	"embed"
	"log/slog"
	"os"

	"typhon/internal/account"
	"typhon/internal/app"
	"typhon/internal/catalog"
	"typhon/internal/download"
	"typhon/internal/install"
	"typhon/internal/library"
	"typhon/internal/search"
	"typhon/internal/settings"
	"typhon/internal/sources"
	"typhon/internal/updates"

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
	application.RegisterEvent[install.Installation]("install:started")
	application.RegisterEvent[install.Installation]("install:updated")
	application.RegisterEvent[install.Installation]("install:completed")
	application.RegisterEvent[install.Installation]("install:failed")
	application.RegisterEvent[install.Installation]("install:cancelled")
	application.RegisterEvent[install.RemovedEvent]("install:removed")
	application.RegisterEvent[sources.Source]("source:updated")
	application.RegisterEvent[sources.SourceError]("source:error")
	application.RegisterEvent[sources.ReleaseBatch]("release:added")
	application.RegisterEvent[sources.ReleaseBatch]("release:removed")
	application.RegisterEvent[sources.ReleaseBatch]("release:matched")
	application.RegisterEvent[sources.ReleaseBatch]("release:needs-review")
	application.RegisterEvent[updates.Update]("update:available")
	application.RegisterEvent[updates.Update]("update:started")
	application.RegisterEvent[updates.Update]("update:updated")
	application.RegisterEvent[updates.Update]("update:completed")
	application.RegisterEvent[updates.Update]("update:failed")
	application.RegisterEvent[updates.Update]("update:rollback")
	application.RegisterEvent[updates.VerifyState]("verify:started")
	application.RegisterEvent[updates.VerifyState]("verify:updated")
	application.RegisterEvent[updates.VerifyState]("verify:completed")
	application.RegisterEvent[updates.VerifyState]("repair:started")
	application.RegisterEvent[updates.VerifyState]("repair:updated")
	application.RegisterEvent[updates.VerifyState]("repair:completed")
}

func main() {
	app.InitLogging()

	settingsService, err := settings.NewService()
	if err != nil {
		fatal("start settings service", err)
	}
	appService := app.NewService(settingsService)
	accountService, err := account.NewService()
	if err != nil {
		fatal("start account service", err)
	}
	libraryService, err := library.NewService()
	if err != nil {
		fatal("start library service", err)
	}
	downloadManager, err := download.NewManager(settingsService)
	if err != nil {
		fatal("start download manager", err)
	}
	installService, err := install.NewService(settingsService, downloadManager, libraryService)
	if err != nil {
		fatal("start install service", err)
	}
	catalogService, err := catalog.NewService()
	if err != nil {
		fatal("start catalog service", err)
	}
	sourcesService, err := sources.NewService(settingsService, catalogService)
	if err != nil {
		fatal("start sources service", err)
	}
	searchService := search.NewService(libraryService, catalogService, sourcesService)
	updateService, err := updates.NewService(settingsService, libraryService, sourcesService, downloadManager, installService)
	if err != nil {
		fatal("start updates service", err)
	}
	downloadManager.SetOnCompleted(installService.HandleDownloadCompleted)
	installService.SetOnFinished(updateService.HandleInstallFinished)
	sourcesService.SetOnChanged(updateService.HandleSourcesRefreshed)
	libraryService.SetOnSessionEnded(updateService.HandleSessionEnded)

	wails := application.New(application.Options{
		Name:        "Typhon",
		Description: "Typhon game launcher",
		Services: []application.Service{
			application.NewService(appService),
			application.NewService(accountService),
			application.NewService(settingsService),
			application.NewService(libraryService),
			application.NewService(downloadManager),
			application.NewService(installService),
			application.NewService(catalogService),
			application.NewService(sourcesService),
			application.NewService(searchService),
			application.NewService(updateService),
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
		fatal("run application", err)
	}
}

func fatal(stage string, err error) {
	slog.Error("startup failed", "stage", stage, "err", err)
	os.Exit(1)
}
