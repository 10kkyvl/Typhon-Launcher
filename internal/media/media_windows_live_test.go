package media

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func liveService(t *testing.T) (*Service, context.Context) {
	t.Helper()
	if os.Getenv("TYPHON_MEDIA_LIVE") != "1" {
		t.Skip("set TYPHON_MEDIA_LIVE=1 to talk to the real system media session")
	}
	svc, err := NewService()
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	if err := svc.ServiceStartup(ctx, application.ServiceOptions{}); err != nil {
		t.Fatalf("ServiceStartup: %v", err)
	}
	t.Cleanup(func() {
		if err := svc.ServiceShutdown(); err != nil {
			t.Errorf("ServiceShutdown: %v", err)
		}
	})
	return svc, ctx
}

func TestLiveCurrent(t *testing.T) {
	svc, ctx := liveService(t)
	for i := 0; i < 3; i++ {
		st, err := svc.Current(ctx)
		if err != nil {
			t.Fatalf("Current #%d: %v", i+1, err)
		}
		if !st.Supported {
			t.Fatalf("Current #%d: Supported is false on Windows", i+1)
		}
		t.Logf("Current #%d: %+v", i+1, st)
	}
}

func awaitPlaying(t *testing.T, svc *Service, ctx context.Context, want bool) State {
	t.Helper()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		st, err := svc.Current(ctx)
		if err != nil {
			t.Fatalf("Current: %v", err)
		}
		if st.Track.Playing == want {
			return st
		}
		select {
		case <-ctx.Done():
			t.Fatalf("Playing did not become %v: last state %+v", want, st)
		case <-tick.C:
		}
	}
}

func TestLiveTogglePlayPause(t *testing.T) {
	if os.Getenv("TYPHON_MEDIA_LIVE_TOGGLE") != "1" {
		t.Skip("set TYPHON_MEDIA_LIVE_TOGGLE=1 to pause and resume whatever is playing")
	}
	svc, ctx := liveService(t)
	before, err := svc.Current(ctx)
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	if !before.Active {
		err := svc.TogglePlayPause(ctx)
		if !errors.Is(err, ErrNoSession) {
			t.Fatalf("TogglePlayPause with no session: got %v, want ErrNoSession", err)
		}
		t.Log("no media session; TogglePlayPause returned ErrNoSession")
		return
	}
	t.Logf("before: %+v", before)
	if err := svc.TogglePlayPause(ctx); err != nil {
		t.Fatalf("TogglePlayPause: %v", err)
	}
	flipped := awaitPlaying(t, svc, ctx, !before.Track.Playing)
	t.Logf("after one toggle: %+v", flipped)
	if err := svc.TogglePlayPause(ctx); err != nil {
		t.Fatalf("TogglePlayPause back: %v", err)
	}
	restored := awaitPlaying(t, svc, ctx, before.Track.Playing)
	t.Logf("after the second toggle: %+v", restored)
}
