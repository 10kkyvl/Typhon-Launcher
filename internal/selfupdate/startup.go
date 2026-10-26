package selfupdate

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"typhon/internal/dialogtext"
	"typhon/internal/settings"
	"typhon/internal/storage"
)

const (
	startupMarkerName    = "startup.json"
	startupMarkerVersion = 1

	// failedStartThreshold is the start that counts as the third in a row: the
	// two before it never reached a working window.
	failedStartThreshold = 3
)

// Seams for tests: the instance lock, and the questions put to the user.
var (
	acquireInstance = holdInstanceLock
	askYesNo        = askYesNoDialog
	showError       = showErrorDialog
	systemLanguage  = defaultSystemLanguage
)

// startupMarker is written before the services start and removed once the
// window is up. A marker still there at the next start means the last start
// did not get that far.
type startupMarker struct {
	Version  string `json:"version"`
	Attempts int    `json:"attempts"`
}

// StartGuard counts launcher starts that did not reach a working window. Its
// failures are logged once and never returned: the guard exists to help when
// the launcher does not start, and a guard that itself stops the launcher from
// starting would be the failure it is there to catch.
type StartGuard struct {
	dir      string
	path     string
	version  string
	attempts int
	inert    bool
	warned   sync.Once
}

// BeginStart records one more start of version ver. Call it as early as the
// config directory is known, before any service reads its state.
func BeginStart(configDir, ver string) *StartGuard {
	g := &StartGuard{dir: configDir, version: ver}
	if !startGuardSupported {
		g.inert = true
		return g
	}
	cache, err := CacheDir(configDir)
	if err != nil {
		g.warn("resolve the start marker", err)
		g.inert = true
		return g
	}
	g.path = filepath.Join(cache, startupMarkerName)

	// A second copy of the launcher exits at the single-instance check long
	// before it could clear the marker; counted, a few double clicks would
	// look like three failed starts. The lock is held until the process ends.
	held, err := acquireInstance(instanceLockName(configDir))
	if err != nil {
		g.warn("take the instance lock, counting anyway", err)
	} else if !held {
		g.inert = true
		return g
	}

	prev, err := readStartupMarker(g.path)
	if err != nil {
		g.warn("read the start marker, counting from scratch", err)
		prev = startupMarker{}
	}
	g.attempts = 1
	if prev.Version == ver && prev.Attempts > 0 {
		g.attempts = prev.Attempts + 1
	}
	g.write(startupMarker{Version: ver, Attempts: g.attempts})
	return g
}

// instanceLockName is per config directory, so a portable copy beside an
// installed launcher does not take the other's turn.
func instanceLockName(configDir string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(filepath.Clean(configDir))))
	return "typhon-start-guard-" + hex.EncodeToString(sum[:8])
}

// Attempts is the number of starts of this version in a row, this one
// included; zero for a guard that is not counting.
func (g *StartGuard) Attempts() int {
	if g.inert {
		return 0
	}
	return g.attempts
}

// Succeeded clears the marker: the launcher is up.
func (g *StartGuard) Succeeded() {
	if g.inert {
		return
	}
	if err := removeIfExists(g.path); err != nil {
		g.warn("clear the start marker", err)
	}
}

// Reset forgets the failed starts, for a user who chose to keep this version.
func (g *StartGuard) Reset() {
	if g.inert {
		return
	}
	g.attempts = 0
	g.write(startupMarker{Version: g.version})
}

func (g *StartGuard) write(m startupMarker) {
	if err := os.MkdirAll(filepath.Dir(g.path), cacheDirMode); err != nil {
		g.warn("create the start marker directory", err)
		return
	}
	if err := storage.Save(g.path, startupMarkerVersion, m); err != nil {
		g.warn("write the start marker", err)
	}
}

func (g *StartGuard) warn(what string, err error) {
	g.warned.Do(func() {
		slog.Warn("selfupdate: start guard", "step", what, "error", err)
	})
}

func readStartupMarker(path string) (startupMarker, error) {
	var m startupMarker
	err := storage.Load(path, startupMarkerVersion, nil, &m)
	if errors.Is(err, fs.ErrNotExist) {
		return startupMarker{}, nil
	}
	if err != nil {
		return startupMarker{}, err
	}
	return m, nil
}

// OfferRollback asks the user whether to go back, once the launcher has
// failed to start often enough and a previous version is saved. It reports
// true when a rollback worker has been started and the caller must exit now.
// A "no" is remembered by resetting the count; an error is returned for the
// caller to log, and the start goes on.
func (g *StartGuard) OfferRollback(portable bool) (bool, error) {
	if g.Attempts() < failedStartThreshold {
		return false, nil
	}
	info := rollbackAvailability(g.dir, g.version)
	if !info.Available {
		slog.Warn("selfupdate: the launcher failed to start repeatedly and there is nothing to roll back to", "reason", info.Reason)
		return false, nil
	}

	language := bootLanguage(g.dir)
	labels := dialogtext.For(language)
	yes, err := askYesNo(labels.RollbackTitle, fmt.Sprintf(labels.RollbackAsk, info.Version))
	if err != nil {
		return false, fmt.Errorf("selfupdate: ask about a rollback: %w", err)
	}
	if !yes {
		g.Reset()
		return false, nil
	}
	if err := launchRollback(g.dir, portable, g.version, language); err != nil {
		showError(labels.RollbackTitle, fmt.Sprintf(labels.RollbackFailed, err))
		return false, fmt.Errorf("selfupdate: start the rollback: %w", err)
	}
	return true, nil
}

// bootLanguage is the language to ask in before any service exists: the one
// in the settings file, or the system's when the file says "system" or cannot
// be read. The fallback only picks the wording of a question.
func bootLanguage(configDir string) string {
	svc, err := settings.NewServiceAt(filepath.Join(configDir, "settings.json"))
	if err != nil {
		slog.Warn("selfupdate: read the language for the rollback question", "error", err)
		return systemLanguage()
	}
	switch lang := svc.GetSettings().Language; lang {
	case settings.LanguageRU, settings.LanguageEN:
		return lang
	}
	return systemLanguage()
}
