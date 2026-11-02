package main

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"typhon/internal/deeplink"
	"typhon/internal/jumplist"
	"typhon/internal/protocolreg"

	"github.com/wailsapp/wails/v3/pkg/application"
)

const eventNavigate = "app:navigate"

var errBadOpenTarget = errors.New("--open требует маршрут library или downloads")

var errLinkWithArguments = errors.New("ссылка typhon:// должна быть единственным аргументом запуска")

// PendingLink is what a typhon:// launch left for the page. Provider and Query
// are empty and Invalid false when there is nothing to show. A Query never goes
// through Resolve: the page only searches the catalog for it.
type PendingLink struct {
	Provider string `json:"provider"`
	ID       string `json:"id"`
	Query    string `json:"query"`
	Invalid  bool   `json:"invalid"`
}

// An event emitted while the page is still loading finds no listener, so the
// request also waits here until the page takes it. A launch asks for one thing:
// a newer route replaces an older link and the other way round.
type NavigationService struct {
	mu      sync.Mutex
	pending string
	link    PendingLink
}

func (n *NavigationService) setPending(route string) {
	n.mu.Lock()
	n.pending = route
	n.link = PendingLink{}
	n.mu.Unlock()
}

func (n *NavigationService) setPendingLink(link PendingLink) {
	n.mu.Lock()
	n.link = link
	n.pending = ""
	n.mu.Unlock()
}

func (n *NavigationService) TakePending() string {
	n.mu.Lock()
	defer n.mu.Unlock()
	route := n.pending
	n.pending = ""
	return route
}

func (n *NavigationService) TakePendingLink() PendingLink {
	n.mu.Lock()
	defer n.mu.Unlock()
	link := n.link
	n.link = PendingLink{}
	return link
}

type navigateEvent struct {
	Route string `json:"route"`
}

func emitNavigate(wails *application.App, route string) {
	if wails == nil {
		return
	}
	wails.Event.Emit(eventNavigate, navigateEvent{Route: route})
}

func openTarget(args []string) (string, bool, error) {
	for i := 1; i < len(args); i++ {
		if args[i] != "--open" {
			continue
		}
		if i+1 >= len(args) {
			return "", true, errBadOpenTarget
		}
		route := strings.TrimSpace(args[i+1])
		switch route {
		case jumplist.RouteLibrary, jumplist.RouteDownloads:
			return route, true, nil
		}
		return "", true, errBadOpenTarget
	}
	return "", false, nil
}

// linkTarget treats a launch that mentions a link as a link launch and nothing
// else. The shell starts `"typhon.exe" "%1"`, so a quote inside a URL that a
// caller did not escape splits it into extra arguments: `--play <id>` smuggled
// that way must not start a game. Only the link alone is accepted.
func linkTarget(args []string) (deeplink.Link, bool, error) {
	if len(args) < 2 {
		return deeplink.Link{}, false, nil
	}
	mentioned := false
	for _, arg := range args[1:] {
		if deeplink.IsLink(arg) {
			mentioned = true
			break
		}
	}
	if !mentioned {
		return deeplink.Link{}, false, nil
	}
	if len(args) != 2 {
		return deeplink.Link{}, true, errLinkWithArguments
	}
	link, err := deeplink.Parse(args[1])
	if err != nil {
		return deeplink.Link{}, true, fmt.Errorf("parse typhon link: %w", err)
	}
	return link, true, nil
}

func pendingLink(link deeplink.Link, err error) PendingLink {
	if err != nil {
		slog.Warn("reject typhon link", "component", "app", "operation", "open_link", "error", err)
		return PendingLink{Invalid: true}
	}
	return PendingLink{Provider: link.Target.Provider, ID: link.Target.ID, Query: link.Query}
}

// firstLaunch settles what a cold start was asked to do. Parse failures do not
// stop the launcher: the window opens as usual, which is strictly better than
// refusing to start over a broken shortcut.
func firstLaunch(args []string, nav *NavigationService) (playRequest, bool) {
	link, linked, err := linkTarget(args)
	if linked {
		nav.setPendingLink(pendingLink(link, err))
		return playRequest{}, false
	}
	req, playRequested, err := playTarget(args)
	if err != nil {
		slog.Error("parse play argument", "args", args[1:], "error", err)
		playRequested = false
	}
	if playRequested {
		return req, true
	}
	route, openRequested, err := openTarget(args)
	if err != nil {
		slog.Error("parse open argument", "args", args[1:], "error", err)
		return playRequest{}, false
	}
	if openRequested {
		nav.setPending(route)
	}
	return playRequest{}, false
}

func navigateFromArgs(args []string, nav *NavigationService, emit func(route string)) bool {
	route, requested, err := openTarget(args)
	if err != nil {
		slog.Error("parse open argument", "args", args, "error", err)
		return false
	}
	if !requested {
		return false
	}
	nav.setPending(route)
	emit(route)
	return true
}

// Wails raises its URL event on Windows and Linux too, from the same os.Args the
// launcher has already read, so there the event would open the link twice. Only
// macOS delivers a URL outside argv.
func urlEventCarriesLink(goos string) bool {
	return goos == "darwin"
}

// launchFromURL turns a URL event into the arguments a second launch would have
// brought, so it takes the same queue and the same checks.
func launchFromURL(url string) []string {
	return []string{"typhon", url}
}

type protocolOwner struct {
	production bool
	portable   bool
	qa         bool
}

// The class is per user and shared by every copy of the launcher, so only the
// copy the installer put in place may hold it: a build in bin\, a go run, an
// unpacked exe or a portable copy would take links away from the installed one.
func registerProtocol(owner protocolOwner, executable func() (string, error), installDir func() (string, bool, error), register func(string) error) error {
	if !owner.production || owner.portable || owner.qa {
		return nil
	}
	dir, recorded, err := installDir()
	if err != nil {
		return err
	}
	if !recorded {
		return nil
	}
	exe, err := executable()
	if err != nil {
		return fmt.Errorf("locate launcher executable: %w", err)
	}
	installed, err := protocolreg.RunsFrom(exe, dir)
	if err != nil {
		return err
	}
	if !installed {
		return nil
	}
	return register(exe)
}
