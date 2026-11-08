package emulators

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"unicode"

	"typhon/internal/platform"
	"typhon/internal/settings"
	"typhon/internal/uierr"
)

const (
	StatusNotSet      = "not_set"
	StatusReady       = "ready"
	StatusMissing     = "missing"
	StatusUnavailable = "unavailable"

	// WarnDiscordExitHang: Vita3K on macOS hangs on exit with its Discord Rich
	// Presence on, so a session never ends and an install is never remembered.
	WarnDiscordExitHang = "discord_exit_hang"

	macAppSuffix = ".app"
)

var (
	ErrUnknownPlatform = uierr.New("emulators.unknown_platform", "неизвестная игровая платформа")
	ErrNotConfigured   = uierr.New("emulators.not_configured", "эмулятор для этой платформы не указан")
	ErrFileMissing     = uierr.New("emulators.file_missing", "файл эмулятора не найден")
	ErrFileUnreadable  = uierr.New("emulators.file_unreadable", "файл эмулятора недоступен")
	ErrPathRelative    = uierr.New("emulators.path_relative", "путь к эмулятору должен быть абсолютным")
	ErrPathInvalid     = uierr.New("emulators.path_invalid", "путь к эмулятору содержит недопустимые символы")
	ErrNotRegularFile  = uierr.New("emulators.not_regular_file", "эмулятор должен быть обычным файлом, а не папкой или ссылкой")
	ErrNotExe          = uierr.New("emulators.not_exe", "эмулятор должен быть файлом .exe")
	ErrWindowsProgram  = uierr.New("emulators.windows_program", "на этой системе эмулятор запускается нативно, программа Windows не подходит")
	ErrBadAppBundle    = uierr.New("emulators.bad_app_bundle", "в приложении не найден исполняемый файл эмулятора")
	ErrSaveFailed      = uierr.New("emulators.save_failed", "не удалось сохранить настройки эмулятора")
	ErrBIOSMissing     = uierr.New("emulators.bios_missing", "эмулятор не найдёт BIOS: положите дамп BIOS своей приставки в папку эмулятора")
	ErrBIOSUnknown     = uierr.New("emulators.bios_dir_unknown", "папка BIOS этого эмулятора неизвестна")
	ErrPKGInvalid      = uierr.New("emulators.pkg_invalid", "файл .pkg повреждён или это не пакет PS3")
	ErrPKGInstalling   = uierr.New("emulators.pkg_installing", "эмулятор устанавливает игру из пакета: подтвердите установку в его окне и запустите игру ещё раз")
	ErrEmulatorRunning = uierr.New("emulators.already_running", "эмулятор уже открыт: закройте его, чтобы запустить игру")

	ErrImagePathEmpty    = uierr.New("emulators.image_path_empty", "не указан файл образа")
	ErrImagePathRelative = uierr.New("emulators.image_path_relative", "путь к образу должен быть абсолютным")
	ErrImageExtension    = uierr.New("emulators.image_extension", "расширение файла не подходит этой платформе")
	ErrImageMissing      = uierr.New("emulators.image_missing", "файл образа не найден")
	ErrImageUnreadable   = uierr.New("emulators.image_unreadable", "файл образа недоступен")
	ErrImageNotFile      = uierr.New("emulators.image_not_file", "образ должен быть обычным файлом, а не папкой или ссылкой")
	ErrImageManualOnly   = uierr.New("emulators.image_manual_only", "такой файл лаунчер не запускает: установите игру в эмуляторе вручную")
	ErrImageIncomplete   = uierr.New("emulators.image_incomplete", "рядом с файлом нет остальных данных игры")
	ErrImageInStorage    = uierr.New("emulators.image_in_storage", "это обновление или дополнение из папки эмулятора, а не игра")
	ErrSFOMissing        = uierr.New("emulators.sfo_missing", "в игре нет файла param.sfo")
	ErrSFOInvalid        = uierr.New("emulators.sfo_invalid", "файл param.sfo игры повреждён")
	ErrTitleIDInvalid    = uierr.New("emulators.title_id_invalid", "в param.sfo нет правильного кода игры")
	ErrPackageInvalid    = uierr.New("emulators.vpk_invalid", "файл .vpk повреждён или содержит недопустимые пути")
	ErrTitleEmpty        = uierr.New("emulators.title_empty", "не указано название игры")
	ErrImageInside       = uierr.New("emulators.image_inside", "в этой папке лежит образ игры из библиотеки")
	ErrCoverNotFound     = uierr.New("emulators.cover_not_found", "обложка для этой игры не найдена")
	ErrCoverLookup       = uierr.New("emulators.cover_lookup_failed", "не удалось выполнить поиск обложки")

	// ErrUnsupported answers an operation that treats the game's folder or
	// file as a Windows installation: an image lies in any folder of the
	// user's, and that folder is not the game's.
	ErrUnsupported = uierr.New("emulators.game_unsupported", "для игры на эмуляторе это действие недоступно")
)

