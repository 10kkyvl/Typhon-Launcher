package install

import (
	"context"
	"log/slog"
	"os"
	"sort"
	"time"

	"typhon/internal/library"
	"typhon/internal/settings"
)

func (s *Service) run(ctx context.Context, id string) {
	item, ok := s.snapshot(id)
	if !ok {
		return
	}
	var err error
	switch {
	case item.Type == TypePortable:
		err = s.runPortable(ctx, id, item)
	case archived(item.Type):
		err = s.runArchive(ctx, id, item)
	case external(item.Type):
		err = s.runInstaller(ctx, id, item)
	default:
		err = errUnknownType
	}
	if err != nil {
		s.fail(id, err)
	}
}

func (s *Service) runPortable(ctx context.Context, id string, item Installation) error {
	s.setStatus(id, StatusPreparing)
	partial := item.Destination + partialSuffix
	if err := os.RemoveAll(partial); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	s.setStatus(id, StatusInstalling)

	report := func(p Progress) { s.updateProgress(id, p) }
	var err error
	if item.Mode == ModeMove {
		err = MoveDir(ctx, item.ContentRoot, partial, report)
	} else {
		err = CopyDir(ctx, item.ContentRoot, partial, report)
	}
	if err == nil {
		err = s.commit(ctx, partial, item.Destination)
	}
	if err != nil {
		s.cleanupPartial(partial)
		return err
	}
	return s.finalize(ctx, id)
}

func (s *Service) runArchive(ctx context.Context, id string, item Installation) error {
	s.setStatus(id, StatusPreparing)
	partial := item.Destination + partialSuffix
	if err := os.RemoveAll(partial); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	s.setStatus(id, StatusExtracting)

	err := ExtractArchive(ctx, item.ArchivePath, partial, func(p Progress) { s.updateProgress(id, p) })
	if err == nil {
		err = s.commit(ctx, partial, item.Destination)
	}
	if err != nil {
		s.cleanupPartial(partial)
		return err
	}
	return s.finalize(ctx, id)
}

func (s *Service) runInstaller(ctx context.Context, id string, item Installation) error {
	s.setStatus(id, StatusPreparing)
	roots := s.installRoots()
	before, err := takeSnapshot(roots)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if item.Unattended {
		return errNeedsUser
	}
	s.setStatus(id, StatusInstalling)

	spec := runSpec{Path: item.InstallerPath, Dir: item.WorkingDir}
	if item.Type == TypeMsiInstaller {
		spec = runSpec{Path: "msiexec", Args: []string{"/i", item.InstallerPath}, Dir: item.WorkingDir}
	}

	s.setExternal(id, true)
	code, err := s.runner.run(ctx, spec)
	s.setExternal(id, false)
	if err != nil {
		return err
	}
	if code != 0 && code != rebootExitCode {
		slog.Error("installer exit code", "id", id, "path", item.InstallerPath, "code", code)
		return errInstallerFail
	}

	after, err := takeSnapshot(roots)
	if err != nil {
		return err
	}
	dirs := diffSnapshot(before, after)
	candidates, err := gather(ctx, dirs, item.Name)
	if err != nil {
		return err
	}
	if dest := pickInstallDir(dirs, candidates); dest != "" {
		s.setDestination(id, dest)
	}
	s.waitForUser(id, candidates)
	return nil
}

func (s *Service) commit(ctx context.Context, partial, destination string) error {
	if entries, err := os.ReadDir(destination); err == nil && len(entries) == 0 {
		os.Remove(destination)
	}
	if err := os.Rename(partial, destination); err == nil {
		return nil
	}
	return MoveDir(ctx, partial, destination, nil)
}

func (s *Service) cleanupPartial(partial string) {
	if partial == "" || s.isClosing() {
		return
	}
	if err := os.RemoveAll(partial); err != nil {
		slog.Warn("remove partial install", "path", partial, "error", err)
	}
}

