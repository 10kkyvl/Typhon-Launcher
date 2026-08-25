package install

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"typhon/internal/download"
	"typhon/internal/library"
)

type RemovalMethod string

const (
	RemovalFiles     RemovalMethod = "files"
	RemovalInstaller RemovalMethod = "installer"
	RemovalRecord    RemovalMethod = "record"
)

const (
	removingSuffix   = ".removing"
	msiCancelled     = 1602
	msiNotInstalled  = 1605
	msiRebootPending = 3010
)

var (
	errGameRunning        = errors.New("игра запущена, закройте её перед удалением")
	errGameBusy           = errors.New("по этой игре идёт установка или обновление")
	errRemoveSeeding      = errors.New("раздача активна, остановите её перед удалением")
	errFilesLocked        = errors.New("файлы игры заняты другим процессом")
	errUninstallCancelled = errors.New("удаление отменено в установщике")
	errUninstallFailed    = errors.New("деинсталлятор завершился с ошибкой")
	errNoUninstaller      = errors.New("деинсталлятор не найден")
	errBadCommand         = errors.New("команда удаления записана неверно")
	errUnsafeRemoval      = errors.New("этот каталог нельзя удалять целиком")
	errNoLibraryAccess    = errors.New("библиотека недоступна")
	errNothingToRemove    = errors.New("удалять с диска нечего")
)

type RemovalInfo struct {
	GameID           string        `json:"gameId"`
	Title            string        `json:"title"`
	InstallDir       string        `json:"installDir"`
	InstallType      string        `json:"installType"`
	Method           RemovalMethod `json:"method"`
	Owned            bool          `json:"owned"`
	UninstallUnknown bool          `json:"uninstallUnknown"`
	SizeBytes        int64         `json:"sizeBytes"`
	SizeUnknown      bool          `json:"sizeUnknown"`
	DirMissing       bool          `json:"dirMissing"`
	QuietUninstall   bool          `json:"quietUninstall"`
	Running          bool          `json:"running"`
	Busy             bool          `json:"busy"`
	DownloadID       string        `json:"downloadId"`
	DownloadPresent  bool          `json:"downloadPresent"`
	DownloadSeeding  bool          `json:"downloadSeeding"`
	DownloadBytes    int64         `json:"downloadBytes"`
	DownloadPath     string        `json:"downloadPath"`
}

type RemoveOptions struct {
	DeleteFiles    bool `json:"deleteFiles"`
	DeleteDownload bool `json:"deleteDownload"`
	KeepInLibrary  bool `json:"keepInLibrary"`
}

type removalPlan struct {
	game        library.Game
	method      RemovalMethod
	uninstall   library.Uninstall
	spec        runSpec
	owned       bool
	installType string
	unknown     bool
}

func (s *Service) InspectRemoval(gameID string) (RemovalInfo, error) {
	plan, err := s.removalPlan(gameID)
	if err != nil {
		return RemovalInfo{}, err
	}
	missing, err := dirMissing(plan.game.InstallDir)
	if err != nil {
		return RemovalInfo{}, err
	}
	info := RemovalInfo{
		GameID:           plan.game.ID,
		Title:            plan.game.Title,
		InstallDir:       plan.game.InstallDir,
		InstallType:      plan.installType,
		Method:           plan.method,
		Owned:            plan.owned,
		UninstallUnknown: plan.unknown,
		SizeBytes:        plan.game.SizeBytes,
		SizeUnknown:      plan.game.SizeUnknown,
		DirMissing:       missing,
		QuietUninstall:   plan.method == RemovalInstaller && plan.spec.Background,
		Running:          s.library.IsRunning(gameID),
		Busy:             s.gameBusy(gameID, plan.game.InstallDir),
		DownloadID:       plan.game.SourceDownloadID,
	}
	d, present, err := s.downloadOf(plan.game.SourceDownloadID)
	if err != nil {
		return RemovalInfo{}, err
	}
	if present {
		info.DownloadPresent = true
		info.DownloadSeeding = d.Seeding
		info.DownloadBytes = d.Downloaded
		info.DownloadPath = downloadRoot(d)
		if info.DownloadPath == "" {
			info.DownloadPath = d.Destination
		}
	}
	return info, nil
}

