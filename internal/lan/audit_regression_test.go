package lan

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
	"typhon/internal/library"
	"typhon/internal/platform"
)

func TestReceivePreservesOccupiedDestinations(t *testing.T) {
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
	svcA.torrentPort = 47001
	svcB.torrentPort = 47002

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

	primary := filepath.Join(svcB.gamesPath(), sanitizeFolderName(offer.Title, offer.GameID))
	fallback := primary + "-" + offer.InfoHash[:8]
	for _, dir := range []string{primary, fallback} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "game.exe"), []byte("MY EXISTING GAME"), 0644); err != nil {
			t.Fatal(err)
		}
	}
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
	got, err := os.ReadFile(filepath.Join(fallback, "game.exe"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "MY EXISTING GAME" {
		t.Fatal("not reproduced")
	}
}
