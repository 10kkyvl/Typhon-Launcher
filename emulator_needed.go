package main

import (
	"log/slog"
	"slices"
	"sync"

	"typhon/internal/emulators"
	"typhon/internal/library"
	"typhon/internal/uierr"

	"github.com/wailsapp/wails/v3/pkg/application"
)

const eventEmulatorsNeeded = "emulators:needed"

// EmulatorNeeded asks the page to let the user pick an emulator. Code is the
// full code of the launch error, such as emulators.not_configured. An empty
// GameID means nothing is waiting.
type EmulatorNeeded struct {
	GameID   string `json:"gameId"`
	Platform string `json:"platform"`
	Code     string `json:"code"`
}

var emulatorPromptCodes = []string{
	uierr.Code(emulators.ErrNotConfigured),
	uierr.Code(emulators.ErrFileMissing),
	uierr.Code(emulators.ErrWindowsProgram),
	uierr.Code(emulators.ErrNotExe),
	uierr.Code(emulators.ErrNotRegularFile),
	uierr.Code(emulators.ErrFileUnreadable),
	uierr.Code(emulators.ErrPathRelative),
}

func emulatorPromptCode(err error) (string, bool) {
	code := uierr.Code(err)
	return code, slices.Contains(emulatorPromptCodes, code)
}

// noteEmulatorNeeded reports a launch that only lacks a working emulator: an
// expected state, not a failure of the launcher. False leaves err to the caller.
func noteEmulatorNeeded(id string, err error) bool {
	code, ok := emulatorPromptCode(err)
	if ok {
		slog.Warn("play needs an emulator", "id", id, "code", code)
	}
	return ok
}

type gameFinder interface {
	Find(id string) (library.Game, error)
}

// EmulatorPromptService carries the prompt from launches the page did not start.
// An event emitted while the page is still loading finds no listener, so the
// prompt also waits here until the page takes it, as NavigationService does for
// routes. A newer prompt replaces an older one.
type EmulatorPromptService struct {
	games gameFinder
	emit  func(EmulatorNeeded)

	mu      sync.Mutex
	pending EmulatorNeeded
}

func newEmulatorPromptService(games gameFinder, emit func(EmulatorNeeded)) *EmulatorPromptService {
	return &EmulatorPromptService{games: games, emit: emit}
}

func (p *EmulatorPromptService) TakePending() EmulatorNeeded {
	p.mu.Lock()
	defer p.mu.Unlock()
	needed := p.pending
	p.pending = EmulatorNeeded{}
	return needed
}

// offer leaves the launch error to its caller either way: a game that cannot be
// asked about still reveals the window.
func (p *EmulatorPromptService) offer(id string, err error) {
	code, ok := emulatorPromptCode(err)
	if !ok {
		return
	}
	game, findErr := p.games.Find(id)
	if findErr != nil {
		slog.Warn("find the game that needs an emulator", "id", id, "error", findErr)
		return
	}
	if game.Emulation == nil {
		slog.Warn("emulator error for a game without emulation", "id", id, "code", code)
		return
	}
	needed := EmulatorNeeded{GameID: id, Platform: game.Emulation.Platform, Code: code}
	p.mu.Lock()
	p.pending = needed
	p.mu.Unlock()
	p.emit(needed)
}

func emitEmulatorNeeded(wails *application.App, needed EmulatorNeeded) {
	if wails == nil {
		return
	}
	wails.Event.Emit(eventEmulatorsNeeded, needed)
}

type launcherPlayer interface {
	trayPlayer
	gamePlayer
}

// promptingPlayer stands in for the library wherever a game starts outside the
// page: the tray, shortcuts, Steam entries and the jump list.
type promptingPlayer struct {
	player  launcherPlayer
	prompts *EmulatorPromptService
}

func (p promptingPlayer) PlayGame(id string) error {
	err := p.player.PlayGame(id)
	p.prompts.offer(id, err)
	return err
}

func (p promptingPlayer) PlayGameProfile(id, profileID string) error {
	err := p.player.PlayGameProfile(id, profileID)
	p.prompts.offer(id, err)
	return err
}
