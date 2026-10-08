package media

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"typhon/internal/uierr"
)

type fake struct {
	mu          sync.Mutex
	unsupported bool
	track       Track
	active      bool
	err         error
	block       bool
	started     bool
	startErr    error
	startGate   chan struct{}
	stopErr     error
	cmds        []command
}

func (f *fake) supported() bool { return !f.unsupported }

func (f *fake) start(context.Context) error {
	if f.startGate != nil {
		<-f.startGate
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.started = f.startErr == nil
	return f.startErr
}

func (f *fake) stop() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.started = false
	return f.stopErr
}

func (f *fake) current(ctx context.Context) (Track, bool, error) {
	if f.block {
		<-ctx.Done()
		return Track{}, false, ctx.Err()
	}
	return f.track, f.active, f.err
}

func (f *fake) control(ctx context.Context, cmd command) error {
	f.mu.Lock()
	f.cmds = append(f.cmds, cmd)
	f.mu.Unlock()
	if f.block {
		<-ctx.Done()
		return ctx.Err()
	}
	return f.err
}

func newFake(f *fake) *Service {
	return &Service{p: f, timeout: time.Minute}
}

func TestCurrent(t *testing.T) {
	long := strings.Repeat("x", 500)
	tests := []struct {
		name string
		f    *fake
		want State
	}{
		{"unsupported platform", &fake{unsupported: true}, State{}},
		{"no session", &fake{}, State{Supported: true}},
		{
			"track is normalised",
			&fake{active: true, track: Track{
				App: "SpotifyAB.SpotifyMusic_zpdnekdrzrea0!Spotify", Title: "So\x00ng\n", Artist: long, Album: "\u202eAlbum",
				Playing: true, CanPlayPause: true, CanNext: true,
			}},
			State{Supported: true, Active: true, Track: Track{
				App: "Spotify", Title: "Song", Artist: strings.Repeat("x", maxText), Album: "Album",
				Playing: true, CanPlayPause: true, CanNext: true,
			}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := newFake(tt.f).Current(context.Background())
			if err != nil {
				t.Fatalf("Current: %v", err)
			}
			if got != tt.want {
				t.Fatalf("Current = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestCurrentReturnsPlatformErrors(t *testing.T) {
	boom := errors.New("boom")
	got, err := newFake(&fake{err: boom, active: true, track: Track{Title: "stale"}}).Current(context.Background())
	if !errors.Is(err, boom) {
		t.Fatalf("Current error = %v, want it to wrap %v", err, boom)
	}
	if got != (State{}) {
		t.Fatalf("Current state = %+v on error, want zero", got)
	}
}

func TestCurrentGivesUpAfterTheTimeout(t *testing.T) {
	svc := newFake(&fake{block: true})
	svc.timeout = time.Millisecond
	_, err := svc.Current(context.Background())
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Current error = %v, want DeadlineExceeded", err)
	}
}

func TestCurrentHonoursACancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := newFake(&fake{block: true}).Current(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Current error = %v, want Canceled", err)
	}
}

func TestCommands(t *testing.T) {
	calls := []struct {
		name string
		run  func(*Service, context.Context) error
		cmd  command
	}{
		{"toggle", (*Service).TogglePlayPause, cmdTogglePlayPause},
		{"next", (*Service).Next, cmdNext},
		{"previous", (*Service).Previous, cmdPrevious},
	}
	for _, c := range calls {
		t.Run(c.name+"/forwarded", func(t *testing.T) {
			f := &fake{}
			if err := c.run(newFake(f), context.Background()); err != nil {
				t.Fatalf("%s: %v", c.name, err)
			}
			if len(f.cmds) != 1 || f.cmds[0] != c.cmd {
				t.Fatalf("platform got %v, want [%v]", f.cmds, c.cmd)
			}
		})
		t.Run(c.name+"/unsupported", func(t *testing.T) {
			f := &fake{unsupported: true}
			if err := c.run(newFake(f), context.Background()); !errors.Is(err, ErrUnsupported) {
				t.Fatalf("%s error = %v, want ErrUnsupported", c.name, err)
			}
			if len(f.cmds) != 0 {
				t.Fatalf("platform was called on an unsupported system: %v", f.cmds)
			}
		})
		for _, want := range []error{ErrNoSession, ErrRefused, ErrNotStarted} {
			t.Run(c.name+"/"+want.Error(), func(t *testing.T) {
				err := c.run(newFake(&fake{err: want}), context.Background())
				if !errors.Is(err, want) {
					t.Fatalf("%s error = %v, want %v", c.name, err, want)
				}
			})
		}
		t.Run(c.name+"/timeout", func(t *testing.T) {
			svc := newFake(&fake{block: true})
			svc.timeout = time.Millisecond
			if err := c.run(svc, context.Background()); !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("%s error = %v, want DeadlineExceeded", c.name, err)
			}
		})
	}
}

func TestUnavailableMediaDoesNotStopTheLauncher(t *testing.T) {
	boom := errors.New("RoInitialize: HRESULT 0x80004005")
	cases := []struct {
		name string
		make func() (*Service, *fake)
	}{
		{"apartment does not start", func() (*Service, *fake) {
			f := &fake{startErr: boom, active: true, track: Track{Title: "stale"}}
			return newFake(f), f
		}},
		{"platform does not load", func() (*Service, *fake) { return newService(nil, boom), nil }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			svc, f := c.make()
			if err := svc.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
				t.Fatalf("ServiceStartup = %v: media controls are optional and must not abort startup", err)
			}
			check := func(what string, err error) {
				t.Helper()
				if !errors.Is(err, ErrUnavailable) || !errors.Is(err, boom) {
					t.Fatalf("%s error = %v, want ErrUnavailable wrapping %v", what, err, boom)
				}
				if code := uierr.Code(err); code != "media.unavailable" {
					t.Fatalf("%s code = %q, want media.unavailable", what, code)
				}
			}
			st, err := svc.Current(context.Background())
			check("Current", err)
			if st != (State{}) {
				t.Fatalf("Current state = %+v, an unavailable service must not look like an idle one", st)
			}
			for name, run := range map[string]func(context.Context) error{
				"toggle": svc.TogglePlayPause, "next": svc.Next, "previous": svc.Previous,
			} {
				check(name, run(context.Background()))
			}
			if f != nil && len(f.cmds) != 0 {
				t.Fatalf("platform was called while unavailable: %v", f.cmds)
			}
			if err := svc.ServiceShutdown(); err != nil {
				t.Fatalf("ServiceShutdown: %v", err)
			}
		})
	}
}

