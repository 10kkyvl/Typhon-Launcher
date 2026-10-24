package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"

	"typhon/internal/instance"
	"typhon/internal/platform"
	"typhon/internal/settings"
)

func launcherInstanceID() (string, error) {
	if !settings.IsPortable() {
		return singleInstanceID, nil
	}
	dir, err := settings.ConfigDir()
	if err != nil {
		return "", err
	}
	return portableInstanceID(dir), nil
}

func isWorkerLaunch(args []string) bool {
	if len(args) < 2 {
		return false
	}
	switch args[1] {
	case "--install-worker", "--install-broker", "--selfupdate-worker":
		return true
	}
	return false
}

// handOffLaunch runs before logging, recovery and every service: a second
// launch that reaches those writes to the same state files as the first one.
func handOffLaunch(ctx context.Context, args []string) (bool, error) {
	if isWorkerLaunch(args) {
		return false, nil
	}
	id, err := launcherInstanceID()
	if err != nil {
		return false, fmt.Errorf("resolve single instance id: %w", err)
	}
	// The regular start repeats this call and logs its failure; when the launch is
	// handed over there is no log to write to.
	_ = platform.AllowForegroundHandoff() //nolint:errcheck // Главный принцип: ошибка не теряется, обычный старт повторяет вызов и пишет её в лог
	workDir, err := os.Getwd()
	if err != nil {
		return false, fmt.Errorf("resolve working directory: %w", err)
	}
	forwarded, err := instance.Forward(ctx, id, args, workDir)
	if err != nil {
		return false, fmt.Errorf("hand the launch to the running instance: %w", err)
	}
	return forwarded, nil
}

// secondLaunches holds launches that arrive from other processes before the
// window and the tray exist: Wails delivers them from its own goroutine as
// soon as application.New returns, long before the tray controller is built.
type secondLaunches struct {
	mu    sync.Mutex
	run   func(args []string)
	queue [][]string
}

func (s *secondLaunches) Receive(args []string) {
	s.mu.Lock()
	if s.run == nil {
		s.queue = append(s.queue, args)
		s.mu.Unlock()
		return
	}
	run := s.run
	s.mu.Unlock()
	run(args)
}

func (s *secondLaunches) Ready(run func(args []string)) {
	for {
		s.mu.Lock()
		if len(s.queue) == 0 {
			s.run = run
			s.mu.Unlock()
			return
		}
		args := s.queue[0]
		s.queue = s.queue[1:]
		s.mu.Unlock()
		run(args)
	}
}

type windowOpener interface {
	Open()
}

func handleSecondLaunch(args []string, player gamePlayer, window windowOpener, nav *NavigationService, emit func(route string)) {
	req, requested, err := playTarget(args)
	if err != nil {
		slog.Error("parse play argument", "args", args, "error", err)
		window.Open()
		return
	}
	if !requested {
		window.Open()
		navigateFromArgs(args, nav, emit)
		return
	}
	playFromShortcut(player, req, window.Open)
}