// PlatformStatus is one row of the emulator settings. Path keeps the stored
// value in every status except StatusNotSet, so the UI can say what went
// missing. Reason explains StatusUnavailable.
type PlatformStatus struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Emulator   string   `json:"emulator"`
	Homepage   string   `json:"homepage"`
	Extensions []string `json:"extensions"`
	ImageNames []string `json:"imageNames"`
	Path       string   `json:"path"`
	Status     string   `json:"status"`
	Reason     string   `json:"reason"`
	// Warnings are codes of known trouble with a ready emulator.
	Warnings []string `json:"warnings"`
}

// Found is an emulator already present on this machine. Nothing is stored until
// the user confirms it through SetPath.
type Found struct {
	Platform string `json:"platform"`
	Path     string `json:"path"`
}

// DetectResult counts the places that could not be checked: an empty Found with
// Unreadable above zero does not mean the emulator is absent.
type DetectResult struct {
	Found      []Found `json:"found"`
	Unreadable int     `json:"unreadable"`
}

type settingsStore interface {
	GetSettings() settings.Settings
	Update(mutate func(*settings.Settings) error) (settings.Settings, error)
}

type Service struct {
	settings settingsStore
	goos     string
	roots    func() ([]string, error)
	lstat    func(string) (fs.FileInfo, error)
	// home is a field so that tests never read the player's real config.
	home func() (StorageHome, error)
	// biosEnv is a field for the same reason.
	biosEnv func() BIOSEnv
}

func NewService(store *settings.Service) (*Service, error) {
	if store == nil {
		return nil, errors.New("настройки недоступны")
	}
	return &Service{settings: store, goos: runtime.GOOS, roots: systemRoots, lstat: os.Lstat, home: DefaultStorageHome, biosEnv: DefaultBIOSEnv}, nil
}

func (s *Service) List(ctx context.Context) ([]PlatformStatus, error) {
	paths := s.settings.GetSettings().EmulatorPaths
	out := make([]PlatformStatus, 0, len(platforms))
	for _, p := range platforms {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		out = append(out, s.statusOf(p, paths[p.ID]))
	}
	return out, nil
}

func (s *Service) statusOf(p Platform, path string) PlatformStatus {
	row := PlatformStatus{
		ID: p.ID, Name: p.Name, Emulator: p.Emulator, Homepage: p.Homepage,
		Extensions: append(slices.Clone(p.Extensions), p.packages...), ImageNames: p.ImageNames(), Path: path, Status: StatusNotSet,
		Warnings: []string{},
	}
	if path == "" {
		return row
	}
	err := s.checkProgram(path)
	switch {
	case err == nil:
		row.Status = StatusReady
		if s.goos == "darwin" && p.DiscordPresenceOn(path, s.home) {
			row.Warnings = append(row.Warnings, WarnDiscordExitHang)
		}
	case errors.Is(err, ErrFileMissing):
		row.Status = StatusMissing
	default:
		row.Status = StatusUnavailable
		row.Reason = uierr.Code(err)
	}
	return row
}

// SetPath stores the program of a console; an empty path forgets it. On macOS
// an .app bundle is reduced to its binary: the launcher starts a process and
// has to track its pid, which opening a bundle through the system would hide.
func (s *Service) SetPath(platform, path string) (PlatformStatus, error) {
	p, ok := Lookup(platform)
	if !ok {
		return PlatformStatus{}, fmt.Errorf("%w: %q", ErrUnknownPlatform, platform)
	}
	path = strings.TrimSpace(path)
	if path != "" {
		resolved, err := s.normalize(p, path)
		if err != nil {
			return PlatformStatus{}, err
		}
		path = resolved
	}
	_, err := s.settings.Update(func(next *settings.Settings) error {
		paths := maps.Clone(next.EmulatorPaths)
		if paths == nil {
			paths = map[string]string{}
		}
		if path == "" {
			delete(paths, platform)
		} else {
			paths[platform] = path
		}
		next.EmulatorPaths = paths
		return nil
	})
	if err != nil {
		return PlatformStatus{}, uierr.Wrap(uierr.Code(ErrSaveFailed), fmt.Errorf("сохранить путь к эмулятору: %w", err))
	}
	return s.statusOf(p, path), nil
}

