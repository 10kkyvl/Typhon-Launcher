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

// An event emitted while the page is still loading finds no listener, so the
// route also waits here until the page takes it.
type NavigationService struct {
	mu      sync.Mutex
	pending string
}

func (n *NavigationService) setPending(route string) {
	n.mu.Lock()
	n.pending = route
	n.mu.Unlock()
}

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
