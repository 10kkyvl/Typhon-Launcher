package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"runtime/debug"
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

// Runs before logging, recovery and the services: past that point a second
// launch writes to the first one's state files.
func handOffLaunch(ctx context.Context, args []string) (bool, error) {
	if isWorkerLaunch(args) {
		return false, nil
	}
	id, err := launcherInstanceID()
	if err != nil {
		return false, fmt.Errorf("resolve single instance id: %w", err)
	}
	// The regular start repeats the call and logs its failure.
	_ = platform.AllowForegroundHandoff() //nolint:errcheck // Главный принцип: ошибка не теряется, обычный старт повторяет вызов и пишет её в лог
	// Wails sends an empty directory when this fails, and so does the hand-off.
	workDir, _ := os.Getwd() //nolint:errcheck // Главный принцип: пустой каталог — то, что отправляет и сам Wails; запуск от этого не отменяется
	forwarded, err := instance.Forward(ctx, id, args, workDir)
	if err != nil {
		return false, fmt.Errorf("hand the launch to the running instance: %w", err)
	}
	return forwarded, nil
}

// Wails delivers second launches from its own goroutine as soon as
// application.New returns, long before the tray controller exists.
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
		runRecovered(run, args)
	}
}

func runRecovered(run func(args []string), args []string) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("second launch handler panicked", "args", args, "panic", r, "stack", string(debug.Stack()))
		}
	}()
	run(args)
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
