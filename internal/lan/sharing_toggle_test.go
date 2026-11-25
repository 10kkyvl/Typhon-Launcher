package lan

import (
	"sync"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"

	"typhon/internal/library"
	"typhon/internal/settings"
)

// The settings row and the LAN screen show the figures Go last sent. With no
// shares nothing announces, so a toggle that sent nothing left the previous
// session's figures on screen.
func TestTogglingSharingSendsTheFiguresOfTheNewState(t *testing.T) {
	lib, err := library.NewServiceAt(t.TempDir() + "/library.json")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := lib.ServiceShutdown(); err != nil {
			t.Errorf("library shutdown: %v", err)
		}
	})
	svc := newTestService(t, lib, nil)
	if err := svc.ServiceStartup(t.Context(), application.ServiceOptions{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := svc.ServiceShutdown(); err != nil {
			t.Errorf("lan shutdown: %v", err)
		}
	})

	var mu sync.Mutex
	var sent []Stats
	svc.setHook(func(name string, data any) {
		if name != "lan:stats" {
			return
		}
		stats, ok := data.(Stats)
		if !ok {
			t.Errorf("lan:stats carried %T", data)
			return
		}
		mu.Lock()
		sent = append(sent, stats)
		mu.Unlock()
	})
	take := func() []Stats {
		mu.Lock()
		defer mu.Unlock()
		out := sent
		sent = nil
		return out
	}

	svc.onSettingsChanged(settings.Settings{LANSharing: true})
	if got := take(); len(got) == 0 {
		t.Fatal("turning sharing on sent no lan:stats")
	}

	svc.onSettingsChanged(settings.Settings{LANSharing: false})
	got := take()
	if len(got) == 0 {
		t.Fatal("turning sharing off sent no lan:stats")
	}
	if last := got[len(got)-1]; last.PeersKnown != 0 || last.OffersKnown != 0 {
		t.Fatalf("figures after sharing went off = %+v", last)
	}

	// A change that leaves sharing as it is says nothing.
	svc.onSettingsChanged(settings.Settings{LANSharing: false})
	if got := take(); len(got) != 0 {
		t.Fatalf("an unchanged setting sent %d lan:stats", len(got))
	}
}
