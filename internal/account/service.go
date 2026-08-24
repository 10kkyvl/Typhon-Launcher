package account

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

const requestTimeout = 30 * time.Second

const (
	StatusAuthenticated   = "authenticated"
	StatusUnauthenticated = "unauthenticated"
	StatusUnavailable     = "unavailable"
	StatusGuest           = "guest"
)

var errNotStarted = errors.New("account service is not started")

type State struct {
	Status string      `json:"status"`
	User   CurrentUser `json:"user"`
	Reason string      `json:"reason"`
}

type Service struct {
	client    *Client
	store     CredentialStore
	statePath string

	mu     sync.Mutex
	guest  bool
	ctx    context.Context
	cancel context.CancelFunc
}

func NewService() (*Service, error) {
	store, err := NewCredentialStore()
	if err != nil {
		return nil, err
	}
	path, err := statePath()
	if err != nil {
		return nil, err
	}
	return newService(store, BaseURL(), path)
}

func newService(store CredentialStore, baseURL, path string) (*Service, error) {
	if path == "" {
		return nil, errors.New("account state path is empty")
	}
	loaded, err := loadState(path)
	if err != nil {
		return nil, err
	}

	s := &Service{store: store, statePath: path, guest: loaded.Guest}
	client, err := NewClient(baseURL, s.token)
	if err != nil {
		return nil, err
	}
	s.client = client
	return s, nil
}

func (s *Service) token() (string, error) {
	cred, err := s.store.Load()
	if err != nil {
		return "", err
	}
	return cred.Token, nil
}

//wails:ignore
func (s *Service) SessionToken() (string, error) {
	token, err := s.token()
	if errors.Is(err, ErrNoCredential) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return token, nil
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

func (s *Service) Bootstrap() (State, error) {
	cred, err := s.store.Load()
	if errors.Is(err, ErrNoCredential) {
		return s.signedOutState(), nil
	}
	if err != nil {
		return State{}, fmt.Errorf("load stored credential: %w", err)
	}
	if cred.Token == "" {
		return s.signedOutState(), nil
	}

	ctx, cancel, err := s.requestContext()
	if err != nil {
		return State{}, err
	}
	defer cancel()

	user, err := s.client.Me(ctx)
	if err == nil {
		return State{Status: StatusAuthenticated, User: user}, nil
	}

	var apiErr *Error
	if !errors.As(err, &apiErr) {
		return State{}, err
	}

	if apiErr.Code == CodeUnauthenticated {
		if delErr := s.store.Delete(); delErr != nil {
			return State{}, fmt.Errorf("discard rejected credential: %w", delErr)
		}
		return s.signedOutState(), nil
	}

	return State{Status: StatusUnavailable, Reason: apiErr.Code}, nil
}

func (s *Service) signedOutState() State {
	if s.isGuest() {
		return State{Status: StatusGuest}
	}
	return State{Status: StatusUnauthenticated}
}

func (s *Service) Register(input RegisterInput) (CurrentUser, error) {
	if err := s.setGuest(false); err != nil {
		return CurrentUser{}, err
	}

	ctx, cancel, err := s.requestContext()
	if err != nil {
		return CurrentUser{}, err
	}
	defer cancel()

	session, err := s.client.Register(ctx, input)
	if err != nil {
		return CurrentUser{}, err
	}
	return s.adopt(session)
}

func (s *Service) Login(input LoginInput) (CurrentUser, error) {
	if err := s.setGuest(false); err != nil {
		return CurrentUser{}, err
	}

	ctx, cancel, err := s.requestContext()
	if err != nil {
		return CurrentUser{}, err
	}
	defer cancel()

	session, err := s.client.Login(ctx, input)
	if err != nil {
		return CurrentUser{}, err
	}
	return s.adopt(session)
}

func (s *Service) adopt(session Session) (CurrentUser, error) {
	saveErr := s.store.Save(Credential{Token: session.Token, Username: session.User.Username})
	if saveErr == nil {
		return session.User, nil
	}

	slog.Error("store session credential", "error", saveErr)
	if err := s.revoke(session.Token); err != nil {
		slog.Error("revoke session after failed credential write", "error", err)
	}
	return CurrentUser{}, fmt.Errorf("store session credential: %w", saveErr)
}

func (s *Service) revoke(token string) error {
	ctx, cancel, err := s.requestContext()
	if err != nil {
		return err
	}
	defer cancel()
	return s.client.Logout(ctx, token)
}

func (s *Service) Logout() error {
	if err := s.setGuest(false); err != nil {
		return err
	}

	cred, err := s.store.Load()
	if err != nil && !errors.Is(err, ErrNoCredential) {
		return fmt.Errorf("load stored credential: %w", err)
	}

	var revokeErr error
	if cred.Token != "" {
		revokeErr = s.revoke(cred.Token)
	}

	if err := s.store.Delete(); err != nil {
		return fmt.Errorf("delete stored credential: %w", err)
	}

	if revokeErr != nil {
		var apiErr *Error
		if errors.As(revokeErr, &apiErr) && apiErr.Code == CodeUnauthenticated {
			return nil
		}
		return revokeErr
	}
	return nil
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
