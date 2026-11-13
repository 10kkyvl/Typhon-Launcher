package main

import (
	"log/slog"
	"slices"
	"sync"

	"typhon/internal/cloudsaves"
	"typhon/internal/emulators"
	"typhon/internal/library"
	"typhon/internal/uierr"

	"github.com/wailsapp/wails/v3/pkg/application"
)

const eventLaunchPending = "launch:pending"

// PendingLaunch is a launch the page did not start that waits for the
// player: an emulator to pick (Platform is set) or cloud saves to settle.
// Code is the full code of the launch error, such as emulators.not_configured
// or cloud.conflict_unresolved. ProfileLaunch is a launch through
// PlayGameProfile, where an empty ProfileID is the game's own executable and
// not the profile the game used last. An empty GameID means nothing is waiting.
type PendingLaunch struct {
	GameID   string `json:"gameId"`
	Platform string `json:"platform"`
	Code     string `json:"code"`

	ProfileID     string `json:"profileId,omitempty"`
	ProfileLaunch bool   `json:"profileLaunch,omitempty"`
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

var savesHoldCodes = []string{
	uierr.Code(cloudsaves.ErrConflictUnresolved),
	uierr.Code(cloudsaves.ErrSyncing),
}

func emulatorPromptCode(err error) (string, bool) {
	code := uierr.Code(err)
	return code, slices.Contains(emulatorPromptCodes, code)
}

func savesHoldCode(err error) (string, bool) {
	code := uierr.Code(err)
	return code, slices.Contains(savesHoldCodes, code)
}

// noteLaunchHeld reports a launch held back for the player, by a missing
// emulator or by the cloud saves: an expected state, not a failure of the
// launcher. False leaves err to the caller.
func noteLaunchHeld(id string, err error) bool {
	if code, ok := emulatorPromptCode(err); ok {
		slog.Warn("play needs an emulator", "id", id, "code", code)
		return true
	}
	if code, ok := savesHoldCode(err); ok {
		slog.Warn("play held back by cloud saves", "id", id, "code", code)
		return true
	}
	return false
}

type gameFinder interface {
	Find(id string) (library.Game, error)
}

// PendingLaunchService carries launches the page did not start and the
// player has to finish in the window: an emulator prompt or a launch the
// cloud saves held back. An event emitted while the page is still loading
// finds no listener, so the launch also waits here until the page takes it, as
// NavigationService does for routes. A newer launch replaces an older one.
type PendingLaunchService struct {
	games gameFinder
	emit  func(PendingLaunch)

	mu      sync.Mutex
	pending PendingLaunch
}

func newPendingLaunchService(games gameFinder, emit func(PendingLaunch)) *PendingLaunchService {
	return &PendingLaunchService{games: games, emit: emit}
}

func (p *PendingLaunchService) TakePending() PendingLaunch {
	p.mu.Lock()
	defer p.mu.Unlock()
	needed := p.pending
	p.pending = PendingLaunch{}
	return needed
}

// offer and offerProfile leave the launch error to their caller either way: a
// game that cannot be asked about still reveals the window.
func (p *PendingLaunchService) offer(id string, err error) {
	p.hold(PendingLaunch{GameID: id}, err)
}

func (p *PendingLaunchService) offerProfile(id, profileID string, err error) {
	p.hold(PendingLaunch{GameID: id, ProfileID: profileID, ProfileLaunch: true}, err)
}

func (p *PendingLaunchService) hold(launch PendingLaunch, err error) {
	emulatorCode, emulator := emulatorPromptCode(err)
	savesCode, saves := savesHoldCode(err)
	if !emulator && !saves {
		return
	}
	game, findErr := p.games.Find(launch.GameID)
	if findErr != nil {
		slog.Warn("find the game of a held-back launch", "id", launch.GameID, "error", findErr)
		return
	}
	if emulator {
		if game.Emulation == nil {
			slog.Warn("emulator error for a game without emulation", "id", launch.GameID, "code", emulatorCode)
			return
		}
		// The page resumes an emulator prompt with the game's own launch, as
		// the Play button does, so the profile is not carried.
		launch = PendingLaunch{GameID: launch.GameID, Platform: game.Emulation.Platform, Code: emulatorCode}
	} else {
		launch.Code = savesCode
	}
	p.mu.Lock()
	p.pending = launch
	p.mu.Unlock()
	p.emit(launch)
}

func emitLaunchPending(wails *application.App, needed PendingLaunch) {
	if wails == nil {
		return
	}
	wails.Event.Emit(eventLaunchPending, needed)
}

type launcherPlayer interface {
	trayPlayer
	gamePlayer
}

// promptingPlayer stands in for the library wherever a game starts outside the
// page: the tray, shortcuts, Steam entries and the jump list. The page resumes
// a held-back launch through the bound library itself, so a resume held back
// again is answered in the window and does not come back here.
type promptingPlayer struct {
	player  launcherPlayer
	prompts *PendingLaunchService
}

func (p promptingPlayer) PlayGame(id string) error {
	err := p.player.PlayGame(id)
	p.prompts.offer(id, err)
	return err
}

func (p promptingPlayer) PlayGameProfile(id, profileID string) error {
	err := p.player.PlayGameProfile(id, profileID)
	p.prompts.offerProfile(id, profileID, err)
	return err
}
