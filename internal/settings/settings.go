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

type Settings struct {
	Theme                string  `json:"theme"`
	Language             string  `json:"language"`
	UIScale              float64 `json:"uiScale"`
	DownloadsPath        string  `json:"downloadsPath"`
	GamesPath            string  `json:"gamesPath"`
	ScreenshotsPath      string  `json:"screenshotsPath"`
	LaunchOnStartup      bool    `json:"launchOnStartup"`
	MinimizeToTray       bool    `json:"minimizeToTray"`
	HardwareAcceleration bool    `json:"hardwareAcceleration"`
	AnimationsEnabled    bool    `json:"animationsEnabled"`
}

func Defaults() Settings {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	base := filepath.Join(home, "Aurora")
	return Settings{
		Theme:                "dark",
		Language:             "ru",
		UIScale:              1,
		DownloadsPath:        filepath.Join(base, "Downloads"),
		GamesPath:            filepath.Join(base, "Games"),
		ScreenshotsPath:      filepath.Join(base, "Screenshots"),
		LaunchOnStartup:      false,
		MinimizeToTray:       true,
		HardwareAcceleration: true,
		AnimationsEnabled:    true,
	}
}

func ConfigDir() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "Aurora"), nil
}

type Service struct {
	mu      sync.Mutex
	path    string
	current Settings
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
	if loaded.UIScale < 0.9 || loaded.UIScale > 1.25 {
		loaded.UIScale = 1
	}
	s.current = loaded
}

func (s *Service) GetSettings() Settings {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.current
}

func (s *Service) SaveSettings(next Settings) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if next.UIScale < 0.9 || next.UIScale > 1.25 {
		next.UIScale = 1
	}
	if s.path == "" {
		return errors.New("settings path unavailable")
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		slog.Error("create config dir", "error", err)
		return fmt.Errorf("create config dir: %w", err)
	}
	data, err := json.MarshalIndent(next, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(s.path, data, 0o644); err != nil {
		slog.Error("write settings", "path", s.path, "error", err)
		return fmt.Errorf("write settings: %w", err)
	}
	s.current = next
	if app := application.Get(); app != nil {
		app.Event.Emit("settings:updated", next)
	}
	return nil
}
