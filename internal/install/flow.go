package install

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"typhon/internal/library"
	"typhon/internal/settings"
	"typhon/internal/usagestats"
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
		err = s.commitExtracted(ctx, partial, item.Destination)
	}
	if err != nil {
		s.cleanupPartial(partial)
		return err
	}
	return s.finalize(ctx, id)
}

func (s *Service) runInstaller(ctx context.Context, id string, item Installation) error {
	s.setStatus(id, StatusPreparing)
	cfg := s.config()
	roots := s.installRoots()
	before, err := takeSnapshot(roots)
	if err != nil {
		return err
	}
	beforeEntries, err := readUninstallEntries()
	if err != nil {
		return err
	}
	shell := s.shellBaseline(ctx, id, cfg)
	if err := ctx.Err(); err != nil {
		return err
	}
	if item.Silent && item.Destination != "" {
		return s.runSilent(ctx, id, item, roots, before, beforeEntries, shell)
	}
	if item.Unattended {
		return errNeedsUser
	}
	s.setStatus(id, StatusInstalling)

	for _, installer := range installerChain(item) {
		spec := runSpec{Path: installer, Dir: item.WorkingDir}
		if item.Type == TypeMsiInstaller {
			msiexec, err := systemExecutable("msiexec.exe")
			if err != nil {
				return err
			}
			spec = runSpec{Path: msiexec, Args: []string{"/i", installer}, Dir: item.WorkingDir}
		}

		s.setExternal(id, true)
		code, err := s.runner.run(ctx, spec)
		s.setExternal(id, false)
		if err != nil {
			return err
		}
		if code != 0 && code != rebootExitCode {
			slog.Error("installer exit code", "id", id, "path", installer, "code", code)
			return errInstallerFail
		}
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
	dest := pickInstallDir(dirs, candidates)
	if dest != "" {
		s.setDestination(id, dest)
	}
	s.setRemoval(id, dest, before, beforeEntries, item.Name)
	s.dropShortcuts(ctx, id, shell, dest)
	s.waitForUser(id, candidates)
	return nil
}

func (s *Service) runSilent(ctx context.Context, id string, item Installation, roots []string, before fsSnapshot, beforeEntries map[string]uninstallEntry, shell shellSnapshot) error {
	logPath := s.installerLogPath(id)
	statePath := s.workerStatePath(id)
	infPath := s.workerInfPath(id)
	cancelPath := s.workerCancelPath(id)
	opts := installOptionsFrom(s.config())
	chain := installerChain(item)
	specs := make([]runSpec, 0, len(chain))
	for _, installer := range chain {
		spec, err := silentSpec(item, installer, logPath, opts)
		if err != nil {
			return err
		}
		spec.StatePath = statePath
		spec.InfPath = infPath
		spec.CancelPath = cancelPath
		specs = append(specs, spec)
	}
	s.setStatus(id, StatusInstalling)

	stop := s.trackInstallSize(ctx, id, item.Destination, item.BytesTotal)
	runErr := s.runSilentChain(ctx, id, item, chain, specs, logPath)
	stop()
	if runErr != nil {
		s.discardSilent(item, before, runErr)
		return runErr
	}

	dropInstallerLog(logPath)

	dest, err := s.silentDestination(ctx, id, item, roots, before)
	if err != nil {
		s.discardSilent(item, before, err)
		return err
	}
	s.setRemoval(id, dest, before, beforeEntries, item.Name)
	s.dropShortcuts(ctx, id, shell, dest)
	return s.finalize(ctx, id)
}

// runSilentChain прогоняет установщики набора по очереди: дополнение GOG ставится
// только поверх уже установленной игры, поэтому порядок из плана обязателен, а
// первая же неудача останавливает цепочку.
func (s *Service) runSilentChain(ctx context.Context, id string, item Installation, chain []string, specs []runSpec, logPath string) error {
	for i, spec := range specs {
		dropInstallerLog(logPath)
		code, err := s.runner.run(ctx, spec)
		if err != nil {
			return err
		}
		exitErr := exitError(item.Engine, code)
		if exitErr == nil {
			continue
		}
		done, logErr := installerLogSucceeded(item.Engine, logPath)
		if logErr != nil {
			slog.Warn("read installer log", "id", id, "path", logPath, "error", logErr)
		}
		if !done {
			slog.Error("silent installer failed", "id", id, "engine", string(item.Engine),
				"path", chain[i], "code", code, "log", installerLogTail(logPath))
			return exitErr
		}
		// Установщики GOG падают при завершении уже после того, как файлы
		// разложены: свой лог они при этом закрывают отметкой об успехе.
		slog.Warn("installer crashed after finishing", "id", id, "engine", string(item.Engine),
			"path", chain[i], "code", code)
	}
	return nil
}

func installerChain(item Installation) []string {
	chain := make([]string, 0, len(item.ExtraInstallers)+1)
	if item.InstallerPath != "" {
		chain = append(chain, item.InstallerPath)
	}
	return append(chain, item.ExtraInstallers...)
}

func installOptionsFrom(cfg settings.Settings) installOptions {
	return installOptions{SkipShortcuts: cfg.InstallSkipShortcuts, SkipExtras: cfg.InstallSkipExtras}
}

// Снимок ярлыков берётся до запуска установщика: без него не отличить ярлык,
// созданный установкой, от ярлыка пользователя, поэтому ошибка обхода отменяет
// уборку целиком, а не разрешает удалять наугад.
func (s *Service) shellBaseline(ctx context.Context, id string, cfg settings.Settings) shellSnapshot {
	if !cfg.InstallSkipShortcuts {
		return shellSnapshot{}
	}
	roots, err := shortcutRoots()
	if err != nil {
		slog.Error("resolve shortcut folders", "id", id, "error", err)
		return shellSnapshot{}
	}
	snap, err := takeShellSnapshot(ctx, roots)
	if err != nil {
		slog.Error("scan shortcut folders", "id", id, "error", err)
		return shellSnapshot{}
	}
	return snap
}

// Ярлыки, созданные установщиком под UAC в общих каталогах, лаунчер удалить не
// может: он работает без прав администратора. Это не повод считать установку
// неудачной, поэтому ошибка только логируется.
func (s *Service) dropShortcuts(ctx context.Context, id string, before shellSnapshot, dest string) {
	if !before.taken || dest == "" {
		return
	}
	removed, err := cleanShellShortcuts(ctx, before, dest)
	if err != nil {
		slog.Warn("remove installer shortcuts", "id", id, "dest", dest, "error", err)
	}
	if len(removed) > 0 {
		slog.Info("installer shortcuts removed", "id", id, "count", len(removed), "paths", removed)
	}
}

// silentDestination доверяет заданному каталогу только после того, как убедился,
// что установщик действительно в него писал: часть установщиков игнорирует
// ключ каталога и ставит игру по своему пути, и тогда его надо найти по снимку.
func (s *Service) silentDestination(ctx context.Context, id string, item Installation, roots []string, before fsSnapshot) (string, error) {
	empty, err := dirEmpty(item.Destination)
	if err != nil {
		return "", err
	}
	if !empty {
		return item.Destination, nil
	}
	after, err := takeSnapshot(roots)
	if err != nil {
		return "", err
	}
	dirs := diffSnapshot(before, after)
	candidates, err := gather(ctx, dirs, item.Name)
	if err != nil {
		return "", err
	}
	found := pickInstallDir(dirs, candidates)
	if found == "" {
		return "", errInstallerNoOutput
	}
	found, err = normalizeRoot(found)
	if err != nil {
		return "", err
	}
	if err := os.Remove(item.Destination); err != nil && !errors.Is(err, fs.ErrNotExist) {
		slog.Warn("remove empty install dir", "path", item.Destination, "error", err)
	}
	slog.Warn("installer ignored target directory", "id", id, "want", item.Destination, "got", found)
	s.forceDestination(id, found)
	return found, nil
}

// discardSilent удаляет каталог, в который писала неудавшаяся тихая
// установка, но только если процесс, который туда писал, точно не жив:
// errInstallerNotConfirmedStopped значит ровно обратное — установщик под
// UAC не подтвердил остановку, и RemoveAll на живого писателя — гонка на
// единственной копии данных (инвариант 9).
func (s *Service) discardSilent(item Installation, before fsSnapshot, cause error) {
	if item.Destination == "" || s.isClosing() {
		return
	}
	if errors.Is(cause, errInstallerNotConfirmedStopped) {
		slog.Warn("keep install dir, installer not confirmed stopped", "path", item.Destination, "error", cause)
		return
	}
	if _, existed := before.dirs[item.Destination]; existed {
		return
	}
	if err := os.RemoveAll(item.Destination); err != nil {
		slog.Warn("remove failed install dir", "path", item.Destination, "error", err)
	}
}

func (s *Service) trackInstallSize(ctx context.Context, id, dir string, total int64) func() {
	ctx, cancel := context.WithCancel(ctx)
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(installPollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				size, err := DirSize(ctx, dir)
				if err != nil {
					continue
				}
				s.updateProgress(id, Progress{BytesDone: size, BytesTotal: total})
			}
		}
	}()
	return cancel
}

