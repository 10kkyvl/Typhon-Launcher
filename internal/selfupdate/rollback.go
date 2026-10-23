package selfupdate

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"typhon/internal/uierr"
	"typhon/internal/version"
)

// RollbackInfo says whether the launcher can go back to the version it was
// updated from. Version is the version it would go back to. When Available is
// false, Reason says why and ReasonCode is a stable code the interface can
// translate; "selfupdate.rollback_none" is the ordinary "nothing saved yet".
type RollbackInfo struct {
	Available  bool   `json:"available"`
	Version    string `json:"version"`
	Reason     string `json:"reason"`
	ReasonCode string `json:"reasonCode"`
}

// Seams for tests: what the platform supports and the uninstall-entry write.
var (
	rollbackSupported    = platformRollbackSupported
	updateDisplayVersion = setDisplayVersion
)

func unavailable(err error) RollbackInfo {
	return RollbackInfo{Reason: err.Error(), ReasonCode: uierr.Code(err)}
}

// checkRollback is cheap on purpose, it runs whenever the settings page
// opens: it checks that the record and the saved launcher are there and agree
// on size, and leaves the hash for the worker that uses them.
func checkRollback(configDir, current string) (previousRecord, error) {
	if err := rollbackSupported(); err != nil {
		return previousRecord{}, err
	}
	rec, err := loadPrevious(configDir)
	if err != nil {
		return previousRecord{}, err
	}
	dir, err := PreviousDir(configDir)
	if err != nil {
		return previousRecord{}, err
	}
	info, err := os.Stat(filepath.Join(dir, previousExeName))
	if err != nil {
		return previousRecord{}, fmt.Errorf("%w: %w", ErrPreviousBroken, err)
	}
	if !info.Mode().IsRegular() || info.Size() != rec.Size {
		return previousRecord{}, fmt.Errorf("%w: %s has %d bytes, the record says %d", ErrPreviousBroken, previousExeName, info.Size(), rec.Size)
	}
	older, err := IsNewer(current, rec.Version)
	if err != nil {
		return previousRecord{}, fmt.Errorf("compare %q with %q: %w", current, rec.Version, err)
	}
	if !older {
		return previousRecord{}, ErrPreviousNotOlder
	}
	return rec, nil
}

func rollbackAvailability(configDir, current string) RollbackInfo {
	rec, err := checkRollback(configDir, current)
	if err != nil {
		return unavailable(err)
	}
	return RollbackInfo{Available: true, Version: rec.Version}
}

// launchRollback stages the worker for a rollback and starts it; the caller
// then exits so the worker can replace the files. It is shared by the settings
// button and by the start-up guard, which has no service to ask.
func launchRollback(configDir string, portable bool, current, language string) error {
	rec, err := checkRollback(configDir, current)
	if err != nil {
		return err
	}
	exe, err := currentExecutable()
	if err != nil {
		return fmt.Errorf("selfupdate: resolve executable: %w", err)
	}
	installDir := filepath.Dir(exe)
	if err := validateInstallDir(installDir); err != nil {
		return err
	}
	workerPath, err := stageWorkerCopy(configDir, exe)
	if err != nil {
		return err
	}
	specPath, err := SpecPath(configDir)
	if err != nil {
		return err
	}
	spec := updateSpec{
		Kind:         specKindRollback,
		Language:     language,
		InstallDir:   installDir,
		ParentPID:    os.Getpid(),
		RelaunchPath: exe,
		Version:      rec.Version,
		FromVersion:  current,
		Portable:     portable,
	}
	if err := writeUpdateSpec(specPath, spec); err != nil {
		return err
	}
	return startWorker(workerPath, specPath)
}

// performRollback is the worker's half of a rollback, run after the launcher
// has exited. Everything that can fail without changing anything comes first
// (the saved launcher's hash, every state backup's hash, the skip record, the
// uninstall entry); the launcher swap is last and is one rename. The order
// matters if the worker dies half-way: state goes back before the swap because
// the newer launcher migrates older state forward again, while the older one
// cannot read newer state. Dying between the two leaves a launcher that starts.
func performRollback(ctx context.Context, configDir, target, from, to string, portable bool) error {
	if from == "" || to == "" {
		return errors.New("selfupdate: rollback needs both the current and the target version")
	}
	rec, err := loadPrevious(configDir)
	if err != nil {
		return err
	}
	if rec.Version != to {
		return fmt.Errorf("%w: saved %q, requested %q", ErrPreviousVersionDiffer, rec.Version, to)
	}
	exe, err := verifyPreviousExe(ctx, configDir, rec)
	if err != nil {
		return err
	}
	items, err := planStateRestore(ctx, configDir, rec)
	if err != nil {
		return err
	}
	if err := markSkipped(configDir, from); err != nil {
		return fmt.Errorf("selfupdate: remember the skipped version: %w", err)
	}

	revert := func() error { return nil }
	if !portable {
		revert, err = updateDisplayVersion(rec.Version)
		if err != nil {
			return fmt.Errorf("selfupdate: update the installed version in the registry: %w", err)
		}
	}
	fail := func(cause error) error {
		if rerr := revert(); rerr != nil {
			return errors.Join(cause, fmt.Errorf("selfupdate: put the registry version back: %w", rerr))
		}
		return cause
	}
	if err := applyStateRestore(ctx, items); err != nil {
		return fail(err)
	}
	if err := swapExecutable(ctx, target, exe, rec.SHA256); err != nil {
		return fail(err)
	}
	if err := dropPrevious(configDir); err != nil {
		slog.Warn("selfupdate: remove the used rollback snapshot, it is dropped on the next start", "error", err)
	}
	return nil
}

// markSkipped makes the update check stop offering the version the launcher
// is leaving, until a newer one is released.
func markSkipped(configDir, ver string) error {
	store, err := NewStore(configDir)
	if err != nil {
		return err
	}
	st, err := store.Load()
	if err != nil {
		return err
	}
	st.SkippedVersion = ver
	return store.Save(st)
}

func sameVersion(a, b string) bool {
	if a == b {
		return true
	}
	cmp, ok := version.Compare(version.Parse(a), version.Parse(b))
	return ok && cmp == 0
}
