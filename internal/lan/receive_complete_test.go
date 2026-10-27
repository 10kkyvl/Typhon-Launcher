package lan

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"typhon/internal/library"

	g "github.com/anacrolix/generics"
	"github.com/anacrolix/torrent/metainfo"
	tstorage "github.com/anacrolix/torrent/storage"
)

type gatedStorage struct {
	tstorage.ClientImplCloser
	gate func()
}

func (s gatedStorage) OpenTorrent(ctx context.Context, info *metainfo.Info, infoHash metainfo.Hash) (tstorage.TorrentImpl, error) {
	ti, err := s.ClientImplCloser.OpenTorrent(ctx, info, infoHash)
	if err != nil {
		return ti, err
	}
	if piece := ti.Piece; piece != nil {
		ti.Piece = func(p metainfo.Piece) tstorage.PieceImpl {
			return gatedPiece{PieceImpl: piece(p), gate: s.gate}
		}
	}
	if piece := ti.PieceWithHash; piece != nil {
		ti.PieceWithHash = func(p metainfo.Piece, hash g.Option[[]byte]) tstorage.PieceImpl {
			return gatedPiece{PieceImpl: piece(p, hash), gate: s.gate}
		}
	}
	return ti, nil
}

type gatedPiece struct {
	tstorage.PieceImpl
	gate func()
}

func (p gatedPiece) MarkComplete() error {
	p.gate()
	return p.PieceImpl.MarkComplete()
}

func TestReceiveWaitsForPartFilePromotion(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	var enterOnce, releaseOnce sync.Once
	releaseAll := func() { releaseOnce.Do(func() { close(release) }) }
	gate := func() {
		enterOnce.Do(func() { close(entered) })
		<-release
	}

	orig := receiveStorage
	t.Cleanup(func() { receiveStorage = orig })
	receiveStorage = func(dest string) tstorage.ClientImplCloser {
		return gatedStorage{ClientImplCloser: flatStorage(dest, false), gate: gate}
	}

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
	svcB := newTestService(t, libB, newTestSettings(t, t.TempDir()))

	transportA, transportB := loopbackPair(t)
	svcA.newTransport = transportA
	svcB.newTransport = transportB
	svcA.torrentPort = 46003
	svcB.torrentPort = 46004

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
	t.Cleanup(releaseAll)

	peersSeen := make(chan struct{}, 8)
	svcB.setHook(func(name string, _ any) {
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
	for found := false; !found; {
		select {
		case <-peersSeen:
			for _, o := range svcB.Available() {
				if o.InfoHash == share.InfoHash {
					offer, found = o, true
				}
			}
		case <-time.After(10 * time.Second):
			t.Fatal("timed out waiting for B to observe A's announce")
		}
	}

	events := make(chan Transfer, 1024)
	svcB.setHook(func(name string, data any) {
		if tr, ok := data.(Transfer); ok && name == "lan:transfer" {
			events <- tr
		}
	})
	next := func(what string) Transfer {
		t.Helper()
		select {
		case tr := <-events:
			return tr
		case <-time.After(30 * time.Second):
			t.Fatalf("timed out waiting for %s", what)
			return Transfer{}
		}
	}
	terminal := func(tr Transfer) bool {
		return tr.Status == TransferCompleted || tr.Status == TransferFailed || tr.Status == TransferCancelled
	}

	if _, err := svcB.Receive(offer.InfoHash, offer.PeerID); err != nil {
		t.Fatalf("Receive: %v", err)
	}

	select {
	case <-entered:
	case <-time.After(30 * time.Second):
		t.Fatal("timed out waiting for the last piece to reach MarkComplete")
	}

	for {
		tr := next("a progress sample with every byte written")
		if terminal(tr) {
			t.Fatalf("transfer finished while MarkComplete was blocked: status = %s, error = %s", tr.Status, tr.Error)
		}
		if tr.Total > 0 && tr.Downloaded >= tr.Total {
			break
		}
	}
	if tr := next("the sample after every byte was written"); terminal(tr) {
		t.Fatalf("transfer finished while MarkComplete was blocked: status = %s, error = %s", tr.Status, tr.Error)
	}

	releaseAll()

	final := next("the transfer to finish")
	for !terminal(final) {
		final = next("the transfer to finish")
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
}