func (s *Service) normalize(p Platform, path string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("%w: %s", ErrPathRelative, path)
	}
	if strings.IndexFunc(path, unicode.IsControl) >= 0 {
		return "", ErrPathInvalid
	}
	path = filepath.Clean(path)
	if s.goos == "darwin" && strings.EqualFold(filepath.Ext(path), macAppSuffix) {
		binary, err := bundleBinary(p, path)
		if err != nil {
			return "", err
		}
		path = binary
	}
	if err := s.checkProgram(path); err != nil {
		return "", err
	}
	return path, nil
}

// checkProgram is run on every launch as well as on saving: the settings file
// can be edited by hand and the file can change after it was chosen.
func (s *Service) checkProgram(path string) error {
	if !filepath.IsAbs(path) {
		return fmt.Errorf("%w: %s", ErrPathRelative, path)
	}
	info, err := s.lstat(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return fmt.Errorf("%w: %s", ErrFileMissing, path)
	case err != nil:
		return uierr.Wrap(uierr.Code(ErrFileUnreadable), fmt.Errorf("файл эмулятора %s: %w", path, err))
	case !info.Mode().IsRegular():
		return fmt.Errorf("%w: %s", ErrNotRegularFile, path)
	}
	if s.goos == "windows" {
		if !strings.EqualFold(filepath.Ext(path), ".exe") {
			return fmt.Errorf("%w: %s", ErrNotExe, path)
		}
		return nil
	}
	if IsWindowsProgram(path) {
		return fmt.Errorf("%w: %s", ErrWindowsProgram, path)
	}
	return nil
}

// IsWindowsProgram is the one list of what needs Windows or a wine runtime.
func IsWindowsProgram(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".exe", ".bat", ".cmd", ".com", ".msi":
		return true
	default:
		return false
	}
}

func bundleBinary(p Platform, bundle string) (string, error) {
	dir := filepath.Join(bundle, "Contents", "MacOS")
	entries, err := os.ReadDir(dir)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return "", fmt.Errorf("%w: %s", ErrBadAppBundle, bundle)
	case err != nil:
		return "", uierr.Wrap(uierr.Code(ErrFileUnreadable), fmt.Errorf("приложение %s: %w", bundle, err))
	}
	var files []string
	for _, entry := range entries {
		if entry.Type().IsRegular() {
			files = append(files, entry.Name())
		}
	}
	for _, name := range p.macBinaries {
		if slices.Contains(files, name) {
			return filepath.Join(dir, name), nil
		}
	}
	if len(files) == 1 {
		return filepath.Join(dir, files[0]), nil
	}
	return "", fmt.Errorf("%w: %s", ErrBadAppBundle, bundle)
}

// ImageConsoles lists the consoles whose game layout the files around image have,
// so that the add form offers only those. An empty list proves nothing: the form
// keeps what the file name alone allows.
func (s *Service) ImageConsoles(ctx context.Context, image string) ([]string, error) {
	out := []string{}
	if !filepath.IsAbs(image) || strings.IndexFunc(image, unicode.IsControl) >= 0 {
		return out, nil
	}
	image = filepath.Clean(image)
	for _, p := range platforms {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if p.FitsLayout(image) {
			out = append(out, p.ID)
		}
	}
	return out, nil
}

// ImageTitle suggests a name for the add form from the game's own param.sfo.
// An empty answer means there is nothing to suggest; the form then waits for the
// user's name as before.
func (s *Service) ImageTitle(ctx context.Context, image, platform string) (string, error) {
	p, ok := Lookup(platform)
	if !ok {
		return "", fmt.Errorf("%w: %q", ErrUnknownPlatform, platform)
	}
	if !filepath.IsAbs(image) || strings.IndexFunc(image, unicode.IsControl) >= 0 {
		return "", ErrPathInvalid
	}
	return p.ReadTitle(ctx, filepath.Clean(image))
}

