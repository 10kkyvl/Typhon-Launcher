package discord

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

const testTimeout = 5 * time.Second

func newTestService(t *testing.T, dial func(context.Context) (io.ReadWriteCloser, error)) *Service {
	t.Helper()
	svc, err := NewService("123456789012345678")
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	svc.dial = dial
	svc.reconnectMin = time.Microsecond
	svc.reconnectMax = time.Millisecond
	return svc
}

func startService(t *testing.T, svc *Service) {
	t.Helper()
	if err := svc.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatalf("ServiceStartup: %v", err)
	}
	t.Cleanup(func() {
		if err := svc.ServiceShutdown(); err != nil {
			t.Fatalf("ServiceShutdown: %v", err)
		}
	})
}

// TestHandshakeAndInitialActivity ловит регрессию в кадрировании: неверные
// опкод/длина/JSON хендшейка или SET_ACTIVITY (pid, details, state,
// timestamps.start в миллисекундах, assets) сломали бы presence молча.
func TestHandshakeAndInitialActivity(t *testing.T) {
	client, server := net.Pipe()
	dialed := make(chan struct{}, 1)
	svc := newTestService(t, func(context.Context) (io.ReadWriteCloser, error) {
		select {
		case dialed <- struct{}{}:
		default:
		}
		return client, nil
	})

	started := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	svc.SetEnabled(true)
	svc.Show(Presence{
		Details:    "В меню",
		State:      "Одиночная игра",
		StartedAt:  started,
		LargeImage: "cover",
		LargeText:  "Typhon",
		SmallImage: "icon",
		SmallText:  "запущено",
	})

	done := make(chan setActivityCommand, 1)
	errCh := make(chan error, 1)
	go func() {
		if _, err := writeReadyHandshakeSafe(server); err != nil {
			errCh <- err
			return
		}
		cmd, err := readSetActivitySafe(server)
		if err != nil {
			errCh <- err
			return
		}
		done <- cmd
	}()

	startService(t, svc)

	select {
	case err := <-errCh:
		t.Fatalf("fake server: %v", err)
	case cmd := <-done:
		if cmd.Args.PID <= 0 {
			t.Fatalf("pid = %d, want > 0", cmd.Args.PID)
		}
		if cmd.Args.Activity == nil {
			t.Fatal("activity is nil, want set")
		}
		if cmd.Args.Activity.Details != "В меню" {
			t.Fatalf("details = %q", cmd.Args.Activity.Details)
		}
		if cmd.Args.Activity.State != "Одиночная игра" {
			t.Fatalf("state = %q", cmd.Args.Activity.State)
		}
		if cmd.Args.Activity.Timestamps == nil || cmd.Args.Activity.Timestamps.Start != started.UnixMilli() {
			t.Fatalf("timestamps = %+v, want start=%d", cmd.Args.Activity.Timestamps, started.UnixMilli())
		}
		if cmd.Args.Activity.Assets == nil || cmd.Args.Activity.Assets.LargeImage != "cover" || cmd.Args.Activity.Assets.SmallImage != "icon" {
			t.Fatalf("assets = %+v", cmd.Args.Activity.Assets)
		}
	case <-time.After(testTimeout):
		t.Fatal("timed out waiting for SET_ACTIVITY")
	}
}

func writeReadyHandshakeSafe(conn io.ReadWriter) (handshakePayload, error) {
	f, err := readFrame(conn)
	if err != nil {
		return handshakePayload{}, err
	}
	var hp handshakePayload
	if err := json.Unmarshal(f.payload, &hp); err != nil {
		return handshakePayload{}, err
	}
	if err := writeJSON(conn, opFrame, dispatchEnvelope{Cmd: "DISPATCH", Evt: "READY"}); err != nil {
		return handshakePayload{}, err
	}
	return hp, nil
}

