package reviews

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

const requestContextTimeout = 30 * time.Second

const (
	minBodyRunes       = 20
	maxBodyRunes       = 5000
	minPlaytimeSeconds = int64(1800)
)

var (
	errUnknownGame = errors.New("unknown_game")
	errBadRequest  = errors.New("bad_request")

	errNotStarted = errors.New("reviews: service is not started")
)

var igdbIDPattern = regexp.MustCompile(`^[1-9][0-9]{0,19}$`)

var sortValues = map[string]struct{}{
	"helpful": {},
	"recent":  {},
}

var filterValues = map[string]struct{}{
	"all":      {},
	"positive": {},
	"negative": {},
}

var voteValues = map[string]struct{}{
	"helpful":   {},
	"unhelpful": {},
	"":          {},
}

var reportReasons = map[string]struct{}{
	"spam":      {},
	"offensive": {},
	"spoilers":  {},
	"off_topic": {},
	"other":     {},
}

type uiError struct {
	code string
	err  error
}

func (e *uiError) Error() string { return e.code }

func (e *uiError) Unwrap() error { return e.err }

func toUI(err error) error {
	var network *NetworkError
	var server *ServerError
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrUnauthorized):
		return &uiError{code: "unauthenticated", err: err}
	case errors.As(err, &network):
		return &uiError{code: "network_error", err: err}
	case errors.As(err, &server):
		return &uiError{code: "server_error", err: err}
	}
	return err
}

type Service struct {
	client        *client
	resolveIGDBID func(canonicalGameID string) string

	mu     sync.Mutex
	ctx    context.Context
	cancel context.CancelFunc
}

func NewService(baseURL string, token func() (string, error), resolveIGDBID func(canonicalGameID string) string) (*Service, error) {
	if resolveIGDBID == nil {
		return nil, errors.New("reviews: resolveIGDBID callback is nil")
	}
	cl, err := newClient(baseURL, token)
	if err != nil {
		return nil, err
	}
	return &Service{
		client:        cl,
		resolveIGDBID: resolveIGDBID,
	}, nil
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
	ctx, cancel := context.WithTimeout(base, requestContextTimeout)
	return ctx, cancel, nil
}

func (s *Service) Limits() Limits {
	return Limits{
		MinBodyRunes:       minBodyRunes,
		MaxBodyRunes:       maxBodyRunes,
		MinPlaytimeSeconds: minPlaytimeSeconds,
	}
}

func (s *Service) igdbID(canonicalGameID string) (string, error) {
	trimmed := strings.TrimSpace(canonicalGameID)
	if trimmed == "" {
		return "", errUnknownGame
	}
	igdbID := s.resolveIGDBID(trimmed)
	if !igdbIDPattern.MatchString(igdbID) {
		return "", errUnknownGame
	}
	return igdbID, nil
}

func (s *Service) List(canonicalGameID, sort, filter, cursor string) (Page, error) {
	igdbID, err := s.igdbID(canonicalGameID)
	if err != nil {
		return Page{}, err
	}
	if _, ok := sortValues[sort]; !ok {
		return Page{}, errBadRequest
	}
	if _, ok := filterValues[filter]; !ok {
		return Page{}, errBadRequest
	}

	ctx, cancel, err := s.requestContext()
	if err != nil {
		return Page{}, err
	}
	defer cancel()
	page, err := s.client.list(ctx, igdbID, sort, filter, strings.TrimSpace(cursor))
	return page, toUI(err)
}

func (s *Service) Mine(canonicalGameID string) (Mine, error) {
	igdbID, err := s.igdbID(canonicalGameID)
	if err != nil {
		return Mine{}, err
	}

	ctx, cancel, err := s.requestContext()
	if err != nil {
		return Mine{}, err
	}
	defer cancel()
	mine, err := s.client.mine(ctx, igdbID)
	return mine, toUI(err)
}

func (s *Service) Save(canonicalGameID string, recommended bool, body string) (Review, error) {
	igdbID, err := s.igdbID(canonicalGameID)
	if err != nil {
		return Review{}, err
	}

	ctx, cancel, err := s.requestContext()
	if err != nil {
		return Review{}, err
	}
	defer cancel()
	review, err := s.client.save(ctx, igdbID, recommended, body)
	return review, toUI(err)
}

func (s *Service) Delete(canonicalGameID string) error {
	igdbID, err := s.igdbID(canonicalGameID)
	if err != nil {
		return err
	}

	ctx, cancel, err := s.requestContext()
	if err != nil {
		return err
	}
	defer cancel()
	return toUI(s.client.delete(ctx, igdbID))
}

func (s *Service) Vote(reviewID int64, vote string) (VoteResult, error) {
	if reviewID <= 0 {
		return VoteResult{}, errBadRequest
	}
	if _, ok := voteValues[vote]; !ok {
		return VoteResult{}, errBadRequest
	}

	ctx, cancel, err := s.requestContext()
	if err != nil {
		return VoteResult{}, err
	}
	defer cancel()
	result, err := s.client.vote(ctx, reviewID, vote)
	return result, toUI(err)
}

func (s *Service) Report(reviewID int64, reason string) error {
	if reviewID <= 0 {
		return errBadRequest
	}
	if _, ok := reportReasons[reason]; !ok {
		return errBadRequest
	}

	ctx, cancel, err := s.requestContext()
	if err != nil {
		return err
	}
	defer cancel()
	return toUI(s.client.report(ctx, reviewID, reason))
}
