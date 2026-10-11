package support

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"typhon/internal/account"
	"typhon/internal/app"
	"typhon/internal/diagnostics"
	"typhon/internal/uierr"

	"github.com/wailsapp/wails/v3/pkg/application"
)

const requestTimeout = 30 * time.Second

var (
	errNotStarted      = errors.New("support service is not started")
	errNoLanguage      = errors.New("support: language source is nil")
	errLogsUnavailable = uierr.New("support.logs_unavailable", "log upload is not available in this session")
)

const bundleReuseWindow = 30 * time.Minute

type Deps struct {
	Language   func() string
	UploadLogs func(ctx context.Context) (string, error)
}

type keptBundle struct {
	target string
	token  string
	id     string
	at     time.Time
}

type Service struct {
	client     *Client
	token      func() (string, error)
	language   func() string
	uploadLogs func(ctx context.Context) (string, error)
	info       func() ClientInfo
	now        func() time.Time

	mu     sync.Mutex
	ctx    context.Context
	cancel context.CancelFunc
	kept   *keptBundle
}

func NewService(baseURL string, token func() (string, error), deps Deps) (*Service, error) {
	if deps.Language == nil {
		return nil, errNoLanguage
	}
	client, err := NewClient(baseURL, token)
	if err != nil {
		return nil, err
	}
	return &Service{
		client:     client,
		token:      token,
		language:   deps.Language,
		uploadLogs: deps.UploadLogs,
		info:       appClientInfo,
		now:        time.Now,
	}, nil
}