func readSetActivitySafe(conn io.Reader) (setActivityCommand, error) {
	f, err := readFrame(conn)
	if err != nil {
		return setActivityCommand{}, err
	}
	var cmd setActivityCommand
	if err := json.Unmarshal(f.payload, &cmd); err != nil {
		return setActivityCommand{}, err
	}
	return cmd, nil
}

// TestClearAndDisabledSendNullActivity ловит регрессию, где Clear/SetEnabled(false)
// перестали бы слать активити с "activity":null (например, если бы omitempty
// проглотил nil-указатель, или снапшот продолжал бы отдавать старое presence).
func TestClearAndDisabledSendNullActivity(t *testing.T) {
	t.Run("clear", func(t *testing.T) {
		results, errCh, svc := startActivityFeed(t)

		svc.SetEnabled(true)
		waitForActivity(t, results, errCh, true, "baseline")
		svc.Show(Presence{Details: "В игре"})
		waitForActivity(t, results, errCh, false, "after Show")
		svc.Clear()
		waitForActivity(t, results, errCh, true, "after Clear")
	})

	t.Run("disabled", func(t *testing.T) {
		results, errCh, svc := startActivityFeed(t)

		svc.SetEnabled(true)
		waitForActivity(t, results, errCh, true, "baseline")
		svc.Show(Presence{Details: "В игре"})
		waitForActivity(t, results, errCh, false, "after Show")
		svc.SetEnabled(false)
		waitForActivity(t, results, errCh, true, "after SetEnabled(false)")
	})
}

// startActivityFeed поднимает фейковый сервер на net.Pipe, доводит хендшейк
// до READY и непрерывно читает кадры SET_ACTIVITY, публикуя activity==nil в
// results. Читает, пока соединение не закроется при ServiceShutdown.
func startActivityFeed(t *testing.T) (<-chan bool, <-chan error, *Service) {
	t.Helper()
	client, server := net.Pipe()
	svc := newTestService(t, func(context.Context) (io.ReadWriteCloser, error) { return client, nil })

	results := make(chan bool, 16)
	errCh := make(chan error, 1)
	go func() {
		if _, err := writeReadyHandshakeSafe(server); err != nil {
			errCh <- err
			return
		}
		for {
			cmd, err := readSetActivitySafe(server)
			if err != nil {
				return
			}
			results <- cmd.Args.Activity == nil
		}
	}()

	startService(t, svc)
	return results, errCh, svc
}

func waitForActivity(t *testing.T, results <-chan bool, errCh <-chan error, want bool, label string) {
	t.Helper()
	deadline := time.After(testTimeout)
	for {
		select {
		case v := <-results:
			if v == want {
				return
			}
		case err := <-errCh:
			t.Fatalf("fake server: %v", err)
		case <-deadline:
			t.Fatalf("timed out waiting for %s activity frame (want nil=%v)", label, want)
		}
	}
}

// TestOversizedFrameReconnects ловит регрессию, где сервис либо падал бы на
// кадре сверх лимита, либо тихо читал бы «сколько влезет», а не уходил в
// реконнект живым.
func TestOversizedFrameReconnects(t *testing.T) {
	var dials int32
	secondDialConn := make(chan net.Conn, 1)

	dial := func(context.Context) (io.ReadWriteCloser, error) {
		n := atomic.AddInt32(&dials, 1)
		client, server := net.Pipe()
		if n == 1 {
			go func() {
				if _, err := writeReadyHandshakeSafe(server); err != nil {
					return
				}
				if _, err := readSetActivitySafe(server); err != nil {
					return
				}
				var header [8]byte
				binary.LittleEndian.PutUint32(header[0:4], opPing)
				binary.LittleEndian.PutUint32(header[4:8], maxFrameLen+1)
				if _, err := server.Write(header[:]); err != nil {
					return
				}
			}()
		} else {
			select {
			case secondDialConn <- server:
			default:
			}
			go func() {
				if _, err := writeReadyHandshakeSafe(server); err != nil {
					return
				}
			}()
		}
		return client, nil
	}

	svc := newTestService(t, dial)
	svc.SetEnabled(true)
	startService(t, svc)

	select {
	case <-secondDialConn:
	case <-time.After(testTimeout):
		t.Fatalf("second dial did not happen, dials=%d", atomic.LoadInt32(&dials))
	}
	if got := atomic.LoadInt32(&dials); got < 2 {
		t.Fatalf("dials = %d, want >= 2 (service must reconnect after oversized frame)", got)
	}
}