func (s *Service) RemoveGame(gameID string, opts RemoveOptions) error {
	plan, err := s.removalPlan(gameID)
	if err != nil {
		return err
	}
	game := plan.game
	if opts.DeleteFiles && !plan.owned {
		return fmt.Errorf("%w: %s", errUnsafeRemoval, game.InstallDir)
	}
	if s.library.IsRunning(gameID) {
		return errGameRunning
	}
	if s.gameBusy(gameID, game.InstallDir) {
		return errGameBusy
	}
	d, present, err := s.downloadOf(game.SourceDownloadID)
	if err != nil {
		return err
	}
	if present && d.Seeding && sharesPath(downloadRoot(d), game.InstallDir) {
		return errRemoveSeeding
	}

	deleteFiles := opts.DeleteFiles && plan.owned && plan.game.InstallDir != ""
	if opts.KeepInLibrary && !deleteFiles && plan.method != RemovalInstaller {
		return errNothingToRemove
	}
	if deleteFiles {
		if err := removableDir(game.InstallDir); err != nil {
			return err
		}
	}

	if plan.method == RemovalInstaller {
		if err := s.runUninstaller(s.baseContext(), plan); err != nil {
			return err
		}
	}
	if deleteFiles {
		if err := s.removeInstallDir(game.InstallDir); err != nil {
			return err
		}
	}

	if err := s.forgetInstallations(gameID); err != nil {
		return err
	}
	if opts.KeepInLibrary {
		if err := s.library.MarkUninstalled(gameID); err != nil {
			return err
		}
	} else if err := s.library.RemoveGame(gameID); err != nil {
		return err
	}
	slog.Info("game removed", "id", gameID, "title", game.Title, "method", plan.method,
		"files", deleteFiles, "kept", opts.KeepInLibrary)

	if opts.DeleteDownload && present {
		if err := s.downloads.DeleteData(game.SourceDownloadID); err != nil {
			return fmt.Errorf("удалить загрузку: %w", err)
		}
	}
	return nil
}

func (s *Service) removalPlan(gameID string) (removalPlan, error) {
	if s.library == nil {
		return removalPlan{}, errNoLibraryAccess
	}
	game, err := s.library.Find(gameID)
	if err != nil {
		return removalPlan{}, err
	}
	plan := removalPlan{
		game:        game,
		uninstall:   game.Uninstall,
		owned:       game.Owned,
		installType: game.InstallType,
		unknown:     game.UninstallUnknown,
	}
	// Записи, сделанные до появления этих полей, и записи, восстановленные
	// сканированием: метка в каталоге доказывает, что установку делали мы.
	if plan.installType == "" && !plan.owned && game.InstallDir != "" {
		marker, err := library.ReadMarker(game.InstallDir)
		switch {
		case err == nil:
			plan.owned = true
			plan.installType = marker.InstallType
			if plan.uninstall.Empty() {
				plan.uninstall = marker.Uninstall
			}
		case !errors.Is(err, fs.ErrNotExist):
			return removalPlan{}, err
		}
	}
	if !plan.uninstall.Empty() {
		spec, ok, err := usableUninstallSpec(plan.uninstall)
		if err != nil {
			return removalPlan{}, err
		}
		if ok {
			plan.spec = spec
			plan.method = RemovalInstaller
		}
	}
	switch {
	case plan.method != "":
	case plan.owned && game.InstallDir != "":
		plan.method = RemovalFiles
	default:
		plan.method = RemovalRecord
	}
	return plan, nil
}

// usableUninstallSpec отделяет «деинсталлятора нет» от «деинсталлятор прочитать
// не удалось». Первое — обычное состояние: игру могли удалить мимо лаунчера, и
// тогда запись обязана удаляться как файлы или как запись, иначе она застрянет
// в библиотеке навсегда. Второе — ошибка, и она доходит до вызывающего.
func usableUninstallSpec(u library.Uninstall) (runSpec, bool, error) {
	spec, err := uninstallSpec(u)
	switch {
	case err == nil:
		return spec, true, nil
	case errors.Is(err, errNoUninstaller), errors.Is(err, errBadCommand),
		errors.Is(err, errNoExecutable), errors.Is(err, fs.ErrNotExist):
		return runSpec{}, false, nil
	default:
		return runSpec{}, false, err
	}
}

func (s *Service) gameBusy(gameID, installDir string) bool {
	s.mu.Lock()
	for _, item := range s.items {
		if !active(item.Status) {
			continue
		}
		if item.GameID == gameID || sharesPath(item.Destination, installDir) {
			s.mu.Unlock()
			return true
		}
	}
	busy := s.busy
	s.mu.Unlock()
	if busy == nil {
		return false
	}
	return busy(gameID)
}

