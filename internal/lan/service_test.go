package lan

import (
	"context"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"typhon/internal/library"
	"typhon/internal/platform"
	"typhon/internal/settings"
)

func newTestLibraryWithGame(t *testing.T) (*library.Service, library.Game, string) {
	t.Helper()
	lib, err := library.NewServiceAt(filepath.Join(t.TempDir(), "library.json"))
	if err != nil {
		t.Fatalf("new library service: %v", err)
	}
	t.Cleanup(func() {
		if err := lib.ServiceShutdown(); err != nil {
			t.Errorf("library shutdown: %v", err)
		}
	})

	installDir := t.TempDir()
	writeTree(t, installDir)
	exe := filepath.Join(installDir, "game.exe")

	game, err := lib.RegisterInstalled(library.InstalledGame{
		Title:      "Test Game",
		Executable: exe,
		InstallDir: installDir,
		Version:    "1.0",
	})
	if err != nil {
		t.Fatalf("register installed: %v", err)
	}
	return lib, game, installDir
}

func newTestService(t *testing.T, lib *library.Service, settingsSvc *settings.Service) *Service {
	t.Helper()
	svc, err := NewServiceAt(t.TempDir(), settingsSvc, lib)
	if err != nil {
		t.Fatalf("new lan service: %v", err)
	}
	return svc
}

func TestShareCacheSkipsRehash(t *testing.T) {
	lib, game, installDir := newTestLibraryWithGame(t)
	svc := newTestService(t, lib, nil)
	if err := svc.enable(context.Background()); err != nil {
		t.Fatalf("enable: %v", err)
	}
	t.Cleanup(func() {
		if err := svc.ServiceShutdown(); err != nil {
			t.Errorf("shutdown: %v", err)
		}
	})

	var mu sync.Mutex
	hashTicks := 0
	svc.setHook(func(name string, data any) {
		if name != "lan:hashing" {
			return
		}
		hp, ok := data.(HashProgress)
		if !ok || hp.Done {
			return
		}
		mu.Lock()
		hashTicks++
		mu.Unlock()
	})
	reset := func() { mu.Lock(); hashTicks = 0; mu.Unlock() }
	ticks := func() int { mu.Lock(); defer mu.Unlock(); return hashTicks }

	sh1, err := svc.Share(game.ID)
	if err != nil {
		t.Fatalf("first Share: %v", err)
	}
	if ticks() == 0 {
		t.Fatal("expected hashing progress on the first Share of a new tree")
	}

	reset()
	sh2, err := svc.Share(game.ID)
	if err != nil {
		t.Fatalf("second Share: %v", err)
	}
	if got := ticks(); got != 0 {
		t.Fatalf("second Share rehashed an unchanged tree: %d progress ticks", got)
	}
	if sh1.InfoHash != sh2.InfoHash {
		t.Fatalf("InfoHash changed across an unchanged Share: %s != %s", sh1.InfoHash, sh2.InfoHash)
	}

	future := time.Now().Add(time.Hour)
	if err := os.Chtimes(filepath.Join(installDir, "game.exe"), future, future); err != nil {
		t.Fatal(err)
	}
	reset()
	if _, err := svc.Share(game.ID); err != nil {
		t.Fatalf("third Share: %v", err)
	}
	if ticks() == 0 {
		t.Fatal("expected rehash after the tree's mtime changed")
	}
}

func TestServiceShutdownStopsGoroutines(t *testing.T) {
	lib, _, _ := newTestLibraryWithGame(t)
	svc := newTestService(t, lib, nil)

	probe, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	if err != nil {
		t.Fatalf("pick loopback port: %v", err)
	}
	udpAddr, ok := probe.LocalAddr().(*net.UDPAddr)
	if !ok {
		t.Fatalf("probe local addr is %T, want *net.UDPAddr", probe.LocalAddr())
	}
	listenAddr := udpAddr.AddrPort()
	if err := probe.Close(); err != nil {
		t.Fatal(err)
	}
	svc.newTransport = func() (transport, error) { return newLoopback(nil, listenAddr) }

	if err := svc.enable(context.Background()); err != nil {
		t.Fatalf("enable: %v", err)
	}
	if err := svc.ServiceShutdown(); err != nil {
		t.Fatalf("shutdown: %v", err)
	}

	again, err := net.ListenUDP("udp", net.UDPAddrFromAddrPort(listenAddr))
	if err != nil {
		t.Fatalf("announce port not released after shutdown: %v", err)
	}
	if err := again.Close(); err != nil {
		t.Fatal(err)
	}
}

func newTestSettings(t *testing.T, gamesRoot string) *settings.Service {
	t.Helper()
	svc, err := settings.NewServiceAt(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("new settings service: %v", err)
	}
	next := svc.GetSettings()
	next.LibraryPath = gamesRoot
	if err := svc.SaveSettings(next); err != nil {
		t.Fatalf("save settings: %v", err)
	}
	return svc
}

