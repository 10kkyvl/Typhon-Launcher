package settings

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"typhon/internal/storage"

	"github.com/wailsapp/wails/v3/pkg/application"
)

const (
	CleanupKeep   = "keep"
	CleanupAsk    = "ask"
	CleanupDelete = "delete"

	RefreshManual   = "manual"
	RefreshHourly   = "1h"
	RefreshSixHours = "6h"
	RefreshHalfDay  = "12h"
	RefreshDaily    = "24h"

	KeepPreviousOff         = "off"
	KeepPreviousFirstLaunch = "first_launch"
	KeepPreviousDay         = "24h"

	LibraryFolderName = "TyphonLibrary"

	dirGames       = "Games"
	dirDownloads   = "Downloads"
	dirScreenshots = "Screenshots"
)

var (
	ErrLibraryNotConfigured = errors.New("библиотека не настроена")
	ErrLibraryPathRelative  = errors.New("путь библиотеки должен быть абсолютным")
	ErrLibraryPathRoot      = errors.New("библиотека не может быть корнем диска")
	ErrLibraryParentEmpty   = errors.New("не выбрана папка для библиотеки")
)

type Settings struct {
	Theme                  string  `json:"theme"`
	Language               string  `json:"language"`
	UIScale                float64 `json:"uiScale"`
	LibraryPath            string  `json:"libraryPath"`
	DownloadsPath          string  `json:"downloadsPath"`
	GamesPath              string  `json:"gamesPath"`
	ScreenshotsPath        string  `json:"screenshotsPath"`
	LaunchOnStartup        bool    `json:"launchOnStartup"`
	MinimizeToTray         bool    `json:"minimizeToTray"`
	DiscordRichPresence    bool    `json:"discordRichPresence"`
	HardwareAcceleration   bool    `json:"hardwareAcceleration"`
	AnimationsEnabled      bool    `json:"animationsEnabled"`
	MaxActiveDownloads     int     `json:"maxActiveDownloads"`
	DownloadRateLimit      int64   `json:"downloadRateLimit"`
	UploadRateLimit        int64   `json:"uploadRateLimit"`
	UploadWhileDownloading bool    `json:"uploadWhileDownloading"`
	SeedAfterDownload      bool    `json:"seedAfterDownload"`
	InstallCleanupPolicy   string  `json:"installCleanupPolicy"`
	AutoInstall            bool    `json:"autoInstall"`
	SourceRefreshInterval  string  `json:"sourceRefreshInterval"`
	VerifyAfterInstall     bool    `json:"verifyAfterInstall"`
	InstallSkipShortcuts   bool    `json:"installSkipShortcuts"`
	InstallSkipExtras      bool    `json:"installSkipExtras"`

	UpdateCheckAutomatically bool   `json:"updateCheckAutomatically"`
	UpdateAutoDownload       bool   `json:"updateAutoDownload"`
	UpdateAutoInstall        bool   `json:"updateAutoInstall"`
	UpdateSaveBackup         bool   `json:"updateSaveBackup"`
	KeepPreviousVersion      string `json:"keepPreviousVersion"`
	AllowTorrentReuse        bool   `json:"allowTorrentReuse"`
}

func Defaults() Settings {
	return Settings{
		Theme:                  "dark",
		Language:               "ru",
		UIScale:                1,
		LaunchOnStartup:        false,
		MinimizeToTray:         true,
		DiscordRichPresence:    false,
		HardwareAcceleration:   true,
		AnimationsEnabled:      true,
		MaxActiveDownloads:     2,
		DownloadRateLimit:      0,
		UploadRateLimit:        0,
		UploadWhileDownloading: false,
		SeedAfterDownload:      false,
		InstallCleanupPolicy:   CleanupDelete,
		AutoInstall:            false,
		SourceRefreshInterval:  RefreshSixHours,
		VerifyAfterInstall:     true,
		InstallSkipShortcuts:   true,
		InstallSkipExtras:      true,

		UpdateCheckAutomatically: true,
		UpdateAutoDownload:       false,
		UpdateAutoInstall:        false,
		UpdateSaveBackup:         true,
		KeepPreviousVersion:      KeepPreviousFirstLaunch,
		AllowTorrentReuse:        true,
	}
}

