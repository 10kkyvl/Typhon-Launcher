package discord

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

type Presence struct {
	Details    string
	State      string
	StartedAt  time.Time
	LargeImage string
	LargeText  string
	SmallImage string
	SmallText  string
}

type Service struct {
	clientID string

	dial         func(context.Context) (io.ReadWriteCloser, error)
	reconnectMin time.Duration
	reconnectMax time.Duration

	mu       sync.Mutex
	enabled  bool
	presence *Presence
	cancel   context.CancelFunc

	wake chan struct{}
	wg   sync.WaitGroup
}

func NewService(clientID string) (*Service, error) {
	id := strings.TrimSpace(clientID)
	if id == "" {
		return nil, errors.New("не задан client id Discord")
	}
	if !isNumeric(id) {
		return nil, fmt.Errorf("client id Discord должен быть числом: %q", clientID)
	}
	return &Service{
		clientID:     id,
		dial:         platformDial,
		reconnectMin: time.Second,
		reconnectMax: 60 * time.Second,
		wake:         make(chan struct{}, 1),
	}, nil
}

func isNumeric(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func (s *Service) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	s.mu.Lock()
	if s.cancel != nil {
		s.mu.Unlock()
		return errors.New("discord сервис уже запущен")
	}
	runCtx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	s.mu.Unlock()

	s.wg.Add(1)
	go s.run(runCtx)
	return nil
}

func (s *Service) ServiceShutdown() error {
	s.mu.Lock()
	cancel := s.cancel
	s.cancel = nil
	s.mu.Unlock()
	if cancel == nil {
		return nil
	}
	cancel()
	s.wg.Wait()
	return nil
}

//wails:ignore
func (s *Service) SetEnabled(enabled bool) {
	s.mu.Lock()
	changed := s.enabled != enabled
	s.enabled = enabled
	s.mu.Unlock()
	if changed {
		s.notify()
	}
}

//wails:ignore
func (s *Service) Show(p Presence) {
	s.mu.Lock()
	s.presence = &p
	s.mu.Unlock()
	s.notify()
}

//wails:ignore
func (s *Service) Clear() {
	s.mu.Lock()
	s.presence = nil
	s.mu.Unlock()
	s.notify()
}

func (s *Service) notify() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func (s *Service) snapshot() *Presence {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.enabled {
		return nil
	}
	return s.presence
}

func (s *Service) run(ctx context.Context) {
	defer s.wg.Done()
	backoff := s.reconnectMin
	warned := false
	// wake может быть уже сигнализирован из Show/SetEnabled/Clear, вызванных до
	// ServiceStartup; run() читает актуальное состояние заново на каждом шаге,
	// так что такой сигнал ничего не несёт и должен быть погашен, иначе первая
	// же итерация serve() примет его за новое изменение и пошлёт лишний кадр.
	select {
	case <-s.wake:
	default:
	}
	for {
		if !s.isEnabled() {
			if !s.waitWake(ctx) {
				return
			}
			continue
		}
		conn, err := s.dial(ctx)
		if err == nil {
			warned = false
			backoff = s.reconnectMin
			err = s.serveWithWatchdog(ctx, conn)
		}
		if ctx.Err() != nil {
			return
		}
		if errors.Is(err, errDisabled) {
			warned = false
			backoff = s.reconnectMin
			continue
		}
		if err != nil && !warned {
			slog.Warn("discord недоступен", "error", err)
			warned = true
		}
		if !s.waitBackoff(ctx, &backoff) {
			return
		}
	}
}

func (s *Service) isEnabled() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.enabled
}

// waitWake ждёт сигнала на пробуждение (переключение тумблера, новый presence)
// без таймера бэкоффа — выключенный сервис не должен ходить к Discord вообще.
func (s *Service) waitWake(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return false
	case <-s.wake:
		return true
	}
}

func (s *Service) waitBackoff(ctx context.Context, cur *time.Duration) bool {
	if *cur <= 0 {
		*cur = s.reconnectMin
	}
	t := time.NewTimer(*cur)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-s.wake:
		return true
	case <-t.C:
	}
	next := *cur * 2
	if next > s.reconnectMax {
		next = s.reconnectMax
	}
	*cur = next
	return true
}

// serveWithWatchdog закрывает conn, если ctx отменяется, пока serve застряла
// в блокирующем чтении/записи именованного канала — у него нет дедлайнов,
// и Close() соединения снаружи остаётся единственным способом её разбудить.
func (s *Service) serveWithWatchdog(ctx context.Context, conn io.ReadWriteCloser) error {
	done := make(chan struct{})
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		select {
		case <-ctx.Done():
			closeConn(conn)
		case <-done:
		}
	}()
	err := s.serve(ctx, conn)
	close(done)
	return err
}

func (s *Service) serve(ctx context.Context, conn io.ReadWriteCloser) error {
	if err := handshake(conn, s.clientID); err != nil {
		closeConn(conn)
		return err
	}

	if err := exchange(conn, s.snapshot()); err != nil {
		closeConn(conn)
		return err
	}

	for {
		select {
		case <-ctx.Done():
			closeConn(conn)
			return ctx.Err()
		case <-s.wake:
			if !s.isEnabled() {
				err := exchange(conn, nil)
				closeConn(conn)
				if err != nil {
					return err
				}
				return errDisabled
			}
			if err := exchange(conn, s.snapshot()); err != nil {
				closeConn(conn)
				return err
			}
		}
	}
}

// exchange отправляет активность и сам дочитывает ответ на неё. Отдельной
// читающей горутины быть не должно: в Windows именованный канал открыт
// синхронным хэндлом, операции на нём выполняются по одной, и висящее чтение
// задерживает любую запись до своего завершения. Discord первым ничего не
// шлёт, поэтому такое чтение не заканчивается никогда, а вместе с ним
// навсегда застревает и отправка presence.
func exchange(conn io.ReadWriter, p *Presence) error {
	if err := sendActivity(conn, nextNonce(), p); err != nil {
		return err
	}
	for {
		f, err := readFrame(conn)
		if err != nil {
			return err
		}
		switch f.op {
		case opPing:
			if err := writeFrame(conn, opPong, f.payload); err != nil {
				return err
			}
		case opClose:
			return ErrConnectionClosed
		case opFrame:
			if err := commandError(f.payload); err != nil {
				slog.Warn("discord отклонил активность", "error", err)
			}
			return nil
		}
	}
}

func closeConn(c io.Closer) {
	if err := c.Close(); err != nil {
		slog.Debug("discord close ipc connection", "error", err)
	}
}

func nextNonce() string {
	return strconv.FormatInt(time.Now().UnixNano(), 10)
}
