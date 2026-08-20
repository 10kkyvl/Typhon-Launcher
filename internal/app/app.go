package app

import (
	"log/slog"
	"runtime"

	"aurora/internal/platform"
	"aurora/internal/settings"

	"github.com/wailsapp/wails/v3/pkg/application"
)

const Version = "0.1.0"

type AppInfo struct {
	Version  string `json:"version"`
	Platform string `json:"platform"`
	Arch     string `json:"arch"`
}

type Service struct {
	settings *settings.Service
}

func NewService(settingsService *settings.Service) *Service {
	return &Service{settings: settingsService}
}

func (s *Service) GetAppInfo() AppInfo {
	return AppInfo{
		Version:  Version,
		Platform: runtime.GOOS,
		Arch:     runtime.GOARCH,
	}
}

func (s *Service) GetSystemInfo() (platform.SystemInfo, error) {
	info, err := platform.GetSystemInfo()
	if err != nil {
		slog.Warn("system info", "error", err)
	}
	return info, nil
}

func (s *Service) GetStorageInfo() (platform.StorageInfo, error) {
	info, err := platform.GetStorageInfo(s.settings.GetSettings().GamesPath)
	if err != nil {
		slog.Error("storage info", "error", err)
	}
	return info, err
}

func (s *Service) SelectExecutable(title string) (string, error) {
	dialog := application.Get().Dialog.OpenFile().
		SetTitle(title).
		CanChooseFiles(true).
		AddFilter("Исполняемые файлы (*.exe)", "*.exe").
		AddFilter("Все файлы", "*.*")
	path, err := dialog.PromptForSingleSelection()
	if err != nil {
		slog.Warn("select executable", "error", err)
		return "", err
	}
	return path, nil
}

func (s *Service) SelectFolder(title string) (string, error) {
	dialog := application.Get().Dialog.OpenFile().
		SetTitle(title).
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
