package settings

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

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
)

type Settings struct {
	Theme                 string  `json:"theme"`
	Language              string  `json:"language"`
	UIScale               float64 `json:"uiScale"`
	DownloadsPath         string  `json:"downloadsPath"`
	GamesPath             string  `json:"gamesPath"`
	ScreenshotsPath       string  `json:"screenshotsPath"`
	LaunchOnStartup       bool    `json:"launchOnStartup"`
	MinimizeToTray        bool    `json:"minimizeToTray"`
	HardwareAcceleration  bool    `json:"hardwareAcceleration"`
	AnimationsEnabled     bool    `json:"animationsEnabled"`
	MaxActiveDownloads    int     `json:"maxActiveDownloads"`
	DownloadRateLimit     int64   `json:"downloadRateLimit"`
	UploadRateLimit       int64   `json:"uploadRateLimit"`
	SeedAfterDownload     bool    `json:"seedAfterDownload"`
	InstallCleanupPolicy  string  `json:"installCleanupPolicy"`
	AutoInstall           bool    `json:"autoInstall"`
	SourceRefreshInterval string  `json:"sourceRefreshInterval"`
	VerifyAfterInstall    bool    `json:"verifyAfterInstall"`
}

func Defaults() Settings {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	base := filepath.Join(home, "Typhon")
	return Settings{
		Theme:                 "dark",
		Language:              "ru",
		UIScale:               1,
		DownloadsPath:         filepath.Join(base, "Downloads"),
		GamesPath:             filepath.Join(base, "Games"),
		ScreenshotsPath:       filepath.Join(base, "Screenshots"),
		LaunchOnStartup:       false,
		MinimizeToTray:        true,
		HardwareAcceleration:  true,
		AnimationsEnabled:     true,
		MaxActiveDownloads:    2,
		DownloadRateLimit:     0,
		UploadRateLimit:       0,
		SeedAfterDownload:     false,
		InstallCleanupPolicy:  CleanupKeep,
		AutoInstall:           false,
		SourceRefreshInterval: RefreshSixHours,
		VerifyAfterInstall:    true,
	}
}

func sanitize(s Settings) Settings {
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
		s.InstallCleanupPolicy = CleanupKeep
	}
	switch s.SourceRefreshInterval {
	case RefreshManual, RefreshHourly, RefreshSixHours, RefreshHalfDay, RefreshDaily:
	default:
		s.SourceRefreshInterval = RefreshSixHours
	}
	return s
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

func NewService() *Service {
	dir, err := ConfigDir()
	if err != nil {
		slog.Error("resolve config dir", "error", err)
		return &Service{current: Defaults()}
	}
	return newServiceAt(filepath.Join(dir, "settings.json"))
}

func newServiceAt(path string) *Service {
	s := &Service{current: Defaults(), path: path}
	s.load()
	return s
}

func (s *Service) load() {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return
	}
	if err != nil {
		slog.Error("read settings", "path", s.path, "error", err)
		return
	}
	loaded := Defaults()
	if err := json.Unmarshal(data, &loaded); err != nil {
		slog.Error("parse settings", "path", s.path, "error", err)
		return
	}
	s.current = sanitize(loaded)
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

	next = sanitize(next)
	if s.path == "" {
		return next, nil, errors.New("settings path unavailable")
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		slog.Error("create config dir", "error", err)
		return next, nil, fmt.Errorf("create config dir: %w", err)
	}
	data, err := json.MarshalIndent(next, "", "  ")
	if err != nil {
		return next, nil, err
	}
	if err := os.WriteFile(s.path, data, 0o644); err != nil {
		slog.Error("write settings", "path", s.path, "error", err)
		return next, nil, fmt.Errorf("write settings: %w", err)
	}
	s.current = next

	subs := make([]func(Settings), 0, len(s.subs))
	for _, fn := range s.subs {
		subs = append(subs, fn)
	}
	return next, subs, nil
}
