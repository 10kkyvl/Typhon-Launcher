package library

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"typhon/internal/steamlink"
	"typhon/internal/uierr"
)

var (
	errSteamExternal   = uierr.New("steam.already_in_steam", "игра уже в Steam")
	errSteamNotStarted = errors.New("служба библиотеки не запущена")
)

type steamBackend interface {
	Supported() bool
	Add(ctx context.Context, sc steamlink.Shortcut, art steamlink.Art, plan steamlink.Sync) error
	Remove(ctx context.Context, appID uint32, plan steamlink.Sync) error
	Reconcile(ctx context.Context, update []steamlink.Shortcut, drop []uint32) error
	Contains(ctx context.Context, appID uint32) (bool, error)
}

type systemSteam struct{}

func (systemSteam) Supported() bool { return steamlink.Supported() }

func (systemSteam) Add(ctx context.Context, sc steamlink.Shortcut, art steamlink.Art, plan steamlink.Sync) error {
	return steamlink.Add(ctx, sc, art, plan)
}

func (systemSteam) Remove(ctx context.Context, appID uint32, plan steamlink.Sync) error {
	return steamlink.Remove(ctx, appID, plan)
}

func (systemSteam) Reconcile(ctx context.Context, update []steamlink.Shortcut, drop []uint32) error {
	return steamlink.Reconcile(ctx, update, drop)
}

func (systemSteam) Contains(ctx context.Context, appID uint32) (bool, error) {
	return steamlink.Contains(ctx, appID)
}

// disabledSteam is what every service starts with. Only the application
// switches to the real Steam (EnableSteam), so a service built by a test, a
// tool or another package never reads or writes the Steam of the machine.
type disabledSteam struct{}

func (disabledSteam) Supported() bool { return false }

func (disabledSteam) Add(context.Context, steamlink.Shortcut, steamlink.Art, steamlink.Sync) error {
	return steamlink.ErrUnsupported
}

func (disabledSteam) Remove(context.Context, uint32, steamlink.Sync) error {
	return steamlink.ErrUnsupported
}

func (disabledSteam) Reconcile(context.Context, []steamlink.Shortcut, []uint32) error {
	return steamlink.ErrUnsupported
}

func (disabledSteam) Contains(context.Context, uint32) (bool, error) {
	return false, steamlink.ErrUnsupported
}

type SteamStatus struct {
	Supported bool `json:"supported"`
	Installed bool `json:"installed"`
	Added     bool `json:"added"`
}

// EnableSteam connects the service to the real Steam and lets ServiceStartup
// reconcile its shortcuts with the library.
//
//wails:ignore
func (s *Service) EnableSteam() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.steam = systemSteam{}
	s.steamSync = true
}

// SetSteamArt wires the lookup of local cover files for a catalog game.
//
//wails:ignore
func (s *Service) SetSteamArt(fn func(canonicalID string) (steamlink.Art, error)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.steamArt = fn
}

// steamTarget is what Steam has to start for a game: the game itself, or the
// launcher with --play when the game needs administrator rights.
func steamTarget(game Game, launcherPath func() (string, error)) (steamlink.Shortcut, error) {
	if game.Executable == "" {
		return steamlink.Shortcut{}, errShortcutNoExecutable
	}
	sc := steamlink.Shortcut{
		AppID: steamlink.AppID(game.ID),
		Name:  game.Title,
		Icon:  game.Executable,
	}
	if game.RunAsAdmin {
		if !safeCommandLineID(game.ID) {
			return steamlink.Shortcut{}, fmt.Errorf("%w: %q", errShortcutBadID, game.ID)
		}
		launcher, err := launcherPath()
		if err != nil {
			return steamlink.Shortcut{}, fmt.Errorf("путь к лаунчеру: %w", err)
		}
		sc.Exe = launcher
		sc.StartDir = filepath.Dir(launcher)
		sc.LaunchOptions = "--play " + game.ID
		return sc, nil
	}
	workDir, err := filepath.Abs(filepath.Dir(game.Executable))
	if err != nil {
		return steamlink.Shortcut{}, fmt.Errorf("рабочая папка игры: %w", err)
	}
	sc.Exe = game.Executable
	sc.StartDir = workDir
	sc.LaunchOptions = steamlink.JoinArgs(game.LaunchArgs)
	return sc, nil
}

// steamState is the part of the library that decides which Steam shortcuts are
// stale. It is copied under the mutex and evaluated outside it.
type steamState struct {
	live         []Game
	liveIDs      map[string]struct{}
	goneIDs      []string
	launcherPath func() (string, error)
}

func (s *Service) steamStateLocked() steamState {
	st := steamState{liveIDs: map[string]struct{}{}, launcherPath: s.launcherPath}
	for _, g := range s.games {
		if g.Uninstalled {
			st.goneIDs = append(st.goneIDs, g.ID)
			continue
		}
		st.live = append(st.live, g)
		st.liveIDs[g.ID] = struct{}{}
	}
	for _, g := range s.archived {
		st.goneIDs = append(st.goneIDs, g.ID)
	}
	return st
}

