package remote

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"

	"typhon/internal/account"
	"typhon/internal/download"
)

// Notification kinds, mirrored by the backend's whitelist.
const (
	KindDownloadCompleted = "download_completed"
	KindDownloadFailed    = "download_failed"
	KindAddStarted        = "add_started"
	KindAddFailed         = "add_failed"
)

const (
	notifyTimeout = 15 * time.Second
	// maxNotifyInFlight drops messages beyond it rather than queueing: a
	// burst means something is wrong, and the backend caps the hour anyway.
	maxNotifyInFlight = 4
	maxNotifyTitle    = 200
)

// NotifyAPI is the backend side of notifications.
type NotifyAPI interface {
	PostRemoteNotification(ctx context.Context, n account.RemoteNotification) error
}

// NotifierDeps wires a Notifier.
type NotifierDeps struct {
	API            NotifyAPI
	InstallationID string
	// Enabled reads "Notify me in Discord" when a message would be sent.
	Enabled func() bool
	Log     *slog.Logger
}

// Notifier asks the Typhon bot to message the owner's Discord account. It
// never blocks the caller and never fails it: a message that cannot be sent
// is logged and dropped.
type Notifier struct {
	d      NotifierDeps
	ctx    context.Context
	mu     sync.Mutex
	flying int
	wg     sync.WaitGroup
}

func NewNotifier(ctx context.Context, d NotifierDeps) (*Notifier, error) {
	if d.API == nil || d.InstallationID == "" || d.Enabled == nil {
		return nil, errors.New("remote: missing notifier dependencies")
	}
	if d.Log == nil {
		d.Log = slog.Default()
	}
	return &Notifier{d: d, ctx: ctx}, nil
}

// Send queues one message in the background.
func (n *Notifier) Send(kind, title, code string) {
	if !n.d.Enabled() {
		return
	}
	title = cleanTitle(title)
	if title == "" {
		return
	}
	n.mu.Lock()
	if n.ctx.Err() != nil || n.flying >= maxNotifyInFlight {
		n.mu.Unlock()
		n.d.Log.Warn("discord notification dropped", "component", "remote", "kind", kind)
		return
	}
	n.flying++
	n.wg.Add(1)
	n.mu.Unlock()
	go func() {
		defer func() {
			n.mu.Lock()
			n.flying--
			n.mu.Unlock()
			n.wg.Done()
		}()
		ctx, cancel := context.WithTimeout(n.ctx, notifyTimeout)
		defer cancel()
		err := n.d.API.PostRemoteNotification(ctx, account.RemoteNotification{
			InstallationID: n.d.InstallationID, Kind: kind, Title: title, Code: code,
		})
		if err != nil {
			n.d.Log.Warn("send discord notification", "component", "remote", "kind", kind, "error", err)
		}
	}()
}

// OnOutcome reports a finished or failed download the person started for a
// game. Updates and repairs are the launcher's own work and stay quiet.
func (n *Notifier) OnOutcome(d download.Download) {
	if d.Origin.Purpose != download.PurposeRelease {
		return
	}
	title := d.Origin.Title
	if title == "" {
		title = d.Name
	}
	switch d.Status {
	case download.StatusCompleted:
		n.Send(KindDownloadCompleted, title, "")
	case download.StatusFailed:
		n.Send(KindDownloadFailed, title, "")
	}
}

// Wait returns once no message is in flight; for shutdown.
func (n *Notifier) Wait() { n.wg.Wait() }

// cleanTitle keeps a title the backend accepts: one line of printable text.
func cleanTitle(s string) string {
	s = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return ' '
		}
		return r
	}, s)
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > maxNotifyTitle {
		s = string(r[:maxNotifyTitle-1]) + "…"
	}
	return s
}
