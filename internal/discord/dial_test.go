package discord

import (
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"testing"
	"time"
)

// TestDialCancelableStopsOnCtxCancel ловит регрессию найденную ревью: если
// dial() полагается на блокирующий системный вызов без собственной поддержки
// ctx (os.OpenFile на именованном канале, когда антивирус держит хэндл),
// ServiceShutdown обязан вернуться сразу по отмене ctx, а не ждать, пока
// системный вызов рано или поздно разблокируется сам.
func TestDialCancelableStopsOnCtxCancel(t *testing.T) {
	release := make(chan struct{})
	started := make(chan struct{}, 1)
	attemptDone := make(chan struct{})

	attempt := func() (io.ReadWriteCloser, error) {
		started <- struct{}{}
		<-release
		close(attemptDone)
		return nil, errors.New("attempt finished after release")
	}

	ctx, cancel := context.WithCancel(context.Background())
	resultCh := make(chan error, 1)
	go func() {
		_, err := dialCancelable(ctx, attempt)
		resultCh <- err
	}()

	select {
	case <-started:
	case <-time.After(testTimeout):
		t.Fatal("attempt was never started")
	}

	cancel()

	select {
	case err := <-resultCh:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("dialCancelable error = %v, want context.Canceled", err)
		}
	case <-time.After(testTimeout):
		t.Fatal("dialCancelable did not return promptly after ctx cancel — still blocked on attempt")
	}

	close(release)
	select {
	case <-attemptDone:
	case <-time.After(testTimeout):
		t.Fatal("background attempt goroutine leaked, never observed release")
	}
}

func TestDialCancelableReturnsAttemptResult(t *testing.T) {
	client, _ := net.Pipe()
	attempt := func() (io.ReadWriteCloser, error) { return client, nil }

	conn, err := dialCancelable(context.Background(), attempt)
	if err != nil {
		t.Fatalf("dialCancelable: %v", err)
	}
	if conn != io.ReadWriteCloser(client) {
		t.Fatal("dialCancelable returned different conn than attempt produced")
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestDialCancelablePropagatesAttemptError(t *testing.T) {
	wantErr := errors.New("no discord pipe")
	attempt := func() (io.ReadWriteCloser, error) { return nil, wantErr }

	_, err := dialCancelable(context.Background(), attempt)
	if !errors.Is(err, wantErr) {
		t.Fatalf("dialCancelable error = %v, want %v", err, wantErr)
	}
}

type closeSpy struct {
	closed chan struct{}
	once   sync.Once
}

func (c *closeSpy) Read([]byte) (int, error)    { return 0, io.EOF }
func (c *closeSpy) Write(p []byte) (int, error) { return len(p), nil }

func (c *closeSpy) Close() error {
	c.once.Do(func() { close(c.closed) })
	return nil
}

// TestDialCancelableClosesAbandonedConn ловит регрессию, найденную ревью:
// отмена ctx возвращает управление сразу, но attempt может успеть открыть
// канал уже после этого — брошенный хэндл обязан быть закрыт, иначе он висит
// до конца процесса.
func TestDialCancelableClosesAbandonedConn(t *testing.T) {
	release := make(chan struct{})
	started := make(chan struct{}, 1)
	conn := &closeSpy{closed: make(chan struct{})}

	attempt := func() (io.ReadWriteCloser, error) {
		started <- struct{}{}
		<-release
		return conn, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	resultCh := make(chan error, 1)
	go func() {
		got, err := dialCancelable(ctx, attempt)
		if got != nil {
			t.Errorf("dialCancelable вернул соединение после отмены: %v", got)
		}
		resultCh <- err
	}()

	select {
	case <-started:
	case <-time.After(testTimeout):
		t.Fatal("attempt не стартовал")
	}

	cancel()
	select {
	case err := <-resultCh:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("dialCancelable error = %v, want context.Canceled", err)
		}
	case <-time.After(testTimeout):
		t.Fatal("dialCancelable не вернулся после отмены ctx")
	}

	close(release)
	select {
	case <-conn.closed:
	case <-time.After(testTimeout):
		t.Fatal("хэндл канала, открытый после отмены, остался незакрытым")
	}
}
