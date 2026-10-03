package media

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

const opTimeout = 5 * time.Second

var (
	ErrUnsupported = errors.New("media controls are not available on this platform")
	ErrNoSession   = errors.New("no media session is active")
	ErrRefused     = errors.New("the player refused the command")
	ErrNotStarted  = errors.New("media service is not running")
)

type Track struct {
	App          string `json:"app"`
	Title        string `json:"title"`
	Artist       string `json:"artist"`
	Album        string `json:"album"`
	Playing      bool   `json:"playing"`
	CanPlayPause bool   `json:"canPlayPause"`
	CanNext      bool   `json:"canNext"`
	CanPrev      bool   `json:"canPrev"`
}

type State struct {
	Supported bool  `json:"supported"`
	Active    bool  `json:"active"`
	Track     Track `json:"track"`
}

type command int

const (
	cmdTogglePlayPause command = iota
	cmdNext
	cmdPrevious
)

type platform interface {
	supported() bool
	start(ctx context.Context) error
	stop() error
	current(ctx context.Context) (track Track, active bool, err error)
	control(ctx context.Context, cmd command) error
}

type Service struct {
	p       platform
	timeout time.Duration
}

func NewService() (*Service, error) {
	p, err := newPlatform()
	if err != nil {
		return nil, fmt.Errorf("media: %w", err)
	}
	return &Service{p: p, timeout: opTimeout}, nil
}

func (s *Service) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	if err := s.p.start(ctx); err != nil {
		return fmt.Errorf("start media service: %w", err)
	}
	return nil
}

func (s *Service) ServiceShutdown() error {
	if err := s.p.stop(); err != nil {
		return fmt.Errorf("stop media service: %w", err)
	}
	return nil
}

func (s *Service) Current(ctx context.Context) (State, error) {
	if !s.p.supported() {
		return State{}, nil
	}
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	track, active, err := s.p.current(ctx)
	if err != nil {
		return State{}, fmt.Errorf("read media session: %w", err)
	}
	if !active {
		return State{Supported: true}, nil
	}
	track.App = shortAppName(track.App)
	track.Title = clean(track.Title, maxText)
	track.Artist = clean(track.Artist, maxText)
	track.Album = clean(track.Album, maxText)
	return State{Supported: true, Active: true, Track: track}, nil
}

func (s *Service) TogglePlayPause(ctx context.Context) error {
	return s.send(ctx, cmdTogglePlayPause, "toggle play/pause")
}

func (s *Service) Next(ctx context.Context) error {
	return s.send(ctx, cmdNext, "skip to next track")
}

func (s *Service) Previous(ctx context.Context) error {
	return s.send(ctx, cmdPrevious, "skip to previous track")
}

func (s *Service) send(ctx context.Context, cmd command, what string) error {
	if !s.p.supported() {
		return ErrUnsupported
	}
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	if err := s.p.control(ctx, cmd); err != nil {
		return fmt.Errorf("%s: %w", what, err)
	}
	return nil
}
