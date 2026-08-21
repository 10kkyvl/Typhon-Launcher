package account

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

const requestTimeout = 30 * time.Second

type Service struct {
	client *Client
}

func NewService() *Service {
	return &Service{client: NewClient(BaseURL(), Token)}
}

func requestContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), requestTimeout)
}

func (s *Service) GetCurrentUser() (CurrentUser, error) {
	ctx, cancel := requestContext()
	defer cancel()
	return s.client.Me(ctx)
}

func (s *Service) UpdateProfile(patch Patch) (CurrentUser, error) {
	ctx, cancel := requestContext()
	defer cancel()
	return s.client.UpdateProfile(ctx, patch)
}

func (s *Service) SelectAvatarFile() (string, error) {
	dialog := application.Get().Dialog.OpenFile().
		SetTitle("Выберите аватар").
		CanChooseFiles(true).
		AddFilter("Изображения (*.png, *.jpg, *.jpeg, *.webp)", "*.png;*.jpg;*.jpeg;*.webp").
		AddFilter("Все файлы", "*.*")
	path, err := dialog.PromptForSingleSelection()
	if err != nil {
		slog.Warn("select avatar file", "error", err)
		return "", err
	}
	return path, nil
}

func (s *Service) UploadAvatar(path string) (CurrentUser, error) {
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return CurrentUser{}, &Error{Code: CodeInvalidAvatar}
		}
		slog.Error("stat avatar file", "error", err)
		return CurrentUser{}, &Error{Code: CodeInvalidAvatar, cause: err}
	}
	if info.Size() > maxAvatarSize {
		return CurrentUser{}, &Error{Code: CodeAvatarTooLarge}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		slog.Error("read avatar file", "error", err)
		return CurrentUser{}, &Error{Code: CodeInvalidAvatar, cause: err}
	}

	ctx, cancel := requestContext()
	defer cancel()
	return s.client.UploadAvatar(ctx, data)
}

func (s *Service) RemoveAvatar() (CurrentUser, error) {
	ctx, cancel := requestContext()
	defer cancel()
	return s.client.RemoveAvatar(ctx)
}

func (s *Service) IsSignedIn() bool {
	_, err := Token()
	return err == nil
}
