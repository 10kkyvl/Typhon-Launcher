package main

import (
	"embed"
	"errors"
	"log/slog"
	"os"

	"typhon/internal/account"
	"typhon/internal/app"
	"typhon/internal/catalog"
	"typhon/internal/discord"
	"typhon/internal/discovery"
	"typhon/internal/download"
	"typhon/internal/install"
	"typhon/internal/legal"
	"typhon/internal/library"
	"typhon/internal/metadata"
	"typhon/internal/metadata/typhonapi"
	"typhon/internal/presence"
	"typhon/internal/search"
	"typhon/internal/selfupdate"
	"typhon/internal/settings"
	"typhon/internal/sources"
	"typhon/internal/updates"

	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed all:frontend/dist
var assets embed.FS

const discordClientID = "1541194395964014623"

var errNoWorkerSpec = errors.New("--install-worker требует путь к файлу задания")
var errNoSelfupdateWorkerSpec = errors.New("--selfupdate-worker требует путь к файлу задания")

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
	application.RegisterEvent[metadata.View]("metadata:updated")
	application.RegisterEvent[discovery.Progress]("discovery:started")
	application.RegisterEvent[discovery.Progress]("discovery:progress")
	application.RegisterEvent[discovery.Result]("discovery:completed")
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
	application.RegisterEvent[selfupdate.Status]("launcher:update_status")
	application.RegisterEvent[selfupdate.Progress]("launcher:update_progress")
}

func main() {
	app.InitLogging()

	if len(os.Args) > 1 && os.Args[1] == "--install-worker" {
		if len(os.Args) < 3 {
			slog.Error("install worker failed", "error", errNoWorkerSpec)
			os.Exit(1)
		}
		if err := install.RunWorker(os.Args[2]); err != nil {
			slog.Error("install worker failed", "error", err)
			os.Exit(1)
		}
		return
	}

	if len(os.Args) > 1 && os.Args[1] == "--selfupdate-worker" {
		if len(os.Args) < 3 {
			slog.Error("selfupdate worker failed", "error", errNoSelfupdateWorkerSpec)
			os.Exit(1)
		}
		if err := selfupdate.RunWorker(os.Args[2]); err != nil {
			slog.Error("selfupdate worker failed", "error", err)
			os.Exit(1)
		}
		return
	}

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
	metadataService, err := metadata.NewService(catalogService, metadataProvider(accountService))
	if err != nil {
		fatal("start metadata service", err)
	}
	discoveryService, err := discovery.NewService(settingsService, libraryService, catalogService, metadataService)
	if err != nil {
		fatal("start discovery service", err)
	}
	searchService := search.NewService(libraryService, catalogService, sourcesService)
	updateService, err := updates.NewService(settingsService, libraryService, sourcesService, downloadManager, installService)
	if err != nil {
		fatal("start updates service", err)
	}
	selfupdateService, err := selfupdate.NewService()
	if err != nil {
		fatal("start selfupdate service", err)
	}
	discordService, err := discord.NewService(discordClientID)
	if err != nil {
		fatal("start discord presence", err)
	}
	legalService, err := legal.NewService(legalDocs)
	if err != nil {
		fatal("start legal service", err)
	}
	presenceWatcher, err := presence.New(discordService, metadataService.CoverSourceURL)
	if err != nil {
		fatal("start discord presence", err)
	}
	installService.SetTitleResolver(func(origin download.Origin) string {
		return gameTitle(catalogService, sourcesService, origin.GameID, origin.ReleaseID)
	})
	if err := libraryService.SyncTitles(func(canonicalGameID, releaseID string) string {
		return gameTitle(catalogService, sourcesService, canonicalGameID, releaseID)
	}); err != nil {
		fatal("sync library titles", err)
	}
	downloadManager.SetOnCompleted(installService.HandleDownloadCompleted)
	installService.SetOnFinished(updateService.HandleInstallFinished)
	installService.SetBusyCheck(updateService.Busy)
	sourcesService.SetOnChanged(updateService.HandleSourcesRefreshed)
	libraryService.SetOnSessionEnded(updateService.HandleSessionEnded)
	libraryService.SetSessionWatcher(presenceWatcher)
	presenceWatcher.Apply(settingsService.GetSettings())
	settingsService.Subscribe(presenceWatcher.Apply)

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
			application.NewService(metadataService),
			application.NewService(discoveryService),
			application.NewService(discordService),
			application.NewService(legalService),
			application.NewService(selfupdateService),
		},
		Assets: application.AssetOptions{
			Handler:    application.AssetFileServerFS(assets),
			Middleware: metadataService.Middleware,
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

func metadataProvider(accountService *account.Service) metadata.Provider {
	client, err := typhonapi.New(account.BaseURL(), accountService.SessionToken)
	if err != nil {
		slog.Error("metadata provider disabled", "error", err)
		return nil
	}
	return client
}

func gameTitle(cat *catalog.Service, src *sources.Service, canonicalGameID, releaseID string) string {
	if title := cat.TitleOf(canonicalGameID); title != "" {
		return title
	}
	if release, ok := src.FindRelease(releaseID); ok {
		return release.Title
	}
	return ""
}

func fatal(stage string, err error) {
	slog.Error("startup failed", "stage", stage, "err", err)
	os.Exit(1)
}