func (s *Service) finalize(ctx context.Context, id string) error {
	s.setStatus(id, StatusVerifying)
	item, ok := s.snapshot(id)
	if !ok {
		return errNotFound
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if item.Executable == "" {
		candidates, err := FindExecutables(ctx, item.Destination, item.Name)
		if err != nil {
			return err
		}
		switch {
		case HighConfidence(candidates):
			s.setExecutable(id, candidates[0].Path, candidates)
		case item.Unattended:
			executable := ""
			if len(candidates) > 0 {
				executable = candidates[0].Path
			}
			s.setExecutable(id, executable, candidates)
		default:
			s.waitForUser(id, candidates)
			return nil
		}
	}
	return s.complete(id)
}

func (s *Service) complete(id string) error {
	item, ok := s.snapshot(id)
	if !ok {
		return errNotFound
	}
	cfg := s.config()
	if cfg.VerifyAfterInstall {
		if err := verifyInstall(item); err != nil {
			return err
		}
	}
	version, source := detectVersion(item)
	var game library.Game
	if !item.SkipRegister {
		registered, err := s.register(item, version, source)
		if err != nil {
			return err
		}
		game = registered
	}

	s.mu.Lock()
	stored := s.findLocked(id)
	if stored == nil {
		s.mu.Unlock()
		return errNotFound
	}
	now := time.Now()
	stored.Status = StatusCompleted
	stored.GameID = game.ID
	stored.DetectedVersion = version
	stored.VersionSource = source
	stored.Progress = 1
	stored.CurrentFile = ""
	stored.Error = ""
	stored.CompletedAt = &now
	s.persistLocked()
	snap := snapshotOf(stored)
	s.mu.Unlock()

	slog.Info("install completed", "id", id, "name", snap.Name, "game", game.ID, "version", version)
	emit(eventCompleted, snap)
	emit(eventUpdated, snap)
	if !item.SkipRegister {
		s.applyCleanup(cfg, snap.DownloadID)
	}
	s.notifyFinished(snap)
	return nil
}

func (s *Service) register(item Installation, version, source string) (library.Game, error) {
	if s.library == nil {
		return library.Game{}, errNoLibrary
	}
	if item.Destination == "" {
		return library.Game{}, errEmptyDestination
	}
	return s.library.RegisterInstalled(library.InstalledGame{
		Title:            item.Name,
		Executable:       item.Executable,
		InstallDir:       item.Destination,
		Version:          version,
		VersionSource:    source,
		SourceDownloadID: item.DownloadID,
		ReleaseID:        item.Origin.ReleaseID,
		SourceID:         item.Origin.SourceID,
		CanonicalGameID:  item.Origin.GameID,
	})
}

func (s *Service) applyCleanup(cfg settings.Settings, downloadID string) {
	if cfg.InstallCleanupPolicy != settings.CleanupDelete || s.downloads == nil {
		return
	}
	d, err := s.downloads.Get(downloadID)
	if err != nil {
		return
	}
	if d.Seeding {
		slog.Info("cleanup skipped, download is seeding", "id", downloadID)
		return
	}
	if err := s.downloads.DeleteData(downloadID); err != nil {
		slog.Warn("cleanup download data", "id", downloadID, "error", err)
		return
	}
	slog.Info("download data removed after install", "id", downloadID)
}

func (s *Service) setExecutable(id, executable string, candidates []Candidate) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item := s.findLocked(id)
	if item == nil {
		return
	}
	item.Executable = executable
	item.Candidates = candidates
	s.persistLocked()
}

func (s *Service) setDestination(id, destination string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item := s.findLocked(id)
	if item == nil || item.Destination != "" {
		return
	}
	item.Destination = destination
	s.persistLocked()
}

const (
	VersionSourceRelease    = "release_metadata"
	VersionSourceExecutable = "executable_metadata"
)

func verifyInstall(item Installation) error {
	if item.Destination != "" {
		entries, err := os.ReadDir(item.Destination)
		if err != nil || len(entries) == 0 {
			return errEmptyInstall
		}
	}
	if item.Executable == "" {
		return nil
	}
	destination := ""
	if controlled(item.Type) {
		destination = item.Destination
	}
	return validExecutable(item.Executable, destination, item.Type)
}

func detectVersion(item Installation) (string, string) {
	if item.Origin.Version != "" {
		return item.Origin.Version, VersionSourceRelease
	}
	if item.Executable != "" {
		if info, ok := ExeVersion(item.Executable); ok && info.Version != "" {
			return info.Version, info.Source
		}
	}
	if item.Destination != "" {
		if info, ok := VersionFromFiles(item.Destination); ok {
			return info.Version, info.Source
		}
	}
	return "", ""
}

func gather(ctx context.Context, dirs []string, title string) ([]Candidate, error) {
	var out []Candidate
	for _, dir := range dirs {
		found, err := FindExecutables(ctx, dir, title)
		if err != nil {
			return nil, err
		}
		out = append(out, found...)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score == out[j].Score {
			return out[i].Path < out[j].Path
		}
		return out[i].Score > out[j].Score
	})
	if len(out) > maxCandidates {
		out = out[:maxCandidates]
	}
	return out, nil
}

func pickInstallDir(dirs []string, candidates []Candidate) string {
	if len(dirs) == 0 {
		return ""
	}
	if len(candidates) > 0 {
		for _, dir := range dirs {
			if inside(dir, candidates[0].Path) {
				return dir
			}
		}
		return ""
	}
	if len(dirs) == 1 {
		return dirs[0]
	}
	return ""
}
