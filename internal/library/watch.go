package library

import (
	"context"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"typhon/internal/procs"

	"github.com/wailsapp/wails/v3/pkg/application"
)

const defaultWatchInterval = 5 * time.Second

// ServiceStartup starts the OS process detector that keeps a game's session
// alive as long as its process runs, independently of who launched it or
// whether the launcher was restarted in the meantime. On a platform where
// procs.List is not supported (invariant: no OS default on failure — here
// the honest answer is "detection unavailable", not "nobody is playing"),
// the detector simply never runs and watching stays false.
func (s *Service) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	if !procs.Supported() {
		s.mu.Lock()
		s.ctx = ctx
		s.mu.Unlock()
		return nil
	}

	runCtx, cancel := context.WithCancel(ctx)
	s.mu.Lock()
	s.ctx = runCtx
	s.cancel = cancel
	s.watching = true
	s.mu.Unlock()

	s.wg.Add(1)
	go s.loop(runCtx)
	return nil
}

func (s *Service) loop(ctx context.Context) {
	defer s.wg.Done()
	s.detectTick(ctx)
	ticker := time.NewTicker(s.watchInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.detectTick(ctx)
		}
	}
}

type matchTarget struct {
	id         string
	installDir string
	executable string
}

// detectTick reconciles s.running against a fresh process snapshot. A scan
// error leaves s.running untouched: a failed enumeration is not evidence
// that nobody is playing, and treating it as an empty list would open or
// close sessions on bad information.
func (s *Service) detectTick(ctx context.Context) {
	list, err := s.scan(ctx)
	if err != nil {
		slog.Warn("scan running processes", "error", err)
		return
	}

	s.mu.Lock()
	targets := make([]matchTarget, 0, len(s.games))
	for i := range s.games {
		g := &s.games[i]
		if g.Uninstalled {
			continue
		}
		if g.InstallDir == "" && g.Executable == "" {
			continue
		}
		targets = append(targets, matchTarget{id: g.ID, installDir: g.InstallDir, executable: g.Executable})
	}
	s.mu.Unlock()

	matches := matchProcesses(list, targets)
	byPID := make(map[uint32]procs.Process, len(list))
	for _, p := range list {
		byPID[p.PID] = p
	}
	now := s.now()

	type opened struct {
		id   string
		proc procs.Process
	}
	var toOpen []opened
	var toClose []string

	s.mu.Lock()
	for id, proc := range matches {
		if sess, ok := s.running[id]; ok {
			sess.lastSeen = now
			// Сессия, запущенная лаунчером, своего createdAt не знает:
			// без него сверка личности при переиспользовании pid
			// работать не может, поэтому он забирается с первого же
			// снимка, где этот pid виден.
			if sess.createdAt.IsZero() && sess.pid != 0 {
				if own, ok := byPID[sess.pid]; ok && !own.CreatedAtUnknown {
					sess.createdAt = own.CreatedAt
				}
			}
			continue
		}
		toOpen = append(toOpen, opened{id: id, proc: proc})
	}
	for id, sess := range s.running {
		if _, ok := matches[id]; ok {
			continue
		}
		// A session's own pid still being in the process table is evidence
		// the game is running even when its path can no longer be matched
		// this tick (permission failure, anti-cheat, elevation): PathUnknown
		// is not proof the game exited, only that we cannot re-verify it by
		// path right now. Without this, a session whose process never
		// becomes path-matchable would be closed by the grace period below
		// on every tick, discarding the exact cmd.Wait() signal that a
		// launcher-started session would otherwise still be waiting on.
		if proc, ok := byPID[sess.pid]; sess.pid != 0 && ok && sameProcessIdentity(sess, proc) {
			sess.lastSeen = now
			if sess.createdAt.IsZero() && !proc.CreatedAtUnknown {
				sess.createdAt = proc.CreatedAt
			}
			continue
		}
		if now.Sub(sess.lastSeen) > 2*s.watchInterval {
			toClose = append(toClose, id)
		}
	}

	var startedGames []Game
	for _, o := range toOpen {
		game := s.findLocked(o.id)
		if game == nil {
			continue
		}
		startedAt := now
		if !o.proc.CreatedAtUnknown {
			startedAt = o.proc.CreatedAt
		} else {
			slog.Debug("detected game process start time unknown, session playtime will undercount", "id", o.id)
		}
		s.running[o.id] = &session{
			pid:       o.proc.PID,
			createdAt: o.proc.CreatedAt,
			startedAt: startedAt,
			lastSeen:  now,
			external:  true,
		}
		startedGames = append(startedGames, *game)
	}
	var watchersSnap []SessionWatcher
	if len(startedGames) > 0 {
		watchersSnap = s.watchers
	}
	closeStarts := make(map[string]time.Time, len(toClose))
	for _, id := range toClose {
		closeStarts[id] = s.running[id].startedAt
	}
	s.mu.Unlock()

	for _, game := range startedGames {
		for _, w := range watchersSnap {
			w.SessionStarted(game)
		}
		emit("game:started", SessionEvent{GameID: game.ID})
		slog.Info("game session detected", "id", game.ID, "title", game.Title)
	}
	for _, id := range toClose {
		s.finishSession(id, closeStarts[id])
	}
}