func TestCallsInFlightSeeTheServiceTurnUnavailable(t *testing.T) {
	boom := errors.New("RoInitialize: HRESULT 0x80004005")
	gate := make(chan struct{})
	svc := newFake(&fake{startErr: boom, startGate: gate, active: true, track: Track{Title: "song"}})
	calls := map[string]func(context.Context) error{
		"current": func(ctx context.Context) error {
			_, err := svc.Current(ctx)
			return err
		},
		"toggle": svc.TogglePlayPause, "next": svc.Next, "previous": svc.Previous,
	}

	const perCall = 3
	var inFlight, done sync.WaitGroup
	wrong := make(chan string, len(calls)*perCall)
	for name, call := range calls {
		for range perCall {
			inFlight.Add(1)
			done.Add(1)
			go func() {
				defer done.Done()
				err := call(context.Background())
				inFlight.Done()
				for err == nil {
					err = call(context.Background())
				}
				if !errors.Is(err, ErrUnavailable) || !errors.Is(err, boom) {
					wrong <- fmt.Sprintf("%s ended with %v", name, err)
				}
			}()
		}
	}
	started := make(chan error, 1)
	go func() { started <- svc.ServiceStartup(context.Background(), application.ServiceOptions{}) }()

	inFlight.Wait()
	close(gate)
	done.Wait()
	if err := <-started; err != nil {
		t.Fatalf("ServiceStartup = %v", err)
	}
	close(wrong)
	for msg := range wrong {
		t.Errorf("%s, want ErrUnavailable wrapping %v", msg, boom)
	}
}

func TestLifecycleErrorsReachTheCaller(t *testing.T) {
	boom := errors.New("boom")
	f := &fake{stopErr: boom}
	svc := newFake(f)
	if err := svc.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatalf("ServiceStartup: %v", err)
	}
	if !f.started {
		t.Fatal("ServiceStartup did not start the platform")
	}
	if err := svc.ServiceShutdown(); !errors.Is(err, boom) {
		t.Fatalf("ServiceShutdown error = %v, want it to wrap %v", err, boom)
	}
}

func TestStateJSON(t *testing.T) {
	raw, err := json.Marshal(State{Supported: true, Active: true, Track: Track{CanPlayPause: true}})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	track, ok := got["track"].(map[string]any)
	if !ok {
		t.Fatalf("no track object in %s", raw)
	}
	for _, key := range []string{"supported", "active"} {
		if _, ok := got[key]; !ok {
			t.Errorf("state JSON has no %q: %s", key, raw)
		}
	}
	for _, key := range []string{"app", "title", "artist", "album", "playing", "canPlayPause", "canNext", "canPrev"} {
		if _, ok := track[key]; !ok {
			t.Errorf("track JSON has no %q: %s", key, raw)
		}
	}
}