// TestHandshakeRejectedReconnects ловит регрессию, где отказ хендшейка
// (DISPATCH/ERROR вместо READY) не отличался бы от успеха или ронял процесс.
func TestHandshakeRejectedReconnects(t *testing.T) {
	var dials int32
	secondDial := make(chan struct{}, 1)

	dial := func(context.Context) (io.ReadWriteCloser, error) {
		n := atomic.AddInt32(&dials, 1)
		client, server := net.Pipe()
		if n == 1 {
			go func() {
				f, err := readFrame(server)
				if err != nil || f.op != opHandshake {
					return
				}
				if err := writeJSON(server, opFrame, dispatchEnvelope{Cmd: "DISPATCH", Evt: "ERROR"}); err != nil {
					return
				}
			}()
		} else {
			select {
			case secondDial <- struct{}{}:
			default:
			}
		}
		return client, nil
	}

	svc := newTestService(t, dial)
	svc.SetEnabled(true)
	startService(t, svc)

	select {
	case <-secondDial:
	case <-time.After(testTimeout):
		t.Fatalf("second dial did not happen, dials=%d", atomic.LoadInt32(&dials))
	}
}

// TestDialAlwaysFailsCleansUp ловит утечку горутин: если Discord никогда не
// поднимается, ServiceStartup+ServiceShutdown обязаны завершиться, а не
// оставить рабочий цикл висеть в бэкоффе.
func TestDialAlwaysFailsCleansUp(t *testing.T) {
	attempted := make(chan struct{}, 1)
	dial := func(context.Context) (io.ReadWriteCloser, error) {
		select {
		case attempted <- struct{}{}:
		default:
		}
		return nil, errors.New("discord не запущен")
	}

	svc := newTestService(t, dial)
	svc.SetEnabled(true)
	if err := svc.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatalf("ServiceStartup: %v", err)
	}

	select {
	case <-attempted:
	case <-time.After(testTimeout):
		t.Fatal("dial was never attempted")
	}

	shutdownDone := make(chan error, 1)
	go func() { shutdownDone <- svc.ServiceShutdown() }()

	select {
	case err := <-shutdownDone:
		if err != nil {
			t.Fatalf("ServiceShutdown: %v", err)
		}
	case <-time.After(testTimeout):
		t.Fatal("ServiceShutdown hung — goroutine leak")
	}
}

// TestDialNotCalledWhileDisabled ловит регрессию, где выключенный тумблер
// (дефолт сервиса) всё равно открывал канал к Discord и уходил в бесконечный
// реконнект вместо того, чтобы вообще не ходить наружу.
func TestDialNotCalledWhileDisabled(t *testing.T) {
	var dials int32
	dial := func(context.Context) (io.ReadWriteCloser, error) {
		atomic.AddInt32(&dials, 1)
		return nil, errors.New("не должен был вызываться")
	}

	svc := newTestService(t, dial)
	startService(t, svc)

	if got := atomic.LoadInt32(&dials); got != 0 {
		t.Fatalf("dial calls while disabled = %d, want 0", got)
	}
}