// sameProcessIdentity reports whether proc is plausibly still the process a
// session recorded. A known creation time on both sides must agree, guarding
// against a recycled pid being mistaken for the original process; when
// either side's creation time is unknown (freshly launched session that
// hasn't observed its own createdAt yet, or a proc whose CreatedAtUnknown is
// set), the pid's mere presence is the only signal available and is treated
// as identity holding.
func sameProcessIdentity(sess *session, proc procs.Process) bool {
	if sess.createdAt.IsZero() || proc.CreatedAtUnknown {
		return true
	}
	return proc.CreatedAt.Equal(sess.createdAt)
}

func matchProcesses(list []procs.Process, targets []matchTarget) map[string]procs.Process {
	matches := make(map[string]procs.Process, len(targets))
	for _, p := range list {
		// По имени файла сопоставлять нельзя (Game.exe есть у сотни игр);
		// процесс с неизвестным путём просто не открывает сессию, но это
		// не значит, что игра не запущена — см. порог закрытия в detectTick.
		if p.PathUnknown {
			continue
		}
		if isUninstallerPath(p.Path) {
			continue
		}
		id := bestMatchTarget(p.Path, targets)
		if id == "" {
			continue
		}
		if _, ok := matches[id]; ok {
			continue
		}
		matches[id] = p
	}
	return matches
}

func bestMatchTarget(path string, targets []matchTarget) string {
	for _, tg := range targets {
		if tg.executable != "" && pathEqualFold(path, tg.executable) {
			return tg.id
		}
	}
	bestID := ""
	bestLen := -1
	for _, tg := range targets {
		if tg.installDir == "" {
			continue
		}
		if !pathWithinFold(path, tg.installDir) {
			continue
		}
		if dirLen := len(filepath.Clean(tg.installDir)); dirLen > bestLen {
			bestLen = dirLen
			bestID = tg.id
		}
	}
	return bestID
}

func pathEqualFold(a, b string) bool {
	return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}

// pathWithinFold reports whether path lies inside dir, comparing case
// insensitively and by path component: C:\Games\Foo2 is not within
// C:\Games\Foo.
func pathWithinFold(path, dir string) bool {
	path = filepath.Clean(path)
	dir = filepath.Clean(dir)
	if strings.EqualFold(path, dir) {
		return true
	}
	if len(path) <= len(dir) || !strings.EqualFold(path[:len(dir)], dir) {
		return false
	}
	return path[len(dir)] == filepath.Separator
}

// isUninstallerPath excludes Inno Setup's unins*.exe and msiexec.exe: both
// run from inside a game's own install directory while the game is being
// removed, and without this exclusion an uninstall in progress would look
// exactly like the game itself running.
func isUninstallerPath(path string) bool {
	base := strings.ToLower(filepath.Base(path))
	if base == "msiexec.exe" {
		return true
	}
	return strings.HasPrefix(base, "unins") && strings.HasSuffix(base, ".exe")
}