func normalizeLibraryPath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", nil
	}
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("%w: %s", ErrLibraryPathRelative, path)
	}
	path = filepath.Clean(path)
	if filepath.Dir(path) == path {
		return "", fmt.Errorf("%w: %s", ErrLibraryPathRoot, path)
	}
	return path, nil
}

func derivePaths(s Settings) Settings {
	if s.LibraryPath == "" {
		s.GamesPath = ""
		s.DownloadsPath = ""
		s.ScreenshotsPath = ""
		return s
	}
	s.GamesPath = filepath.Join(s.LibraryPath, dirGames)
	s.DownloadsPath = filepath.Join(s.LibraryPath, dirDownloads)
	s.ScreenshotsPath = filepath.Join(s.LibraryPath, dirScreenshots)
	return s
}

func legacyLibraryPath(gamesPath string) string {
	gamesPath = strings.TrimSpace(gamesPath)
	if gamesPath == "" || !filepath.IsAbs(gamesPath) {
		return ""
	}
	root := filepath.Clean(gamesPath)
	if filepath.Base(root) == dirGames {
		root = filepath.Dir(root)
	}
	if filepath.Dir(root) == root {
		return ""
	}
	return root
}

func sanitize(s Settings) (Settings, error) {
	library, err := normalizeLibraryPath(s.LibraryPath)
	if err != nil {
		return Settings{}, err
	}
	s.LibraryPath = library
	s = derivePaths(s)
	if s.UIScale < 0.9 || s.UIScale > 1.25 {
		s.UIScale = 1
	}
	if s.MaxActiveDownloads < 1 {
		s.MaxActiveDownloads = 1
	}
	if s.MaxActiveDownloads > 10 {
		s.MaxActiveDownloads = 10
	}
	if s.DownloadRateLimit < 0 {
		s.DownloadRateLimit = 0
	}
	if s.UploadRateLimit < 0 {
		s.UploadRateLimit = 0
	}
	switch s.InstallCleanupPolicy {
	case CleanupKeep, CleanupAsk, CleanupDelete:
	default:
		s.InstallCleanupPolicy = CleanupDelete
	}
	switch s.SourceRefreshInterval {
	case RefreshManual, RefreshHourly, RefreshSixHours, RefreshHalfDay, RefreshDaily:
	default:
		s.SourceRefreshInterval = RefreshSixHours
	}
	switch s.KeepPreviousVersion {
	case KeepPreviousOff, KeepPreviousFirstLaunch, KeepPreviousDay:
	default:
		s.KeepPreviousVersion = KeepPreviousFirstLaunch
	}
	return s, nil
}

var migrateConfigDirOnce sync.Once

func ConfigDir() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	newDir := filepath.Join(dir, "Typhon")
	migrateConfigDirOnce.Do(func() {
		oldDir := filepath.Join(dir, "Aurora")
		if _, err := os.Stat(newDir); !errors.Is(err, os.ErrNotExist) {
			return
		}
		if _, err := os.Stat(oldDir); errors.Is(err, os.ErrNotExist) {
			return
		}
		if err := os.Rename(oldDir, newDir); err != nil {
			slog.Warn("migrate config dir", "from", oldDir, "to", newDir, "error", err)
		}
	})
	return newDir, nil
}

type Service struct {
	mu      sync.Mutex
	path    string
	current Settings
	subs    map[int]func(Settings)
	nextSub int
}

func NewService() (*Service, error) {
	dir, err := ConfigDir()
	if err != nil {
		return nil, fmt.Errorf("resolve config dir: %w", err)
	}
	return NewServiceAt(filepath.Join(dir, "settings.json"))
}

func NewServiceAt(path string) (*Service, error) {
	if path == "" {
		return nil, errors.New("settings path unavailable")
	}
	s := &Service{current: Defaults(), path: path}
	current, err := s.load()
	if err != nil {
		return nil, err
	}
	s.current = current
	return s, nil
}

