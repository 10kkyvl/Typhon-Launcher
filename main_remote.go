package main

import (
	"context"
	"log/slog"
	"runtime"
	"sync"
	"time"

	"typhon/internal/account"
	"typhon/internal/download"
	"typhon/internal/library"
	"typhon/internal/messaging"
	"typhon/internal/remote"
	"typhon/internal/settings"
	"typhon/internal/tray"
)

const remoteRegisterTimeout = 15 * time.Second

type remoteControlDeps struct {
	installationID string
	token          func() (string, error)
	downloads      remote.Downloads
	manager        *download.Manager
	releases       remote.Releases
	library        *library.Service
	settings       *settings.Service
	messaging      *messaging.Service
}

// startRemoteControl wires Discord remote control: the chat stream wakes it,
// and turning the setting on registers this launcher with the backend. It
// also wires the messages to the owner's Discord account: finished and
// failed downloads, and the outcome of downloads started from Discord. It
// returns what to call at shutdown.
func startRemoteControl(parent context.Context, d remoteControlDeps) (func(), error) {
	if d.installationID == "" {
		slog.Warn("discord remote control unavailable: no installation id", "component", "remote")
		return func() {}, nil
	}
	client, err := account.NewClient(account.BaseURL(), d.token)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(parent)
	notifier, err := remote.NewNotifier(ctx, remote.NotifierDeps{
		API:            client,
		InstallationID: d.installationID,
		Enabled:        func() bool { return d.settings.GetSettings().DiscordNotifications },
	})
	if err != nil {
		cancel()
		return nil, err
	}
	deps := remote.Deps{
		API:            client,
		Downloads:      d.downloads,
		InstallationID: d.installationID,
		Enabled:        func() bool { return d.settings.GetSettings().DiscordRemoteControl },
		Running:        func() []string { return runningTitles(d.library.RecentGames(tray.MaxRecent)) },
		Releases:       d.releases,
		DownloadsPath:  func() string { return d.settings.GetSettings().DownloadsPath },
		AddEnabled:     func() bool { return d.settings.GetSettings().DiscordRemoteDownloads },
		Notify:         notifier.Send,
	}
	if d.manager != nil {
		deps.Starter = d.manager
		d.manager.SetOnOutcome(notifier.OnOutcome)
	}
	svc, err := remote.New(ctx, deps)
	if err != nil {
		cancel()
		return nil, err
	}
	d.messaging.SetRemoteWake(svc.Wake)
	if err := d.settings.AddApplier(func(prev, next settings.Settings) error {
		if prev.DiscordRemoteControl == next.DiscordRemoteControl {
			return nil
		}
		reqCtx, done := context.WithTimeout(ctx, remoteRegisterTimeout)
		defer done()
		if next.DiscordRemoteControl {
			// The backend must know the launcher before the bot can reach it,
			// so a failed registration keeps the switch off.
			return client.EnableRemoteDevice(reqCtx, d.installationID, deviceName(d.installationID))
		}
		// Turning it off always works locally: nothing runs once the switch
		// is off, whether or not the backend heard about it.
		if err := client.DisableRemoteDevice(reqCtx, d.installationID); err != nil {
			slog.Warn("withdraw discord remote control", "component", "remote", "error", err)
		}
		return nil
	}); err != nil {
		cancel()
		return nil, err
	}
	// A launcher that had control on renews its registration at start: the
	// backend row goes when the account is deleted or control is withdrawn
	// elsewhere, and the device name may have changed.
	var renew sync.WaitGroup
	if d.settings.GetSettings().DiscordRemoteControl {
		renew.Go(func() {
			reqCtx, done := context.WithTimeout(ctx, remoteRegisterTimeout)
			defer done()
			if err := client.EnableRemoteDevice(reqCtx, d.installationID, deviceName(d.installationID)); err != nil {
				slog.Warn("renew discord remote control", "component", "remote", "error", err)
				return
			}
			//nolint:contextcheck // Wake runs on the service's own context, cancelled at shutdown.
			svc.Wake()
		})
	}
	return func() {
		cancel()
		waitBounded("discord remote control", shutdownWaitLimit, func() { renew.Wait(); svc.Wait(); notifier.Wait() })
	}, nil
}

func runningTitles(games []library.RecentGame) []string {
	out := []string{}
	for _, g := range games {
		if g.Running {
			out = append(out, g.Title)
		}
	}
	return out
}

// deviceName is how the launcher appears in the bot's device list. It is the
// system and the start of the installation id, not the computer's name: the
// host name would be new personal data on the server.
func deviceName(installationID string) string {
	system := map[string]string{"windows": "Windows", "darwin": "macOS", "linux": "Linux"}[runtime.GOOS]
	if system == "" {
		system = runtime.GOOS
	}
	short := installationID
	if len(short) > 4 {
		short = short[:4]
	}
	return "Typhon · " + system + " · " + short
}
