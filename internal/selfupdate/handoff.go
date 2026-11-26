package selfupdate

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"typhon/internal/platform"
	"typhon/internal/storage"
	"typhon/internal/uierr"
	"typhon/internal/windowhandoff"
)

// A seamless update ("handoff") swaps the launcher on disk while it runs,
// starts the new build beside it, and closes the old process once the new
// window is on screen. This file holds what both sides share: the argument
// that carries the hand-over, the journal that makes it recoverable and the
// leftovers it cleans.

const (
	handoffFlag           = "--handoff"
	handoffJournalName    = "handoff.json"
	handoffJournalVersion = 1
	handoffDirName        = "handoff"
	handoffReadyPrefix    = "ready-"

	// oldPrefix names the previous launcher parked beside the new one. The
	// folder may hold other things (a portable copy), so cleanup removes only
	// names with this prefix and nothing else (invariant 10).
	oldPrefix    = ".typhon-old-"
	stagedPrefix = ".typhon-new-"

	// defaultHandoffBackground is the dark theme's page colour (--bg of the
	// dark preset), shown by the hidden window before the stylesheet loads.
	defaultHandoffBackground = "#0e0f11"
)

// Journal stages. The journal is written before the first change to the
// launcher on disk and removed once the successor has taken over.
const (
	stagePrepared = "prepared" // journal written, the swap may be half done
	stageSwapped  = "swapped"  // the new launcher is in place, the successor not confirmed
	stageReady    = "ready"    // the successor rendered; the old process is closing
	stageDone     = "done"     // the successor owns the launcher; leftovers may go
	// stageUndoOwed: the hand-over failed and putting the old launcher back
	// failed too. The parked launcher may be the only one left; nothing is
	// swept and no new update starts until recovery has put it back.
	stageUndoOwed = "undo-owed"
)

// handoffMethod says how the new launcher gets onto the disk.
type handoffMethod string

const (
	methodExeFile   handoffMethod = "exe-file"  // the artifact is the launcher binary
	methodExeZip    handoffMethod = "exe-zip"   // the binary is inside a portable zip
	methodInstaller handoffMethod = "installer" // the Windows NSIS installer, run silently
	methodBundle    handoffMethod = "bundle"    // a macOS .app in a zip
)

// ErrHandoffUnsupported means this build, platform or installation cannot
// update without a restart; nothing was changed and the worker path is open.
var ErrHandoffUnsupported = uierr.New("selfupdate.handoff_unsupported", "selfupdate: updating without a restart is not available here")

// ErrHandoffPending refuses an update, with or without a restart, while an
// earlier hand-over has not been settled: the launcher on disk may not be the
// build that runs, and a new attempt would save the wrong build as the
// previous version and sweep the parked one. The next launch settles it.
var ErrHandoffPending = uierr.New("selfupdate.handoff_pending", "selfupdate: an earlier update is not settled yet")

var errHandoffBadSpec = errors.New("selfupdate: the hand-off argument is not valid")

// errJournalUnparseable is a journal that was read but cannot be understood:
// broken JSON, or a format this build does not know.
var errJournalUnparseable = errors.New("selfupdate: the hand-off journal cannot be parsed")

// unparseableJournalSuffix marks a journal set aside because it could not be
// parsed; a unix-nanosecond stamp follows, so none overwrites another.
const unparseableJournalSuffix = ".unreadable-"

// HandoffSpec is what the old launcher passes to its successor on the command
// line (base64url JSON after --handoff).
type HandoffSpec struct {
	Nonce       string                 `json:"nonce"`
	ParentPID   int                    `json:"parentPid"`
	Geometry    windowhandoff.Geometry `json:"geometry"`
	PhaseStart  int64                  `json:"phaseStart"`
	ReadyPath   string                 `json:"readyPath"`
	ConfigDir   string                 `json:"configDir"`
	Version     string                 `json:"version"`
	FromVersion string                 `json:"fromVersion,omitempty"`
	Background  string                 `json:"background,omitempty"`
}

// HandoffInfo is what the update screen asks of a successor launcher. Active
// stays true until the old process is gone and this one owns the launcher.
// Waiting turns true when the old process is still shutting down
// handoffSlowAfter after this window was revealed (launcher:handoff_waiting),
// and false again once it is gone.
type HandoffInfo struct {
	Active     bool   `json:"active"`
	PhaseStart int64  `json:"phaseStart"`
	Version    string `json:"version"`
	Background string `json:"background"`
	Waiting    bool   `json:"waiting"`
}