// TestDialCalledAfterEnabled проверяет обратную сторону того же поведения:
// включение тумблера должно немедленно разбудить рабочий цикл на дозвон,
// а не ждать до 60 секунд бэкоффа.
func TestDialCalledAfterEnabled(t *testing.T) {
	var dials int32
	dialed := make(chan struct{}, 1)
	dial := func(context.Context) (io.ReadWriteCloser, error) {
		atomic.AddInt32(&dials, 1)
		select {
		case dialed <- struct{}{}:
		default:
		}
		client, server := net.Pipe()
		go func() {
			if _, err := writeReadyHandshakeSafe(server); err != nil {
				return
			}
			for {
				if _, err := readFrame(server); err != nil {
					return
				}
			}
		}()
		return client, nil
	}

	svc := newTestService(t, dial)
	startService(t, svc)

	if got := atomic.LoadInt32(&dials); got != 0 {
		t.Fatalf("dial calls before SetEnabled(true) = %d, want 0", got)
	}

	svc.SetEnabled(true)

	select {
	case <-dialed:
	case <-time.After(testTimeout):
		t.Fatal("dial was never called after SetEnabled(true)")
	}
}

// TestDisableClosesConnectionAfterNullActivity ловит регрессию, где выключение
// тумблера на живом соединении не снимало бы presence перед разрывом (Discord
// остался бы показывать «играет») либо продолжало бы реконнектиться, пока
// пользователь явно не включит presence обратно.
func TestDisableClosesConnectionAfterNullActivity(t *testing.T) {
	var dials int32
	client, server := net.Pipe()
	dial := func(context.Context) (io.ReadWriteCloser, error) {
		atomic.AddInt32(&dials, 1)
		return client, nil
	}

	svc := newTestService(t, dial)
	startService(t, svc)

	frames := make(chan setActivityCommand, 8)
	closed := make(chan struct{})
	go func() {
		if _, err := writeReadyHandshakeSafe(server); err != nil {
			return
		}
		for {
			cmd, err := readSetActivitySafe(server)
			if err != nil {
				close(closed)
				return
			}
			frames <- cmd
		}
	}()

	svc.SetEnabled(true)
	svc.Show(Presence{Details: "В игре"})

	deadline := time.After(testTimeout)
	for shown := false; !shown; {
		select {
		case cmd := <-frames:
			shown = cmd.Args.Activity != nil && cmd.Args.Activity.Details == "В игре"
		case <-deadline:
			t.Fatal("timed out waiting for shown presence frame")
		}
	}

	svc.SetEnabled(false)

	deadline = time.After(testTimeout)
	for null := false; !null; {
		select {
		case cmd := <-frames:
			null = cmd.Args.Activity == nil
		case <-deadline:
			t.Fatal("timed out waiting for null activity frame after disable")
		}
	}

	select {
	case <-closed:
	case <-time.After(testTimeout):
		t.Fatal("connection was not closed after disabling")
	}

	if got := atomic.LoadInt32(&dials); got != 1 {
		t.Fatalf("dials after disable = %d, want 1 (service must not reconnect while disabled)", got)
	}
}

func TestNewServiceValidatesClientID(t *testing.T) {
	cases := []struct {
		name     string
		clientID string
		wantErr  bool
	}{
		{"empty", "", true},
		{"whitespace", "   ", true},
		{"non_numeric", "abc123", true},
		{"valid", "123456789012345678", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewService(tc.clientID)
			if tc.wantErr && err == nil {
				t.Fatalf("NewService(%q) = nil error, want error", tc.clientID)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("NewService(%q) = %v, want nil error", tc.clientID, err)
			}
		})
	}
}