func silentSpec(item Installation, installer, logPath string, opts installOptions) (runSpec, error) {
	plan, err := silentArgs(item.Engine, installer, item.Destination, logPath, opts)
	if err != nil {
		return runSpec{}, err
	}
	path := installer
	if item.Engine == EngineMsi {
		msiexec, err := systemExecutable("msiexec.exe")
		if err != nil {
			return runSpec{}, err
		}
		path = msiexec
	}
	return runSpec{
		Path: path, Args: plan.Args, Dir: item.WorkingDir, CmdLine: plan.CmdLine, Tail: plan.Tail, Background: true, Hidden: true,
		ID: item.ID, Engine: item.Engine, InstallerPath: installer, Destination: item.Destination, LogPath: logPath, Options: opts,
	}, nil
}

func dirEmpty(dir string) (bool, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("read dir %s: %w", dir, err)
	}
	return len(entries) == 0, nil
}

func dropInstallerLog(path string) {
	if path == "" {
		return
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		slog.Warn("remove installer log", "path", path, "error", err)
	}
}

// installerLogSucceeded отвечает на вопрос, разложил ли установщик файлы, когда
// код возврата говорит об обратном: Inno закрывает свой лог отметкой об успехе
// до кода возврата, и падение на выходе не отменяет уже сделанную установку.
func installerLogSucceeded(engine Engine, path string) (bool, error) {
	if engine != EngineInno || path == "" {
		return false, nil
	}
	data, err := readLogTail(path, installerLogScanLimit)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return strings.Contains(decodeLogText(data), innoSuccessMarker), nil
}