// handoffJournal is the record of a handoff in progress.
type handoffJournal struct {
	Nonce       string        `json:"nonce"`
	Stage       string        `json:"stage"`
	Method      handoffMethod `json:"method"`
	Target      string        `json:"target"`
	OldPath     string        `json:"oldPath"`
	StagedPath  string        `json:"stagedPath,omitempty"`
	Version     string        `json:"version"`
	FromVersion string        `json:"fromVersion"`
	ParentPID   int           `json:"parentPid"`
	StartedAt   time.Time     `json:"startedAt"`
}

// Encode packs the spec for the command line.
func (s HandoffSpec) Encode() (string, error) { return windowhandoff.Encode(s) }

// ParseHandoffArgs finds --handoff in args and decodes it. found is false when
// the flag is absent; a present but malformed flag is an error, so a broken
// successor never mistakes itself for a normal start.
func ParseHandoffArgs(args []string) (spec HandoffSpec, found bool, err error) {
	for i := 1; i < len(args); i++ {
		if args[i] != handoffFlag {
			continue
		}
		value := args[i+1:]
		if len(value) == 0 {
			return HandoffSpec{}, true, fmt.Errorf("%w: no value", errHandoffBadSpec)
		}
		if err := windowhandoff.Decode(value[0], &spec); err != nil {
			return HandoffSpec{}, true, fmt.Errorf("%w: %w", errHandoffBadSpec, err)
		}
		if err := spec.validate(); err != nil {
			return HandoffSpec{}, true, err
		}
		return spec, true, nil
	}
	return HandoffSpec{}, false, nil
}

func (s HandoffSpec) validate() error {
	switch {
	case s.Nonce == "" || validatePathSegment(s.Nonce) != nil:
		return fmt.Errorf("%w: nonce", errHandoffBadSpec)
	case s.ParentPID <= 0:
		return fmt.Errorf("%w: parent pid", errHandoffBadSpec)
	case s.ConfigDir == "" || !filepath.IsAbs(s.ConfigDir):
		return fmt.Errorf("%w: config dir", errHandoffBadSpec)
	case s.ReadyPath != filepath.Join(s.ConfigDir, "selfupdate", handoffDirName, handoffReadyPrefix+s.Nonce):
		// The path is input from the command line; only the one marker the old
		// process polls may be written.
		return fmt.Errorf("%w: ready path", errHandoffBadSpec)
	}
	return nil
}

// ValidBackground reports whether c is a #rrggbb colour.
func ValidBackground(c string) bool {
	if len(c) != 7 || c[0] != '#' {
		return false
	}
	_, err := hex.DecodeString(c[1:])
	return err == nil
}

// BackgroundRGB parses spec.Background, falling back to the default theme.
func (s HandoffSpec) BackgroundRGB() (r, g, b uint8) {
	c := s.Background
	if !ValidBackground(c) {
		c = defaultHandoffBackground
	}
	raw, err := hex.DecodeString(c[1:])
	if err != nil {
		// Both inputs were validated above; the default is a constant.
		return 14, 15, 17
	}
	return raw[0], raw[1], raw[2]
}

func newNonce() (string, error) {
	raw := make([]byte, 8)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("selfupdate: make a hand-off nonce: %w", err)
	}
	return hex.EncodeToString(raw), nil
}

func handoffJournalPath(configDir string) (string, error) {
	dir, err := CacheDir(configDir)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, handoffJournalName), nil
}

// HandoffReadyPath is the marker the successor writes when its window shows.
func HandoffReadyPath(configDir, nonce string) (string, error) {
	dir, err := CacheDir(configDir)
	if err != nil {
		return "", err
	}
	if err := validatePathSegment(nonce); err != nil {
		return "", err
	}
	return filepath.Join(dir, handoffDirName, handoffReadyPrefix+nonce), nil
}

func saveJournal(configDir string, j handoffJournal) error {
	path, err := handoffJournalPath(configDir)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), cacheDirMode); err != nil {
		return fmt.Errorf("selfupdate: create the hand-off journal dir: %w", err)
	}
	if err := storage.Save(path, handoffJournalVersion, j); err != nil {
		return fmt.Errorf("selfupdate: write the hand-off journal: %w", err)
	}
	return nil
}