// plan lists the shortcuts to refresh (installed games whose target can be
// built) and the app ids to drop (archived and uninstalled games that are not
// installed under the same id).
func (st steamState) plan() steamlink.Sync {
	var plan steamlink.Sync
	for _, g := range st.live {
		if g.External != nil && g.External.Provider == ProviderSteam {
			continue
		}
		sc, err := steamTarget(g, st.launcherPath)
		if err != nil {
			continue
		}
		if err := sc.Validate(); err != nil {
			slog.Warn("steam shortcut skipped", "id", g.ID, "error", err)
			continue
		}
		plan.Update = append(plan.Update, sc)
	}
	seen := map[uint32]struct{}{}
	for _, id := range st.goneIDs {
		if _, live := st.liveIDs[id]; live {
			continue
		}
		appID := steamlink.AppID(id)
		if _, dup := seen[appID]; dup {
			continue
		}
		seen[appID] = struct{}{}
		plan.Drop = append(plan.Drop, appID)
	}
	return plan
}

func (s *Service) AddToSteam(id string) error {
	if !s.steam.Supported() {
		return steamlink.ErrUnsupported
	}
	s.mu.Lock()
	found := s.findLocked(id)
	if found == nil {
		s.mu.Unlock()
		return errNotFound
	}
	game := *found
	ctx := s.ctx
	artFor := s.steamArt
	state := s.steamStateLocked()
	s.mu.Unlock()

	if game.Uninstalled {
		return errShortcutUninstalled
	}
	if game.External != nil && game.External.Provider == ProviderSteam {
		return errSteamExternal
	}
	if game.Executable == "" {
		return errShortcutNoExecutable
	}
	if ctx == nil {
		return errSteamNotStarted
	}
	if _, err := os.Stat(game.Executable); err != nil {
		return fmt.Errorf("исполняемый файл игры: %w", err)
	}
	sc, err := steamTarget(game, state.launcherPath)
	if err != nil {
		return err
	}

	var art steamlink.Art
	if game.CanonicalGameID != "" && artFor != nil {
		art, err = artFor(game.CanonicalGameID)
		if err != nil {
			return fmt.Errorf("обложка для Steam: %w", err)
		}
	}
	if err := s.steam.Add(ctx, sc, art, state.plan()); err != nil {
		return err
	}
	slog.Info("game added to steam", "id", id, "title", game.Title, "exe", sc.Exe)
	return nil
}

func (s *Service) RemoveFromSteam(id string) error {
	if !s.steam.Supported() {
		return steamlink.ErrUnsupported
	}
	s.mu.Lock()
	ctx := s.ctx
	state := s.steamStateLocked()
	s.mu.Unlock()
	if ctx == nil {
		return errSteamNotStarted
	}
	if err := s.steam.Remove(ctx, steamlink.AppID(id), state.plan()); err != nil {
		return err
	}
	slog.Info("game removed from steam", "id", id)
	return nil
}

func (s *Service) SteamStatus(id string) (SteamStatus, error) {
	status := SteamStatus{Supported: s.steam.Supported()}
	if !status.Supported {
		return status, nil
	}
	s.mu.Lock()
	ctx := s.ctx
	s.mu.Unlock()
	if ctx == nil {
		return SteamStatus{}, errSteamNotStarted
	}
	added, err := s.steam.Contains(ctx, steamlink.AppID(id))
	if errors.Is(err, steamlink.ErrNotInstalled) {
		return status, nil
	}
	if err != nil {
		return SteamStatus{}, err
	}
	status.Installed = true
	status.Added = added
	return status, nil
}

// dropSteam removes the Steam shortcut of a game that has just been removed or
// uninstalled. The game is already gone and saved, so a Steam that is running
// or unreadable must not turn that into a failure; the shortcut stays and the
// next Steam operation or the startup reconcile removes it.
func (s *Service) dropSteam(ctx context.Context, id string) {
	if !s.steam.Supported() {
		return
	}
	if ctx == nil {
		slog.Warn("steam shortcut left behind: library not started", "id", id)
		return
	}
	logSteamReconcile("drop", id, s.steam.Reconcile(ctx, nil, []uint32{steamlink.AppID(id)}))
}

// reconcileSteam brings the shortcuts in Steam in line with the library:
// stale ones go, existing ones follow their game. Failure only leaves them as
// they were, so it is logged and never returned.
func (s *Service) reconcileSteam(ctx context.Context, trigger string) {
	if !s.steam.Supported() {
		return
	}
	if ctx == nil {
		slog.Warn("steam shortcuts not reconciled: library not started", "trigger", trigger)
		return
	}
	s.mu.Lock()
	state := s.steamStateLocked()
	s.mu.Unlock()
	plan := state.plan()
	logSteamReconcile(trigger, "", s.steam.Reconcile(ctx, plan.Update, plan.Drop))
}

func logSteamReconcile(trigger, id string, err error) {
	switch {
	case err == nil:
	case errors.Is(err, steamlink.ErrNotInstalled), errors.Is(err, steamlink.ErrUnsupported):
	case errors.Is(err, steamlink.ErrSteamRunning):
		slog.Info("steam shortcuts not reconciled: Steam is running", "trigger", trigger, "id", id)
	default:
		slog.Warn("reconcile steam shortcuts", "trigger", trigger, "id", id, "error", err)
	}
}

// startSteamSync runs one reconcile in the background when the application has
// enabled Steam. The goroutine is counted in wg, so ServiceShutdown waits for
// it, and it stops with ctx.
func (s *Service) startSteamSync(ctx context.Context) {
	s.mu.Lock()
	enabled := s.steamSync && !s.closed
	if enabled {
		s.wg.Add(1)
	}
	s.mu.Unlock()
	if !enabled {
		return
	}
	go func() {
		defer s.wg.Done()
		s.reconcileSteam(ctx, "startup")
	}()
}