func installerLogTail(path string) string {
	if path == "" {
		return ""
	}
	data, err := readLogTail(path, installerLogTailLimit)
	if err != nil {
		return "лог недоступен: " + err.Error()
	}
	return strings.TrimSpace(decodeLogText(data))
}

func readLogTail(path string, limit int64) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := f.Close(); err != nil {
			slog.Warn("close installer log", "path", path, "error", err)
		}
	}()
	if info.Size() > limit {
		if _, err := f.Seek(info.Size()-limit, io.SeekStart); err != nil {
			return nil, err
		}
	}
	return io.ReadAll(io.LimitReader(f, limit))
}

// Inno пишет лог в UTF-16LE, а на диске он может оказаться и в UTF-8: нулевые
// байты убираем, чтобы обе кодировки читались одним поиском по подстроке.
func decodeLogText(data []byte) string {
	return strings.ReplaceAll(string(data), "\x00", "")
}

// setRemoval выясняет, чем игру потом удалять: свежая запись в ветке Uninstall
// даёт деинсталлятор, а отсутствие каталога в снимке до установки — право
// удалить каталог целиком. Ошибка чтения реестра не превращается в «удалять
// нечем»: она помечается UninstallUnknown, и UI предложит системный апплет.
func (s *Service) setRemoval(id, destination string, before fsSnapshot, beforeEntries map[string]uninstallEntry, name string) {
	owned := false
	if destination != "" {
		_, existed := before.dirs[destination]
		owned = !existed
	}
	uninstall, unknown := library.Uninstall{}, false
	afterEntries, err := readUninstallEntries()
	if err != nil {
		slog.Error("read uninstall entries", "id", id, "error", err)
		unknown = true
	} else if picked, ok := pickUninstall(beforeEntries, afterEntries, destination, name); ok {
		uninstall = picked
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	item := s.findLocked(id)
	if item == nil {
		return
	}
	item.Owned = owned
	item.Uninstall = uninstall
	item.UninstallUnknown = unknown
	s.persistLocked()
}

func (s *Service) commitExtracted(ctx context.Context, partial, destination string) error {
	root, err := normalizeRoot(partial)
	if err != nil {
		return err
	}
	if root == partial {
		return s.commit(ctx, partial, destination)
	}
	if err := s.commit(ctx, root, destination); err != nil {
		return err
	}
	s.cleanupPartial(partial)
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
	duration := time.Duration(0)
	if snap.CompletedAt != nil {
		duration = snap.CompletedAt.Sub(snap.StartedAt)
	}
	s.recordUsage(usagestats.Event{
		Type:      usagestats.TypeInstallCompleted,
		Timestamp: time.Now(),
		Properties: usagestats.Properties{
			GameID:          snap.Origin.GameID,
			InstallerType:   installerType(snap.Type),
			DurationSeconds: usageDurationSeconds(duration),
		},
	})
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
	title := s.titleOf(item.Origin)
	if title == "" {
		title = item.Name
	}
	return s.library.RegisterInstalled(library.InstalledGame{
		Title:            title,
		Executable:       item.Executable,
		InstallDir:       item.Destination,
		Version:          version,
		VersionSource:    source,
		SourceDownloadID: item.DownloadID,
		ReleaseID:        item.Origin.ReleaseID,
		SourceID:         item.Origin.SourceID,
		CanonicalGameID:  item.Origin.GameID,
		InstallType:      string(item.Type),
		Owned:            item.Owned,
		Uninstall:        item.Uninstall,
		UninstallUnknown: item.UninstallUnknown,
	})
}

func (s *Service) applyCleanup(cfg settings.Settings, downloadID string) {
	if cfg.InstallCleanupPolicy != settings.CleanupDelete || s.downloads == nil {
		return
	}
	d, err := s.downloads.Get(downloadID)
	if err != nil {
		slog.Error("cleanup lookup download", "id", downloadID, "error", err)
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

func (s *Service) installerLogPath(id string) string {
	if s.store == nil || s.store.dir == "" {
		return ""
	}
	return filepath.Join(s.store.dir, "installer-"+id+".log")
}

func (s *Service) workerStatePath(id string) string {
	if s.store == nil || s.store.dir == "" {
		return ""
	}
	return workerStatePath(s.store.dir, id)
}

func (s *Service) workerInfPath(id string) string {
	if s.store == nil || s.store.dir == "" {
		return ""
	}
	return workerInfPath(s.store.dir, id)
}

func (s *Service) workerCancelPath(id string) string {
	if s.store == nil || s.store.dir == "" {
		return ""
	}
	return workerCancelPath(s.store.dir, id)
}

func (s *Service) forceDestination(id, destination string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item := s.findLocked(id)
	if item == nil || destination == "" {
		return
	}
	item.Destination = destination
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
	if ownDestination(item.Type, item.Silent) {
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