// loadJournal returns fs.ErrNotExist (wrapped) when there is no journal.
func loadJournal(configDir string) (handoffJournal, error) {
	path, err := handoffJournalPath(configDir)
	if err != nil {
		return handoffJournal{}, err
	}
	var j handoffJournal
	if err := storage.Load(path, handoffJournalVersion, nil, &j); err != nil {
		// A failure to read the file comes as a path error; anything else
		// was said by the bytes.
		var pathErr *fs.PathError
		if !errors.As(err, &pathErr) {
			return handoffJournal{}, fmt.Errorf("%w: %w", errJournalUnparseable, err)
		}
		return handoffJournal{}, err
	}
	return j, nil
}

// setAsideJournal renames a journal that cannot be parsed next to itself, so
// that it stops holding updates back and stays for whoever wants to read it.
// Nothing it may describe is touched.
func setAsideJournal(configDir string, cause error) error {
	path, err := handoffJournalPath(configDir)
	if err != nil {
		return err
	}
	aside := fmt.Sprintf("%s%s%d", path, unparseableJournalSuffix, time.Now().UnixNano())
	if err := storage.Rename(path, aside); err != nil {
		return fmt.Errorf("selfupdate: set aside the hand-off journal that cannot be parsed (%w): %w", cause, err)
	}
	slog.Error("selfupdate: the hand-off journal cannot be parsed, set it aside; updates are allowed again", "path", aside, "cause", cause)
	return nil
}

// runsFromOwnName reports that exe is not under one of the names a hand-off
// parks or stages a launcher under: this start is the launcher's ordinary
// path, and something runnable sits at the launcher's name (this process).
func runsFromOwnName(exe string, exeErr error) bool {
	if exeErr != nil {
		return false
	}
	if _, err := os.Lstat(exe); err != nil {
		return false
	}
	for p := exe; ; p = filepath.Dir(p) {
		base := filepath.Base(p)
		if strings.HasPrefix(base, oldPrefix) || strings.HasPrefix(base, stagedPrefix) {
			return false
		}
		if filepath.Dir(p) == p {
			return true
		}
	}
}

func clearJournal(configDir string) error {
	path, err := handoffJournalPath(configDir)
	if err != nil {
		return err
	}
	return removeIfExists(path)
}

// clearReadyMarkers removes the markers of earlier hand-offs. They live in a
// directory of their own, so whatever sits there is ours.
func clearReadyMarkers(configDir string) error {
	cache, err := CacheDir(configDir)
	if err != nil {
		return err
	}
	dir := filepath.Join(cache, handoffDirName)
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var errs []error
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), handoffReadyPrefix) || !e.Type().IsRegular() {
			continue
		}
		errs = append(errs, removeIfExists(filepath.Join(dir, e.Name())))
	}
	return errors.Join(errs...)
}

// sweepOld removes the parked previous launchers (and staged leftovers) in
// dir. Only names with our prefixes go, files or bundle directories alike, and
// nothing outside dir is touched. Call it only when no old process is alive:
// Windows refuses to delete a running image and the failure is just logged.
func sweepOld(dir string) {
	if dir == "" {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			slog.Warn("selfupdate: scan for parked launchers", "dir", dir, "error", err)
		}
		return
	}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, oldPrefix) && !strings.HasPrefix(name, stagedPrefix) {
			continue
		}
		path := filepath.Join(dir, name)
		if err := os.RemoveAll(path); err != nil {
			slog.Warn("selfupdate: remove a parked launcher, will retry on the next start", "path", path, "error", err)
		}
	}
}

// targetPresent reports whether something sits at the launcher's name.
func targetPresent(j handoffJournal) bool {
	_, err := os.Lstat(j.Target)
	return err == nil
}

// runsFromTarget reports that this process was started from the launcher's
// name and that name is still taken: then the build at the name is the one
// running, and its version is known. Recovery runs at start, before anything
// could have moved the name since.
func runsFromTarget(j handoffJournal, exe string, exeErr error) bool {
	if exeErr != nil || !targetPresent(j) {
		return false
	}
	if j.Method == methodBundle {
		return platform.SamePath(filepath.Dir(filepath.Dir(filepath.Dir(exe))), j.Target)
	}
	return platform.SamePath(exe, j.Target)
}