func (s *Service) downloadOf(id string) (download.Download, bool, error) {
	if id == "" || s.downloads == nil {
		return download.Download{}, false, nil
	}
	d, err := s.downloads.Get(id)
	if errors.Is(err, download.ErrNotFound) {
		return download.Download{}, false, nil
	}
	if err != nil {
		return download.Download{}, false, err
	}
	return d, true, nil
}

func (s *Service) forgetInstallations(gameID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	kept := make([]*Installation, 0, len(s.items))
	dropped := make([]string, 0, 2)
	for _, item := range s.items {
		if item.GameID == gameID && !active(item.Status) {
			dropped = append(dropped, item.ID)
			continue
		}
		kept = append(kept, item)
	}
	if len(dropped) == 0 {
		return nil
	}
	previous := s.items
	s.items = kept
	if err := s.persistNowLocked(); err != nil {
		s.items = previous
		return err
	}
	for _, id := range dropped {
		emit(eventRemoved, RemovedEvent{ID: id})
	}
	return nil
}

func (s *Service) removeInstallDir(dir string) error {
	staged, err := s.stageForRemoval(dir)
	if err != nil {
		return err
	}
	if staged == "" {
		return nil
	}
	if err := os.RemoveAll(staged); err != nil {
		return fmt.Errorf("удалить %s: %w", dir, err)
	}
	return s.removals.drop(staged)
}

// stageForRemoval переименовывает каталог перед удалением: переименование либо
// проходит целиком, либо не трогает ничего, поэтому занятый каталог виден сразу,
// а не после того, как половина файлов уже удалена.
func (s *Service) stageForRemoval(dir string) (string, error) {
	missing, err := dirMissing(dir)
	if err != nil {
		return "", err
	}
	if missing {
		return "", nil
	}
	staged, err := freeRemovalPath(dir)
	if err != nil {
		return "", err
	}
	if err := s.removals.add(staged); err != nil {
		return "", err
	}
	if err := os.Rename(dir, staged); err != nil {
		locked := fmt.Errorf("%w: %w", errFilesLocked, err)
		if dropErr := s.removals.drop(staged); dropErr != nil {
			return "", errors.Join(locked, dropErr)
		}
		return "", locked
	}
	return staged, nil
}

