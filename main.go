package main

import (
	"embed"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"

	"typhon/internal/account"
	"typhon/internal/accountsync"
	"typhon/internal/app"
	"typhon/internal/autostart"
	"typhon/internal/catalog"
	"typhon/internal/clientid"
	"typhon/internal/diagnostics"
	"typhon/internal/discord"
	"typhon/internal/discovery"
	"typhon/internal/download"
	"typhon/internal/heartbeat"
	"typhon/internal/install"
	"typhon/internal/legal"
	"typhon/internal/library"
	"typhon/internal/metadata"
	"typhon/internal/metadata/typhonapi"
	"typhon/internal/presence"
	"typhon/internal/redact"
	"typhon/internal/search"
	"typhon/internal/selfupdate"
	"typhon/internal/settings"
	"typhon/internal/sources"
	"typhon/internal/tray"
	"typhon/internal/updates"
	"typhon/internal/usagestats"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/appicon.png
var trayIcon []byte

const discordClientID = "1541194395964014623"

const singleInstanceID = "com.typhon.launcher"

var errNoWorkerSpec = errors.New("--install-worker требует путь к файлу задания")
var errNoSelfupdateWorkerSpec = errors.New("--selfupdate-worker требует путь к файлу задания")
var errNoPlayTarget = errors.New("--play требует идентификатор игры")

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

// registerLocalIdentity hands the machine and account names to redact so they
// are scrubbed from diagnostics text wherever they appear, not only inside a
// path: a username in "account egor is not authorized" matches no path rule.
// A name that will not resolve is logged and skipped rather than fatal — the
// pattern rules still run, and a launcher that refuses to start over a
// hostname lookup trades far more than it gains.
func registerLocalIdentity() {
	host, err := os.Hostname()
	if err != nil {
		slog.Warn("resolve hostname for redaction", "error", err)
	}
	user := ""
	home, err := os.UserHomeDir()
	if err != nil {
		slog.Warn("resolve home dir for redaction", "error", err)
	} else {
		user = filepath.Base(home)
	}
	redact.SetLocal(host, user)
}

func main() {
	// A locked or unwritable log file must not stop the launcher from
	// starting, but it must not pass silently either: stderr logging is
	// already live here, so the failure is recorded rather than discarded.
	if err := app.InitLogging(); err != nil {
		slog.Error("init logging", "component", "app", "operation", "init_logging", "error", err)
	}

	registerLocalIdentity()

	// diagService is assigned once the identity/telemetry block below
	// constructs it; the defer reads the variable itself at panic time, not
	// a snapshot taken here, so it still reports panics that happen after
	// the service exists.
	var diagService *diagnostics.Service
	defer func() {
		if r := recover(); r != nil {
			if diagService != nil {
				diagService.CapturePanic("launcher", r, debug.Stack())
			}
			panic(r) //nolint:forbidigo // re-throws an already-recovered invariant violation so the process still crashes as it would have; this is not a new panic on input/file/network
		}
	}()

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

	// A shortcut with a broken argument must not keep the launcher from
	// starting: the window opens as usual and the user can launch the game
	// by hand, which is strictly better than refusing to start at all.
	playID, playRequested, err := playTarget(os.Args)
	if err != nil {
		slog.Error("parse play argument", "args", os.Args[1:], "error", err)
		playRequested = false
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
	provider := metadataProvider(accountService)
	metadataService, err := metadata.NewService(catalogService, provider)
	if err != nil {
		fatal("start metadata service", err)
	}
	configDir, err := settings.ConfigDir()
	if err != nil {
		fatal("resolve config dir", err)
	}
	accountSyncService, err := accountsync.NewService(
		configDir,
		account.BaseURL(),
		accountService.SessionToken,
		syncSettings{settingsService},
		syncLibrary{libraryService},
		syncCatalog{catalogService},
		syncMetadata{provider},
	)
	if err != nil {
		fatal("start account sync service", err)
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
	libraryService.AddSessionWatcher(presenceWatcher)
	presenceWatcher.Apply(settingsService.GetSettings())
	settingsService.Subscribe(presenceWatcher.Apply)

	var extraServices []application.Service

	// Битый или недоступный installation.json — не повод не пускать пользователя
	// играть: presence и телеметрия в этом запуске просто не поднимаются, лаунчер
	// работает дальше без них.
	identity, err := clientid.Load()
	if err != nil {
		slog.Error("load client identity", "error", err)
	} else {
		resolveGameID := func(catalogGameID string) string { return catalogService.IGDBIDOf(catalogGameID) }
		heartbeatService, err := heartbeat.NewService(identity, resolveGameID)
		if err != nil {
			fatal("start presence", err)
		}
		// Both services ask *Allowed, never the switch itself: on a fresh
		// install the diagnostics switch already reads true so the consent
		// prompt can start with it selected, and sending on the strength of
		// that would be collecting from someone who has not been asked yet.
		usageService, err := usagestats.NewService(identity,
			func() bool { return settingsService.GetSettings().UsageStatsAllowed() }, resolveGameID)
		if err != nil {
			fatal("start usage stats", err)
		}
		usageService.SetEnabled(settingsService.GetSettings().UsageStatsAllowed())
		settingsService.Subscribe(func(s settings.Settings) { usageService.SetEnabled(s.UsageStatsAllowed()) })

		diagnosticsService, err := diagnostics.NewService(identity,
			func() bool { return settingsService.GetSettings().DiagnosticsAllowed() })
		if err != nil {
			fatal("start diagnostics", err)
		}
		diagnosticsService.SetEnabled(settingsService.GetSettings().DiagnosticsAllowed())
		settingsService.Subscribe(func(s settings.Settings) { diagnosticsService.SetEnabled(s.DiagnosticsAllowed()) })
		diagService = diagnosticsService

		libraryService.AddSessionWatcher(heartbeatService)
		libraryService.SetUsageRecorder(usageService.Record)
		downloadManager.SetUsageRecorder(usageService.Record)
		installService.SetUsageRecorder(usageService.Record)
		updateService.SetUsageRecorder(usageService.Record)

		extraServices = append(extraServices,
			application.NewService(heartbeatService),
			application.NewService(usageService),
			application.NewService(diagnosticsService),
		)
	}

	current := settingsService.GetSettings()

	var trayController *tray.Controller

	services := []application.Service{
		application.NewService(appService),
		application.NewService(accountService),
		application.NewService(accountSyncService),
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
	}
	services = append(services, extraServices...)

	wails := application.New(application.Options{
		Name:        "Typhon",
		Description: "Typhon game launcher",
		Windows: application.WindowsOptions{
			AdditionalBrowserArgs: browserArgs(current.HardwareAcceleration),
		},
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: singleInstanceID,
			OnSecondInstanceLaunch: func(data application.SecondInstanceData) {
				id, requested, err := playTarget(data.Args)
				if err != nil {
					slog.Error("parse play argument", "args", data.Args, "error", err)
				}
				if !requested {
					trayController.Open()
					return
				}
				if err := libraryService.PlayGame(id); err != nil {
					slog.Error("play from shortcut", "id", id, "error", err)
					trayController.Open()
				}
			},
		},
		Services: services,
		Assets: application.AssetOptions{
			Handler:    application.AssetFileServerFS(assets),
			Middleware: metadataService.Middleware,
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	window := wails.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            "Typhon",
		Width:            1440,
		Height:           900,
		MinWidth:         1000,
		MinHeight:        680,
		Frameless:        true,
		BackgroundColour: application.NewRGB(11, 16, 22),
		// Started from a game shortcut the launcher stays out of the way, but
		// only when the tray icon is there to bring it back: hiding a window
		// nobody can reopen leaves a process the user cannot reach.
		Hidden: playRequested && current.MinimizeToTray,
		Mac: application.MacWindow{
			InvisibleTitleBarHeight: 50,
			Backdrop:                application.MacBackdropTranslucent,
			TitleBar:                application.MacTitleBarHiddenInset,
		},
		URL: "/",
	})

	autostartService, err := autostart.NewService(wails.Autostart)
	if err != nil {
		fatal("start autostart service", err)
	}
	trayController, err = tray.New(windowControl{window: window}, func() (tray.Tray, error) {
		return newSystemTray(wails, trayController)
	}, wails.Quit)
	if err != nil {
		fatal("start tray controller", err)
	}
	if err := settingsService.AddApplier(func(prev, next settings.Settings) error {
		if prev.LaunchOnStartup == next.LaunchOnStartup {
			return nil
		}
		return autostartService.Apply(next.LaunchOnStartup)
	}); err != nil {
		fatal("register autostart applier", err)
	}
	if err := settingsService.AddApplier(func(prev, next settings.Settings) error {
		if prev.MinimizeToTray == next.MinimizeToTray {
			return nil
		}
		return trayController.Apply(next.MinimizeToTray)
	}); err != nil {
		fatal("register tray applier", err)
	}

	// A locked-down registry or a refused tray icon must not keep the launcher
	// from starting: the toggle in settings goes through SaveSettings, which
	// runs the same appliers and does report the failure to the user.
	if err := autostartService.Apply(current.LaunchOnStartup); err != nil {
		slog.Error("apply autostart", "error", err)
	}
	if err := trayController.Apply(current.MinimizeToTray); err != nil {
		slog.Error("apply tray", "error", err)
	}

	window.RegisterHook(events.Common.WindowClosing, func(event *application.WindowEvent) {
		if trayController.CloseRequested() {
			event.Cancel()
		}
	})

	if playRequested {
		if err := libraryService.PlayGame(playID); err != nil {
			slog.Error("play from shortcut", "id", playID, "error", err)
			window.Show()
		}
	}

	slog.Info("typhon starting", "version", app.Version)
	if err := wails.Run(); err != nil {
		fatal("run application", err)
	}
}

func playTarget(args []string) (string, bool, error) {
	for i := 1; i < len(args); i++ {
		if args[i] != "--play" {
			continue
		}
		if i+1 >= len(args) {
			return "", true, errNoPlayTarget
		}
		id := strings.TrimSpace(args[i+1])
		if id == "" {
			return "", true, errNoPlayTarget
		}
		return id, true, nil
	}
	return "", false, nil
}

func browserArgs(hardwareAcceleration bool) []string {
	if hardwareAcceleration {
		return nil
	}
	return []string{"--disable-gpu"}
}

type windowControl struct {
	window *application.WebviewWindow
}

func (w windowControl) Show() {
	w.window.Show()
}

func (w windowControl) Hide() {
	w.window.Hide()
}

func (w windowControl) Focus() {
	w.window.Focus()
}

func newSystemTray(wails *application.App, controller *tray.Controller) (tray.Tray, error) {
	menu := application.NewMenu()
	menu.Add("Открыть Typhon").OnClick(func(*application.Context) {
		controller.Open()
	})
	menu.AddSeparator()
	menu.Add("Выход").OnClick(func(*application.Context) {
		controller.Quit()
	})

	systemTray := wails.SystemTray.New()
	systemTray.SetIcon(trayIcon)
	systemTray.SetTooltip("Typhon")
	systemTray.SetMenu(menu)
	systemTray.OnClick(controller.Open)
	return systemTray, nil
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