// TestConcurrentShowClearSetEnabledDuringShutdown создаёт реальную гонку между
// вызовами Show/Clear/SetEnabled из-под чужого мьютекса и ServiceShutdown —
// проходит только под go test -race, если методы действительно не блокируют
// и не трогают состояние сервиса без своего мьютекса.
func TestConcurrentShowClearSetEnabledDuringShutdown(t *testing.T) {
	dial := func(context.Context) (io.ReadWriteCloser, error) {
		client, server := net.Pipe()
		go func() {
			if _, err := writeReadyHandshakeSafe(server); err != nil {
				return
			}
			for {
				if _, err := readFrame(server); err != nil {
					return
				}
			}
		}()
		return client, nil
	}

	svc := newTestService(t, dial)
	if err := svc.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatalf("ServiceStartup: %v", err)
	}

	var wg sync.WaitGroup
	stop := make(chan struct{})
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				svc.Show(Presence{Details: "race"})
				svc.SetEnabled(n%2 == 0)
				svc.Clear()
			}
		}(i)
	}

	time.AfterFunc(50*time.Millisecond, func() { close(stop) })
	wg.Wait()

	if err := svc.ServiceShutdown(); err != nil {
		t.Fatalf("ServiceShutdown: %v", err)
	}
}

type recordSink struct {
	slog.Handler
	records chan string
}

func (h recordSink) Enabled(context.Context, slog.Level) bool { return true }

func (h recordSink) Handle(_ context.Context, r slog.Record) error {
	msg := r.Message
	r.Attrs(func(a slog.Attr) bool {
		msg += " " + a.Key + "=" + a.Value.String()
		return true
	})
	select {
	case h.records <- msg:
	default:
	}
	return nil
}

func captureLogs(t *testing.T) <-chan string {
	t.Helper()
	sink := recordSink{Handler: slog.DiscardHandler, records: make(chan string, 16)}
	previous := slog.Default()
	slog.SetDefault(slog.New(sink))
	t.Cleanup(func() { slog.SetDefault(previous) })
	return sink.records
}

// TestRejectedActivityIsReported ловит проглоченную ошибку: Discord отвечает на
// SET_ACTIVITY кадром с evt=ERROR (например, несуществующий ключ ассета), и до
// фикса сервис молча его игнорировал — присутствия нет, в логе пусто.
func TestRejectedActivityIsReported(t *testing.T) {
	logs := captureLogs(t)
	client, server := net.Pipe()
	var dials int32
	svc := newTestService(t, func(context.Context) (io.ReadWriteCloser, error) {
		atomic.AddInt32(&dials, 1)
		return client, nil
	})

	second := make(chan setActivityCommand, 1)
	errCh := make(chan error, 1)
	go func() {
		if _, err := writeReadyHandshakeSafe(server); err != nil {
			errCh <- err
			return
		}
		if _, err := readSetActivitySafe(server); err != nil {
			errCh <- err
			return
		}
		rejection := []byte(`{"cmd":"SET_ACTIVITY","evt":"ERROR","data":{"code":4000,"message":"Invalid activity"},"nonce":"1"}`)
		if err := writeFrame(server, opFrame, rejection); err != nil {
			errCh <- err
			return
		}
		cmd, err := readSetActivitySafe(server)
		if err != nil {
			errCh <- err
			return
		}
		second <- cmd
	}()

	svc.SetEnabled(true)
	startService(t, svc)

	deadline := time.After(testTimeout)
	for {
		select {
		case line := <-logs:
			if !strings.Contains(line, "discord отклонил активность") {
				continue
			}
			if !strings.Contains(line, "4000") || !strings.Contains(line, "Invalid activity") {
				t.Fatalf("в логе нет причины отказа: %q", line)
			}
			goto rejected
		case err := <-errCh:
			t.Fatalf("fake server: %v", err)
		case <-deadline:
			t.Fatal("отказ Discord не попал в лог")
		}
	}

rejected:
	svc.Show(Presence{Details: "В игре"})
	select {
	case cmd := <-second:
		if cmd.Args.Activity == nil || cmd.Args.Activity.Details != "В игре" {
			t.Fatalf("следующая активность = %+v", cmd.Args.Activity)
		}
	case err := <-errCh:
		t.Fatalf("fake server: %v", err)
	case <-time.After(testTimeout):
		t.Fatal("после отказа сервис перестал слать активность")
	}
	if got := atomic.LoadInt32(&dials); got != 1 {
		t.Fatalf("dials = %d, отказ не должен рвать соединение", got)
	}
}
