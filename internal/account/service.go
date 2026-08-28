package account

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
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
	StatusOffline         = "offline"
)

var errNotStarted = errors.New("account service is not started")

type State struct {
	Status string      `json:"status"`
	User   CurrentUser `json:"user"`
	Reason string      `json:"reason"`
}

type Service struct {
	client      *Client
	store       CredentialStore
	statePath   string
	profilePath string

	mu           sync.Mutex
	guest        bool
	profile      cachedProfile
	profileEpoch uint64
	ctx          context.Context
	cancel       context.CancelFunc

	profileMu sync.Mutex
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

	profPath := profilePathFrom(path)
	if profPath == "" {
		return nil, errors.New("account profile cache path is empty")
	}
	profile, err := loadProfile(profPath)
	if err != nil {
		return nil, fmt.Errorf("load cached profile: %w", err)
	}

	s := &Service{store: store, statePath: path, profilePath: profPath, guest: loaded.Guest, profile: profile}
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
		if rememberErr := s.rememberProfile(ctx, user); rememberErr != nil {
			return State{}, rememberErr
		}
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
		if delErr := s.forgetProfile(); delErr != nil {
			return State{}, fmt.Errorf("discard cached profile: %w", delErr)
		}
		return s.signedOutState(), nil
	}

	if isOfflineError(apiErr) {
		if state, ok := s.offlineState(apiErr.Code); ok {
			return state, nil
		}
	}

	return State{Status: StatusUnavailable, Reason: apiErr.Code}, nil
}

func isOfflineError(apiErr *Error) bool {
	if apiErr.Code == CodeNetwork {
		return true
	}
	return apiErr.Code == CodeServer && (apiErr.Status == 0 || apiErr.Status >= 500)
}

func (s *Service) offlineState(reason string) (State, bool) {
	profile := s.currentProfile()
	if profile.User.ID == "" {
		return State{}, false
	}
	user := profile.User
	if profile.Avatar.Data != "" {
		user.AvatarURL = fmt.Sprintf("data:%s;base64,%s", profile.Avatar.MIME, profile.Avatar.Data)
	} else {
		user.AvatarURL = ""
	}
	return State{Status: StatusOffline, User: user, Reason: reason}, true
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
	return s.adopt(ctx, session)
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
	return s.adopt(ctx, session)
}

func (s *Service) adopt(ctx context.Context, session Session) (CurrentUser, error) {
	saveErr := s.store.Save(Credential{Token: session.Token, Username: session.User.Username})
	if saveErr == nil {
		if err := s.rememberProfile(ctx, session.User); err != nil {
			// The credential is already stored, so the session is real: a cache failure only
			// costs offline mode on the next launch and must not read as a failed sign-in.
			slog.Warn("cache profile after sign-in", "error", err)
		}
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
		var apiErr *Error
		if errors.As(revokeErr, &apiErr) && apiErr.Code == CodeUnauthenticated {
			revokeErr = nil
		}
	}

	deleteErr := s.store.Delete()
	if deleteErr != nil {
		deleteErr = fmt.Errorf("delete stored credential: %w", deleteErr)
	}

	forgetErr := s.forgetProfile()
	if forgetErr != nil {
		forgetErr = fmt.Errorf("delete cached profile: %w", forgetErr)
	}

	return errors.Join(revokeErr, deleteErr, forgetErr)
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
	user, err := s.client.UpdateProfile(ctx, patch)
	if err != nil {
		return CurrentUser{}, err
	}
	if err := s.rememberProfile(ctx, user); err != nil {
		return CurrentUser{}, err
	}
	return user, nil
}

func (s *Service) PickAvatar() (AvatarImage, error) {
	dialog := application.Get().Dialog.OpenFile().
		SetTitle("Выберите аватар").
		CanChooseFiles(true).
		AddFilter("Изображения (*.png, *.jpg, *.jpeg, *.webp)", "*.png;*.jpg;*.jpeg;*.webp").
		AddFilter("Все файлы", "*.*")
	path, err := dialog.PromptForSingleSelection()
	if err != nil {
		slog.Warn("select avatar file", "error", err)
		return AvatarImage{}, err
	}
	if path == "" {
		return AvatarImage{}, nil
	}
	return readAvatarImage(path)
}

func (s *Service) UploadAvatar(encoded string) (CurrentUser, error) {
	data, err := decodeAvatar(encoded)
	if err != nil {
		return CurrentUser{}, err
	}

	ctx, cancel, err := s.requestContext()
	if err != nil {
		return CurrentUser{}, err
	}
	defer cancel()
	user, err := s.client.UploadAvatar(ctx, data)
	if err != nil {
		return CurrentUser{}, err
	}
	if err := s.rememberProfile(ctx, user); err != nil {
		return CurrentUser{}, err
	}
	return user, nil
}

func (s *Service) RemoveAvatar() (CurrentUser, error) {
	ctx, cancel, err := s.requestContext()
	if err != nil {
		return CurrentUser{}, err
	}
	defer cancel()
	user, err := s.client.RemoveAvatar(ctx)
	if err != nil {
		return CurrentUser{}, err
	}
	if err := s.rememberProfile(ctx, user); err != nil {
		return CurrentUser{}, err
	}
	return user, nil
}