func appClientInfo() ClientInfo {
	info := app.CurrentInfo()
	return ClientInfo{AppVersion: info.Version, OS: info.Platform, Arch: info.Arch}
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

func (s *Service) base() (context.Context, error) {
	s.mu.Lock()
	base := s.ctx
	s.mu.Unlock()
	if base == nil {
		return nil, errNotStarted
	}
	if err := base.Err(); err != nil {
		return nil, fmt.Errorf("support service is stopped: %w", err)
	}
	return base, nil
}

func (s *Service) requestContext() (context.Context, context.CancelFunc, error) {
	base, err := s.base()
	if err != nil {
		return nil, nil, err
	}
	ctx, cancel := context.WithTimeout(base, requestTimeout)
	return ctx, cancel, nil
}

func (s *Service) Limits() Limits {
	return Limits{MaxSubjectRunes: MaxSubjectRunes, MaxBodyRunes: MaxBodyRunes}
}

func (s *Service) List(cursor string) (Page, error) {
	ctx, cancel, err := s.requestContext()
	if err != nil {
		return Page{}, err
	}
	defer cancel()
	return s.client.List(ctx, cursor)
}

func (s *Service) Get(number int64) (Detail, error) {
	ctx, cancel, err := s.requestContext()
	if err != nil {
		return Detail{}, err
	}
	defer cancel()
	return s.client.Get(ctx, number)
}

func (s *Service) Resolve(number int64) (Ticket, error) {
	ctx, cancel, err := s.requestContext()
	if err != nil {
		return Ticket{}, err
	}
	defer cancel()
	return s.client.Resolve(ctx, number)
}

func (s *Service) Unread() (int, error) {
	ctx, cancel, err := s.requestContext()
	if err != nil {
		return 0, err
	}
	defer cancel()
	return s.client.Unread(ctx)
}

func (s *Service) Create(input CreateInput) (Ticket, error) {
	req, err := s.newCreateRequest(input)
	if err != nil {
		return Ticket{}, err
	}
	base, err := s.base()
	if err != nil {
		return Ticket{}, err
	}
	if input.AttachLogs {
		bundleID, err := s.uploadBundle(base, createTarget)
		if err != nil {
			return Ticket{}, err
		}
		req.LogBundleID = bundleID
	}

	ctx, cancel, err := s.requestContext()
	if err != nil {
		return Ticket{}, err
	}
	defer cancel()
	ticket, err := s.client.Create(ctx, req)
	if input.AttachLogs {
		s.settleBundle(createTarget, err)
	}
	return ticket, err
}

func (s *Service) Reply(number int64, body string, attachLogs bool) (Message, error) {
	body, err := normalizeBody(body)
	if err != nil {
		return Message{}, err
	}
	if number <= 0 {
		return Message{}, &account.Error{Code: CodeTicketNotFound, Status: http.StatusNotFound}
	}
	base, err := s.base()
	if err != nil {
		return Message{}, err
	}
	target := replyTarget(number)
	bundleID := ""
	if attachLogs {
		bundleID, err = s.uploadBundle(base, target)
		if err != nil {
			return Message{}, err
		}
	}

	ctx, cancel, err := s.requestContext()
	if err != nil {
		return Message{}, err
	}
	defer cancel()
	msg, err := s.client.Reply(ctx, number, body, bundleID)
	if attachLogs {
		s.settleBundle(target, err)
	}
	return msg, err
}

const createTarget = "create"

func replyTarget(number int64) string {
	return "reply:" + strconv.FormatInt(number, 10)
}

func (s *Service) uploadBundle(ctx context.Context, target string) (string, error) {
	if s.uploadLogs == nil {
		return "", errLogsUnavailable
	}
	tok, err := s.token()
	if err != nil && !errors.Is(err, account.ErrNoCredential) {
		return "", fmt.Errorf("resolve session token: %w", err)
	}
	if err != nil || tok == "" {
		return "", &account.Error{Code: account.CodeUnauthenticated, Status: http.StatusUnauthorized}
	}
	if id := s.keptFor(target, tok); id != "" {
		return id, nil
	}
	id, err := s.uploadLogs(ctx)
	if err != nil {
		if uierr.Code(err) == "" {
			return "", uierr.Wrap(diagnostics.ErrCodeLogUploadFailed, err)
		}
		return "", err
	}
	if strings.TrimSpace(id) == "" {
		return "", uierr.New(diagnostics.ErrCodeLogUploadFailed, "log upload returned an empty bundle id")
	}
	s.mu.Lock()
	s.kept = &keptBundle{target: target, token: tok, id: id, at: s.now()}
	s.mu.Unlock()
	return id, nil
}

func (s *Service) keptFor(target, token string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := s.kept
	if k == nil || k.target != target || k.token != token || s.now().Sub(k.at) >= bundleReuseWindow {
		return ""
	}
	return k.id
}

func (s *Service) settleBundle(target string, err error) {
	if err != nil && !bundleSpent(err) {
		return
	}
	s.mu.Lock()
	if s.kept != nil && s.kept.target == target {
		s.kept = nil
	}
	s.mu.Unlock()
}

func bundleSpent(err error) bool {
	var apiErr *account.Error
	if !errors.As(err, &apiErr) {
		return false
	}
	switch apiErr.Code {
	case CodeTicketBundleUnavailable, CodeTicketBundleLimit, CodeTicketNotFound:
		return true
	}
	return false
}

func (s *Service) newCreateRequest(input CreateInput) (createRequest, error) {
	if !slices.Contains(topics, input.Topic) {
		return createRequest{}, &account.Error{Code: CodeTicketInvalidTopic, Field: "topic", Status: http.StatusUnprocessableEntity}
	}
	subject, err := normalizeSubject(input.Subject)
	if err != nil {
		return createRequest{}, err
	}
	body, err := normalizeBody(input.Body)
	if err != nil {
		return createRequest{}, err
	}
	if input.GameIgdbID != nil && *input.GameIgdbID <= 0 {
		return createRequest{}, &account.Error{Code: CodeTicketInvalidGame, Field: "gameIgdbId", Status: http.StatusUnprocessableEntity}
	}
	lang := s.language()
	info := s.info()
	return createRequest{
		Topic:      input.Topic,
		Subject:    subject,
		Body:       body,
		GameIgdbID: input.GameIgdbID,
		Client:     &info,
		Lang:       lang,
	}, nil
}