// loopbackPair binds two ephemeral UDP ports (closing them immediately) and
// wires two loopback transport factories pointed at each other, so a test
// can run two Services' announce protocol in one process without joining a
// real multicast group.
func loopbackPair(t *testing.T) (aTransport, bTransport func() (transport, error)) {
	t.Helper()
	pick := func() netip.AddrPort {
		conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
		if err != nil {
			t.Fatalf("pick loopback port: %v", err)
		}
		udpAddr, ok := conn.LocalAddr().(*net.UDPAddr)
		if !ok {
			t.Fatalf("probe local addr is %T, want *net.UDPAddr", conn.LocalAddr())
		}
		if err := conn.Close(); err != nil {
			t.Fatalf("close probe: %v", err)
		}
		return udpAddr.AddrPort()
	}
	addrA, addrB := pick(), pick()
	return func() (transport, error) {
			return newLoopback([]netip.AddrPort{addrB}, addrA)
		}, func() (transport, error) {
			return newLoopback([]netip.AddrPort{addrA}, addrB)
		}
}

// TestLoopbackEndToEnd is the acceptance test for the whole package: two
// Services, two real torrent.Client instances talking over real (loopback)
// sockets, announcing over a loopback UDP transport instead of multicast.
// One shares a small install, the other discovers it, downloads it and
// registers it as a game with the right executable path.
func TestLoopbackEndToEnd(t *testing.T) {
	libA, gameA, _ := newTestLibraryWithGame(t)
	svcA := newTestService(t, libA, nil)

	libB, err := library.NewServiceAt(filepath.Join(t.TempDir(), "library.json"))
	if err != nil {
		t.Fatalf("new library B: %v", err)
	}
	t.Cleanup(func() {
		if err := libB.ServiceShutdown(); err != nil {
			t.Errorf("library B shutdown: %v", err)
		}
	})
	gamesRoot := t.TempDir()
	settingsB := newTestSettings(t, gamesRoot)
	svcB := newTestService(t, libB, settingsB)

	transportA, transportB := loopbackPair(t)
	svcA.newTransport = transportA
	svcB.newTransport = transportB
	// Distinct fixed ports: two Services in one process must not race for
	// the package's default listenPort the way two machines never would.
	svcA.torrentPort = 46001
	svcB.torrentPort = 46002

	ctx := context.Background()
	if err := svcA.enable(ctx); err != nil {
		t.Fatalf("enable A: %v", err)
	}
	t.Cleanup(func() {
		if err := svcA.ServiceShutdown(); err != nil {
			t.Errorf("shutdown A: %v", err)
		}
	})
	if err := svcB.enable(ctx); err != nil {
		t.Fatalf("enable B: %v", err)
	}
	t.Cleanup(func() {
		if err := svcB.ServiceShutdown(); err != nil {
			t.Errorf("shutdown B: %v", err)
		}
	})

	peersSeen := make(chan struct{}, 8)
	svcB.setHook(func(name string, data any) {
		if name == "lan:peers" {
			select {
			case peersSeen <- struct{}{}:
			default:
			}
		}
	})

	share, err := svcA.Share(gameA.ID)
	if err != nil {
		t.Fatalf("Share: %v", err)
	}

	var offer Offer
	found := false
	for !found {
		select {
		case <-peersSeen:
			for _, o := range svcB.Available() {
				if o.InfoHash == share.InfoHash {
					offer = o
					found = true
					break
				}
			}
		case <-time.After(10 * time.Second):
			t.Fatal("timed out waiting for B to observe A's announce")
		}
	}

	transferDone := make(chan Transfer, 8)
	svcB.setHook(func(name string, data any) {
		if name != "lan:transfer" {
			return
		}
		tr, ok := data.(Transfer)
		if ok && (tr.Status == TransferCompleted || tr.Status == TransferFailed) {
			select {
			case transferDone <- tr:
			default:
			}
		}
	})

	if _, err := svcB.Receive(offer.InfoHash, offer.PeerID); err != nil {
		t.Fatalf("Receive: %v", err)
	}

	var final Transfer
	select {
	case final = <-transferDone:
	case <-time.After(30 * time.Second):
		t.Fatal("timed out waiting for the transfer to finish")
	}
	if final.Status != TransferCompleted {
		t.Fatalf("transfer status = %s, error = %s", final.Status, final.Error)
	}

	game, err := libB.Find(final.GameID)
	if err != nil {
		t.Fatalf("find received game: %v", err)
	}
	if _, err := os.Stat(game.Executable); err != nil {
		t.Fatalf("registered executable missing: %v", err)
	}
	if !platform.Inside(game.InstallDir, game.Executable) {
		t.Fatalf("executable %s is not inside install dir %s", game.Executable, game.InstallDir)
	}
}