// BIOSFolder is the folder the configured emulator of a console reads its BIOS
// from, for the player to put a dump into.
func (s *Service) BIOSFolder(platformID string) (string, error) {
	p, ok := Lookup(platformID)
	if !ok {
		return "", fmt.Errorf("%w: %q", ErrUnknownPlatform, platformID)
	}
	program, err := s.Resolve(platformID)
	if err != nil {
		return "", err
	}
	dir, ok := p.BIOSDir(program, s.goos, s.biosEnv())
	if !ok {
		return "", ErrBIOSUnknown
	}
	return dir, nil
}

// OpenBIOSFolder shows that folder, made first if the emulator has not yet: it
// is a folder the emulator itself creates and reads, inside its own data.
func (s *Service) OpenBIOSFolder(platformID string) error {
	dir, err := s.BIOSFolder(platformID)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("создать папку BIOS: %w", err)
	}
	if err := platform.OpenFolder(dir); err != nil {
		return fmt.Errorf("открыть папку BIOS: %w", err)
	}
	return nil
}

// CloseEmulator closes the open copies of a console's emulator that would stop
// a game from starting, after the player agreed to it.
func (s *Service) CloseEmulator(ctx context.Context, platformID string) error {
	p, ok := Lookup(platformID)
	if !ok {
		return fmt.Errorf("%w: %q", ErrUnknownPlatform, platformID)
	}
	if !p.SingleInstance() {
		return nil
	}
	program, err := s.Resolve(platformID)
	if err != nil {
		return err
	}
	return CloseRunning(ctx, program)
}

// Resolve gives the program to start for a console.
//
//wails:ignore
func (s *Service) Resolve(platform string) (string, error) {
	if _, ok := Lookup(platform); !ok {
		return "", fmt.Errorf("%w: %q", ErrUnknownPlatform, platform)
	}
	path := s.settings.GetSettings().EmulatorPaths[platform]
	if path == "" {
		return "", ErrNotConfigured
	}
	if err := s.checkProgram(path); err != nil {
		return "", err
	}
	return path, nil
}

// OfficialPage is the only place the launcher sends a user for an emulator.
func (s *Service) OfficialPage(platform string) (string, error) {
	p, ok := Lookup(platform)
	if !ok {
		return "", fmt.Errorf("%w: %q", ErrUnknownPlatform, platform)
	}
	return p.Homepage, nil
}

// Detect looks for emulators already installed in the usual places. It only
// reports what it found; the choice is the user's.
func (s *Service) Detect(ctx context.Context) (DetectResult, error) {
	roots, err := s.roots()
	if err != nil {
		return DetectResult{}, fmt.Errorf("папки поиска эмуляторов: %w", err)
	}
	result := DetectResult{Found: []Found{}}
	for _, p := range platforms {
		for _, candidate := range s.candidates(p, roots) {
			if err := ctx.Err(); err != nil {
				return DetectResult{}, err
			}
			info, err := s.lstat(candidate)
			switch {
			case err == nil:
				if info.Mode().IsRegular() {
					result.Found = append(result.Found, Found{Platform: p.ID, Path: candidate})
				}
			case errors.Is(err, fs.ErrNotExist):
			default:
				result.Unreadable++
			}
		}
	}
	return result, nil
}

func (s *Service) candidates(p Platform, roots []string) []string {
	var out []string
	for _, root := range roots {
		if s.goos == "windows" {
			for _, dir := range p.dirs {
				for _, exe := range p.windowsExes {
					out = append(out, filepath.Join(root, dir, exe))
				}
			}
			continue
		}
		for _, bundle := range p.macBundles {
			for _, binary := range p.macBinaries {
				out = append(out, filepath.Join(root, bundle, "Contents", "MacOS", binary))
			}
		}
	}
	return out
}

func systemRoots() ([]string, error) {
	if runtime.GOOS != "windows" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("домашняя папка: %w", err)
		}
		return []string{"/Applications", filepath.Join(home, "Applications")}, nil
	}
	var roots []string
	add := func(dir string) {
		if dir != "" && !slices.ContainsFunc(roots, func(seen string) bool { return strings.EqualFold(seen, dir) }) {
			roots = append(roots, dir)
		}
	}
	add(os.Getenv("ProgramW6432"))
	add(os.Getenv("ProgramFiles"))
	add(os.Getenv("ProgramFiles(x86)"))
	if local := os.Getenv("LOCALAPPDATA"); local != "" {
		add(local)
		add(filepath.Join(local, "Programs"))
	}
	return roots, nil
}