func (s *Service) load() (Settings, error) {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		return Defaults(), nil
	}
	if err != nil {
		return Settings{}, fmt.Errorf("read settings %s: %w", s.path, err)
	}
	loaded := Defaults()
	if err := json.Unmarshal(data, &loaded); err != nil {
		return Settings{}, fmt.Errorf("parse settings %s: %w", s.path, err)
	}
	if loaded.LibraryPath == "" {
		loaded.LibraryPath = legacyLibraryPath(loaded.GamesPath)
	}
	current, err := sanitize(loaded)
	if err != nil {
		return Settings{}, fmt.Errorf("settings %s: %w", s.path, err)
	}
	return current, nil
}

//wails:ignore
func (s *Service) Subscribe(fn func(Settings)) func() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.subs == nil {
		s.subs = map[int]func(Settings){}
	}
	id := s.nextSub
	s.nextSub++
	s.subs[id] = fn
	return func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		delete(s.subs, id)
	}
}

func (s *Service) GetSettings() Settings {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.current
}

func (s *Service) SaveSettings(next Settings) error {
	next, subs, err := s.persist(next)
	if err != nil {
		return err
	}
	if app := application.Get(); app != nil {
		app.Event.Emit("settings:updated", next)
	}
	for _, notify := range subs {
		notify(next)
	}
	return nil
}

func (s *Service) persist(next Settings) (Settings, []func(Settings), error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	next, err := sanitize(next)
	if err != nil {
		return Settings{}, nil, err
	}
	if s.path == "" {
		return next, nil, errors.New("settings path unavailable")
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return next, nil, fmt.Errorf("create config dir: %w", err)
	}
	data, err := json.MarshalIndent(next, "", "  ")
	if err != nil {
		return next, nil, err
	}
	if err := storage.WriteAtomic(s.path, data); err != nil {
		return next, nil, fmt.Errorf("write settings: %w", err)
	}
	s.current = next

	subs := make([]func(Settings), 0, len(s.subs))
	for _, fn := range s.subs {
		subs = append(subs, fn)
	}
	return next, subs, nil
}

func libraryRootFor(parent string) (string, error) {
	parent = strings.TrimSpace(parent)
	if parent == "" {
		return "", ErrLibraryParentEmpty
	}
	if !filepath.IsAbs(parent) {
		return "", fmt.Errorf("%w: %s", ErrLibraryPathRelative, parent)
	}
	parent = filepath.Clean(parent)
	root := parent
	if !strings.EqualFold(filepath.Base(parent), LibraryFolderName) {
		root = filepath.Join(parent, LibraryFolderName)
	}
	return normalizeLibraryPath(root)
}

func (s *Service) ProposeLibraryPath(parent string) (string, error) {
	return libraryRootFor(parent)
}

func (s *Service) SetupLibrary(parent string) (Settings, error) {
	root, err := libraryRootFor(parent)
	if err != nil {
		return Settings{}, err
	}
	if err := createLibrary(root); err != nil {
		return Settings{}, err
	}
	next := s.GetSettings()
	next.LibraryPath = root
	if err := s.SaveSettings(next); err != nil {
		return Settings{}, err
	}
	return s.GetSettings(), nil
}

func createLibrary(root string) error {
	dirs := []string{
		root,
		filepath.Join(root, dirGames),
		filepath.Join(root, dirDownloads),
		filepath.Join(root, dirScreenshots),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("создать папку %s: %w", dir, err)
		}
		if err := checkWritable(dir); err != nil {
			return err
		}
	}
	return nil
}

func checkWritable(dir string) error {
	f, err := os.CreateTemp(dir, ".typhon-probe-*")
	if err != nil {
		return fmt.Errorf("нет доступа на запись в %s: %w", dir, err)
	}
	name := f.Name()
	writeErr := writeProbe(f)
	closeErr := f.Close()
	removeErr := os.Remove(name)
	switch {
	case writeErr != nil:
		return fmt.Errorf("нет доступа на запись в %s: %w", dir, writeErr)
	case closeErr != nil:
		return fmt.Errorf("нет доступа на запись в %s: %w", dir, closeErr)
	case removeErr != nil:
		return fmt.Errorf("нет доступа на запись в %s: %w", dir, removeErr)
	}
	return nil
}

func writeProbe(f *os.File) error {
	if _, err := f.Write([]byte("typhon")); err != nil {
		return err
	}
	return f.Sync()
}
