package app

import (
	"log/slog"
	"runtime"
	"sync/atomic"

	"typhon/internal/devmock"
	"typhon/internal/dialogtext"
	"typhon/internal/platform"
	"typhon/internal/settings"

	"github.com/wailsapp/wails/v3/pkg/application"
)

var Version = "0.7.1"

type AppInfo struct {
	Version  string `json:"version"`
	Platform string `json:"platform"`
	Arch     string `json:"arch"`
	DevMock  bool   `json:"devMock"`
}

type Service struct {
	settings *settings.Service
	russian  atomic.Bool
}

func NewService(settingsService *settings.Service) *Service {
	s := &Service{settings: settingsService}
	s.SetUILanguage(settingsService.GetSettings().Language)
	return s
}

// SetUILanguage receives the resolved webview locale; "system" is resolved there.
func (s *Service) SetUILanguage(language string) {
	s.russian.Store(language == "ru")
}

//wails:ignore
func (s *Service) UILanguage() string {
	if s.russian.Load() {
		return "ru"
	}
	return "en"
}

func (s *Service) GetAppInfo() AppInfo {
	return AppInfo{
		Version:  Version,
		Platform: runtime.GOOS,
		Arch:     runtime.GOARCH,
		DevMock:  devmock.Enabled,
	}
}

func (s *Service) GetSystemInfo() (platform.SystemInfo, error) {
	info, err := platform.GetSystemInfo()
	if err != nil {
		slog.Warn("system info", "error", err)
	}
	return info, nil
}

// GetWineStatus говорит интерфейсу, нужен ли на этой платформе CrossOver и
// установлен ли он: на macOS игры ставятся и запускаются только через него.
func (s *Service) GetWineStatus() platform.WineStatus {
	return platform.Wine()
}

func (s *Service) GetStorageInfo() (platform.StorageInfo, error) {
	library := s.settings.GetSettings().LibraryPath
	if library == "" {
		return platform.StorageInfo{}, settings.ErrLibraryNotConfigured
	}
	return s.GetStorageInfoFor(library)
}

func (s *Service) GetStorageInfoFor(path string) (platform.StorageInfo, error) {
	info, err := platform.GetStorageInfo(path)
	if err != nil {
		slog.Error("storage info", "path", path, "error", err)
		return platform.StorageInfo{}, err
	}
	return info, nil
}

func (s *Service) SelectExecutable(title, language string) (string, error) {
	labels := dialogtext.For(language)
	dialog := application.Get().Dialog.OpenFile().
		SetTitle(title).
		SetMessage(title).
		CanChooseFiles(true).
		AddFilter(labels.Executables, "*.exe").
		AddFilter(labels.AllFiles, "*.*")
	path, err := dialog.PromptForSingleSelection()
	if err != nil {
		slog.Warn("select executable", "error", err)
		return "", err
	}
	return path, nil
}

// SelectGameExecutable opens the picker in the game's CrossOver bottle on
// macOS. Other platforms keep using their native dialog.
func (s *Service) SelectGameExecutable(title, installDir, current, language string) (string, error) {
	labels := dialogtext.For(language)
	if runtime.GOOS == "darwin" {
		return platform.SelectGameExecutable(title, installDir, current, language)
	}
	dialog := application.Get().Dialog.OpenFile().
		SetTitle(title).
		SetMessage(title).
		SetDirectory(installDir).
		CanChooseFiles(true).
		AddFilter(labels.Executables, "*.exe").
		AddFilter(labels.AllFiles, "*.*")
	return dialog.PromptForSingleSelection()
}

func (s *Service) SelectFolder(title string) (string, error) {
	dialog := application.Get().Dialog.OpenFile().
		SetTitle(title).
		SetMessage(title).
		CanChooseDirectories(true).
		CanChooseFiles(false)
	path, err := dialog.PromptForSingleSelection()
	if err != nil {
		slog.Warn("select folder", "error", err)
		return "", err
	}
	return path, nil
}

func (s *Service) OpenFolder(path string) error {
	if err := platform.OpenFolder(path); err != nil {
		slog.Error("open folder", "path", path, "error", err)
		return err
	}
	return nil
}

func (s *Service) OpenGameFolder(path, executable string) error {
	return platform.OpenGameFolder(path, executable)
}
