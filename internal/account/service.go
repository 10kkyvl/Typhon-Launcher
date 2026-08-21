package account

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

const requestTimeout = 30 * time.Second

var errNotStarted = errors.New("account service is not started")

type Service struct {
	client *Client

	mu     sync.Mutex
	ctx    context.Context
	cancel context.CancelFunc
}

func NewService() (*Service, error) {
	client, err := NewClient(BaseURL(), Token)
	if err != nil {
		return nil, err
	}
	return &Service{client: client}, nil
}

func (s *Service) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	s.mu.Lock()
	s.ctx, s.cancel = context.WithCancel(ctx)
	s.mu.Unlock()
	return nil
}

func (s *Service) ServiceShutdown() error {
	s.mu.Lock()
	cancel := s.cancel
	s.cancel = nil
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	return nil
}

func (s *Service) requestContext() (context.Context, context.CancelFunc, error) {
	s.mu.Lock()
	base := s.ctx
	s.mu.Unlock()
	if base == nil {
		return nil, nil, errNotStarted
	}
	ctx, cancel := context.WithTimeout(base, requestTimeout)
	return ctx, cancel, nil
}

func (s *Service) GetCurrentUser() (CurrentUser, error) {
	ctx, cancel, err := s.requestContext()
	if err != nil {
		return CurrentUser{}, err
	}
	defer cancel()
	return s.client.Me(ctx)
}

func (s *Service) UpdateProfile(patch Patch) (CurrentUser, error) {
	ctx, cancel, err := s.requestContext()
	if err != nil {
		return CurrentUser{}, err
	}
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

	ctx, cancel, err := s.requestContext()
	if err != nil {
		return CurrentUser{}, err
	}
	defer cancel()
	return s.client.UploadAvatar(ctx, data)
}

func (s *Service) RemoveAvatar() (CurrentUser, error) {
	ctx, cancel, err := s.requestContext()
	if err != nil {
		return CurrentUser{}, err
	}
	defer cancel()
	return s.client.RemoveAvatar(ctx)
}

func (s *Service) IsSignedIn() bool {
	_, err := Token()
	return err == nil
}