func freeRemovalPath(dir string) (string, error) {
	base := dir + removingSuffix
	for attempt := 0; attempt < maxSuffixAttempt; attempt++ {
		candidate := base
		if attempt > 0 {
			candidate = fmt.Sprintf("%s-%d", base, attempt)
		}
		missing, err := dirMissing(candidate)
		if err != nil {
			return "", err
		}
		if missing {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("%w: %s", errFilesLocked, base)
}

func (s *Service) sweepRemovals() {
	paths, err := s.removals.load()
	if err != nil {
		slog.Error("load pending removals", "error", err)
		return
	}
	for _, path := range paths {
		if !strings.Contains(filepath.Base(path), removingSuffix) {
			slog.Error("unexpected pending removal", "path", path)
			continue
		}
		if err := os.RemoveAll(path); err != nil {
			slog.Warn("remove pending", "path", path, "error", err)
			continue
		}
		if err := s.removals.drop(path); err != nil {
			slog.Warn("forget pending removal", "path", path, "error", err)
			continue
		}
		slog.Info("pending removal finished", "path", path)
	}
}

func dirMissing(path string) (bool, error) {
	if strings.TrimSpace(path) == "" {
		return true, nil
	}
	_, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("stat %s: %w", path, err)
	}
	return false, nil
}

// removableDir не пускает удаление в корень тома и в каталог, до которого мы
// добрались по ссылке: RemoveAll по junction уносит содержимое цели.
func removableDir(dir string) error {
	clean := filepath.Clean(dir)
	if clean == "" || clean == "." {
		return errEmptyDestination
	}
	if !filepath.IsAbs(clean) {
		return fmt.Errorf("%w: %s", errUnsafeRemoval, dir)
	}
	parent := filepath.Dir(clean)
	if parent == clean || filepath.Dir(parent) == parent && filepath.Base(parent) == "" {
		return fmt.Errorf("%w: %s", errUnsafeRemoval, dir)
	}
	info, err := os.Lstat(clean)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("stat %s: %w", clean, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("%w: %s", errUnsafeRemoval, dir)
	}
	return nil
}

func downloadRoot(d download.Download) string {
	if d.Destination == "" || d.Name == "" {
		return ""
	}
	return filepath.Join(d.Destination, d.Name)
}

func sharesPath(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	return samePath(a, b) || inside(a, b) || inside(b, a)
}

func (s *Service) runUninstaller(ctx context.Context, plan removalPlan) error {
	spec := plan.spec
	if spec.Path == "" {
		return errNoUninstaller
	}
	slog.Info("uninstaller started", "game", plan.game.ID, "path", spec.Path, "quiet", spec.Background)
	code, err := s.runner.run(ctx, spec)
	if err != nil {
		return err
	}
	switch code {
	case 0, msiRebootPending, msiNotInstalled:
		return nil
	case msiCancelled:
		return errUninstallCancelled
	default:
		slog.Error("uninstaller exit code", "game", plan.game.ID, "path", spec.Path, "code", code)
		return fmt.Errorf("%w: %d", errUninstallFailed, code)
	}
}

// uninstallSpec собирает запуск деинсталлятора так, чтобы он не задавал
// вопросов. Ключи по определённому движку идут раньше строки QuietUninstallString:
// вендор пишет туда в лучшем случае /SILENT, который всё равно рисует окно, и
// никогда не пишет _?=, без которого удаление уходит в фон и его код возврата
// перестаёт что-либо значить.
func uninstallSpec(u library.Uninstall) (runSpec, error) {
	if u.ProductCode != "" {
		path, err := systemExecutable("msiexec.exe")
		if err != nil {
			return runSpec{}, err
		}
		args := []string{"/x", u.ProductCode, "/qn", "/norestart"}
		return runSpec{Path: path, Args: args, Background: true}, nil
	}
	spec, err := commandSpec(u.Command)
	if err != nil {
		if u.QuietCommand == "" {
			return runSpec{}, err
		}
		return quietCommandSpec(u.QuietCommand)
	}
	engine, err := DetectEngine(spec.Path)
	if err != nil {
		return runSpec{}, err
	}
	if quiet, ok := quietUninstallArgs(engine, filepath.Dir(spec.Path)); ok {
		spec.Args = append(spec.Args, quiet...)
		spec.Background = true
		return spec, nil
	}
	if u.QuietCommand != "" {
		return quietCommandSpec(u.QuietCommand)
	}
	return spec, nil
}

func quietCommandSpec(command string) (runSpec, error) {
	spec, err := commandSpec(command)
	if err != nil {
		return runSpec{}, err
	}
	spec.Background = true
	return spec, nil
}

// quietUninstallArgs: _?= обязан идти последним и удерживает деинсталлятор от
// перезапуска копией из %TEMP% — без него процесс возвращает управление сразу,
// его код возврата ничего не значит, а каталог удаляется из-под работающего
// удаления. Ценой остаётся сам unins000.exe, который снимет уборка остатков.
func quietUninstallArgs(engine Engine, dir string) ([]string, bool) {
	var args []string
	switch engine {
	case EngineInno:
		args = []string{"/VERYSILENT", "/SUPPRESSMSGBOXES", "/NORESTART"}
	case EngineNsis:
		args = []string{"/S"}
	default:
		return nil, false
	}
	if dir != "" {
		args = append(args, "_?="+filepath.Clean(dir))
	}
	return args, true
}

func commandSpec(command string) (runSpec, error) {
	path, args, err := splitCommand(command)
	if err != nil {
		return runSpec{}, err
	}
	resolved, err := resolveExecutable(path)
	if err != nil {
		return runSpec{}, err
	}
	return runSpec{Path: resolved, Args: args, Dir: filepath.Dir(resolved)}, nil
}

func splitCommand(command string) (string, []string, error) {
	command = strings.TrimSpace(command)
	if command == "" {
		return "", nil, errNoUninstaller
	}
	if strings.HasPrefix(command, `"`) {
		end := strings.Index(command[1:], `"`)
		if end < 0 {
			return "", nil, errBadCommand
		}
		return command[1 : end+1], splitArgs(command[end+2:]), nil
	}
	if idx := strings.Index(strings.ToLower(command), ".exe"); idx >= 0 {
		end := idx + len(".exe")
		return command[:end], splitArgs(command[end:]), nil
	}
	fields := splitArgs(command)
	if len(fields) == 0 {
		return "", nil, errBadCommand
	}
	return fields[0], fields[1:], nil
}

func splitArgs(tail string) []string {
	out := make([]string, 0, 4)
	current := strings.Builder{}
	quoted := false
	for _, r := range tail {
		switch {
		case r == '"':
			quoted = !quoted
		case r == ' ' && !quoted:
			if current.Len() > 0 {
				out = append(out, current.String())
				current.Reset()
			}
		default:
			current.WriteRune(r)
		}
	}
	if current.Len() > 0 {
		out = append(out, current.String())
	}
	return out
}