// settleBundleJournal settles a bundle hand-off from what this start proves.
// The parked name holds the new bundle until the exchange and the old one
// after it, and no stage written around an atomic exchange can tell a kill
// just before it from one just after it. The running version can: a start
// from the bundle at the target knows which build is there, and then the
// parked bundle is the other one and surplus. Without that proof nothing is
// removed and the journal stays.
func settleBundleJournal(j handoffJournal, exe string, exeErr error, current string) error {
	if !runsFromTarget(j, exe, exeErr) {
		return fmt.Errorf("selfupdate: this launcher does not run from %s and cannot tell which bundle is parked; the hand-off is left for a start from there", j.Target)
	}
	if sameVersion(j.Version, current) {
		slog.Info("selfupdate: the new bundle runs, removing the parked one", "stage", j.Stage)
	} else {
		slog.Warn("selfupdate: a bundle hand-off did not finish; the previous build runs, removing the parked new one", "stage", j.Stage)
	}
	if err := os.RemoveAll(j.OldPath); err != nil {
		return fmt.Errorf("selfupdate: remove the parked bundle: %w", err)
	}
	return nil
}

// RecoverHandoff settles a handoff whose old process died before it finished
// (design step 9). Run it on a normal start, before the services load state;
// the successor of a live handoff never calls it. current is the running
// version: if it is the one the journal was moving to, the successor made it
// and the leftovers go; otherwise the swap is undone.
func RecoverHandoff(configDir, current string) error {
	exe, exeErr := currentExecutable()
	j, err := loadJournal(configDir)
	if errors.Is(err, fs.ErrNotExist) {
		if exeErr == nil {
			sweepOld(handoffOldDir(exe))
		}
		return nil
	}
	if errors.Is(err, errJournalUnparseable) && runsFromOwnName(exe, exeErr) {
		return setAsideJournal(configDir, err)
	}
	if err != nil {
		return fmt.Errorf("selfupdate: read the hand-off journal: %w", err)
	}
	if j.ParentPID > 0 && j.ParentPID != os.Getpid() {
		alive, aerr := parentAlive(j.ParentPID)
		if aerr != nil {
			return fmt.Errorf("selfupdate: probe the old launcher: %w", aerr)
		}
		if alive {
			return nil // a handoff is running; its owner settles it
		}
	}

	if err := checkOwnedName(j); err != nil {
		return err
	}

	var errs []error
	if j.Method == methodBundle {
		if err := settleBundleJournal(j, exe, exeErr, current); err != nil {
			// Keep the journal: the next start tries again.
			return err
		}
		if !sameVersion(j.Version, current) {
			if derr := discardStaging(configDir); derr != nil {
				errs = append(errs, fmt.Errorf("selfupdate: drop the unused snapshot: %w", derr))
			}
		}
	} else if (j.Stage == stageDone && targetPresent(j)) || (sameVersion(j.Version, current) && runsFromTarget(j, exe, exeErr)) {
		// A launcher sits at the name: the successor that took over, or this
		// start, which is the new build run from there. The parked one is
		// surplus, whatever the stage, an owed undo included.
		sweepOld(filepath.Dir(j.OldPath))
	} else {
		// Putting the parked launcher back is safe from any state: only the
		// launcher that was running ever goes to the parked name, and with the
		// name empty it is the only launcher left.
		slog.Warn("selfupdate: a hand-off did not finish, putting the previous launcher back", "stage", j.Stage, "version", j.Version)
		if uerr := undoParkedExe(j); uerr != nil {
			return fmt.Errorf("selfupdate: undo an interrupted hand-off: %w", uerr)
		}
		if derr := discardStaging(configDir); derr != nil {
			errs = append(errs, fmt.Errorf("selfupdate: drop the unused snapshot: %w", derr))
		}
	}
	if cerr := clearReadyMarkers(configDir); cerr != nil {
		errs = append(errs, cerr)
	}
	if cerr := clearJournal(configDir); cerr != nil {
		errs = append(errs, fmt.Errorf("selfupdate: remove the hand-off journal: %w", cerr))
	}
	return errors.Join(errs...)
}
