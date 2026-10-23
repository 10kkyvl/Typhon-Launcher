package main

import (
	"errors"
	"log/slog"
	"strings"
	"sync"

	"typhon/internal/jumplist"

	"github.com/wailsapp/wails/v3/pkg/application"
)

const eventNavigate = "app:navigate"

var errBadOpenTarget = errors.New("--open требует маршрут library или downloads")

// NavigationService hands the page a cold start asked for to the frontend. An
// event emitted while the window is still loading finds nobody listening, so
// the route waits here until the page comes for it.
type NavigationService struct {
	mu      sync.Mutex
	pending string
}

func (n *NavigationService) setPending(route string) {
	n.mu.Lock()
	n.pending = route
	n.mu.Unlock()
}

// TakePending returns the route once and forgets it; an empty string means the
// launcher was not started with --open.
func (n *NavigationService) TakePending() string {
	n.mu.Lock()
	defer n.mu.Unlock()
	route := n.pending
	n.pending = ""
	return route
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

// openTarget reads --open <route>, the way a Jump List task or an external
// shortcut asks the window to land on a page. Only routes the frontend knows
// are accepted: anything else is a typo, not a request.
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

func navigateFromArgs(args []string, navigate func(route string)) bool {
	route, requested, err := openTarget(args)
	if err != nil {
		slog.Error("parse open argument", "args", args, "error", err)
		return false
	}
	if !requested {
		return false
	}
	navigate(route)
	return true
}
