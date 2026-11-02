package lan

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"typhon/internal/library"
	"typhon/internal/platform"

	g "github.com/anacrolix/generics"
	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
	tstorage "github.com/anacrolix/torrent/storage"
)

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func captureLogs(t *testing.T) *lockedBuffer {
	t.Helper()
	logs := &lockedBuffer{}
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	return logs
}

func shareWhole(t *testing.T, svc *Service, game library.Game, _ string) Share {
	t.Helper()
	share, err := svc.Share(game.ID)
	if err != nil {
		t.Fatalf("Share: %v", err)
	}
	return share
}

// shareLikeOldLauncher publishes a torrent of the whole folder, marker included,
// which is what a sender before this fix announces.
func shareLikeOldLauncher(t *testing.T, svc *Service, game library.Game, root string) Share {
	t.Helper()
	return publishOldTorrent(t, svc, game, root, "game.exe")
}

func publishOldTorrent(t *testing.T, svc *Service, game library.Game, root, exe string) Share {
	t.Helper()
	var info metainfo.Info
	if err := info.BuildFromFilePath(root); err != nil {
		t.Fatalf("build info: %v", err)
	}
	if !torrentHasPathPrefix(info, library.MarkerName) {
		t.Fatalf("torrent of an old sender has no marker: %+v", info.Files)
	}
	infoHash, data, err := buildTorrent(info)
	if err != nil {
		t.Fatalf("build torrent: %v", err)
	}
	if err := svc.store.writeTorrent(infoHash, data); err != nil {
		t.Fatalf("write torrent: %v", err)
	}
	share := Share{
		GameID:      game.ID,
		Title:       game.Title,
		Exe:         exe,
		Root:        root,
		InfoHash:    infoHash,
		SizeBytes:   info.TotalLength(),
		Fingerprint: "before-the-marker-was-excluded",
		BuiltAt:     time.Now(),
		Enabled:     true,
	}
	if err := svc.persistShare(share); err != nil {
		t.Fatalf("persist share: %v", err)
	}
	run := liveRun(svc)
	if err := svc.startSeeding(run, share); err != nil {
		t.Fatalf("start seeding: %v", err)
	}
	run.signalChanged()
	return share
}

func torrentHasPathPrefix(info metainfo.Info, first string) bool {
	for _, f := range info.Files {
		if len(f.Path) > 0 && f.Path[0] == first {
			return true
		}
	}
	return false
}

type markerTransfer struct {
	final   Transfer
	lib     *library.Service
	source  library.Game
	torrent *torrent.Torrent
	sender  *Service
	share   Share
}

type markerPair struct {
	svcA, svcB *Service
	libB       *library.Service
	gameA      library.Game
	rootA      string
	share      Share
	offer      Offer
	dest       string
}

func (p *markerPair) receive(t *testing.T) (Transfer, <-chan Transfer) {
	t.Helper()
	done := make(chan Transfer, 1)
	p.svcB.setHook(func(name string, data any) {
		tr, ok := data.(Transfer)
		if !ok || name != "lan:transfer" {
			return
		}
		if tr.Status == TransferCompleted || tr.Status == TransferFailed || tr.Status == TransferCancelled {
			select {
			case done <- tr:
			default:
			}
		}
	})
	tr, err := p.svcB.Receive(p.offer.InfoHash, p.offer.PeerID)
	if err != nil {
		t.Fatalf("Receive: %v", err)
	}
	return tr, done
}

func waitTransfer(t *testing.T, done <-chan Transfer) Transfer {
	t.Helper()
	select {
	case final := <-done:
		return final
	case <-time.After(30 * time.Second):
		t.Fatal("timed out waiting for the transfer to finish")
		return Transfer{}
	}
}

func liveRun(svc *Service) *runState {
	svc.mu.Lock()
	defer svc.mu.Unlock()
	return svc.run
}

func runMarkerTransfer(t *testing.T, port int, prepare func(root string), publish func(*testing.T, *Service, library.Game, string) Share) markerTransfer {
	t.Helper()
	p := newMarkerPair(t, port, prepare, publish)
	_, done := p.receive(t)
	final := waitTransfer(t, done)
	return markerTransfer{
		final:   final,
		lib:     p.libB,
		source:  p.gameA,
		torrent: p.svcB.transferTorrent(liveRun(p.svcB), final.ID),
		sender:  p.svcA,
		share:   p.share,
	}
}

func newMarkerPair(t *testing.T, port int, prepare func(root string), publish func(*testing.T, *Service, library.Game, string) Share) *markerPair {
	t.Helper()
	libA, gameA, rootA := newTestLibraryWithGame(t)
	if prepare != nil {
		prepare(rootA)
	}
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
	svcA.torrentPort = port
	svcB.torrentPort = port + 1

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
	svcB.setHook(func(name string, _ any) {
		if name == "lan:peers" {
			select {
			case peersSeen <- struct{}{}:
			default:
			}
		}
	})

	share := publish(t, svcA, gameA, rootA)

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

	return &markerPair{
		svcA:  svcA,
		svcB:  svcB,
		libB:  libB,
		gameA: gameA,
		rootA: rootA,
		share: share,
		offer: offer,
		dest:  filepath.Join(svcB.gamesPath(), sanitizeFolderName(offer.Title, offer.GameID)),
	}
}

func TestReceivedGameCarriesReceiversMarker(t *testing.T) {
	tests := []struct {
		name    string
		port    int
		publish func(*testing.T, *Service, library.Game, string) Share
	}{
		{name: "new sender leaves the marker out", port: 46005, publish: shareWhole},
		{name: "old sender sends its marker", port: 46007, publish: shareLikeOldLauncher},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			logs := captureLogs(t)
			got := runMarkerTransfer(t, tc.port, nil, tc.publish)
			if got.final.Status != TransferCompleted {
				t.Fatalf("transfer status = %s, error = %s", got.final.Status, got.final.Error)
			}

			game, err := got.lib.Find(got.final.GameID)
			if err != nil {
				t.Fatalf("find received game: %v", err)
			}
			marker, err := library.ReadMarker(game.InstallDir)
			if err != nil {
				t.Fatalf("read marker: %v", err)
			}
			if marker.GameID != game.ID {
				t.Fatalf("marker game id = %q, want receiver's %q (sender's is %q)", marker.GameID, game.ID, got.source.ID)
			}
			if marker.InstallType != "lan" {
				t.Fatalf("marker install type = %q, want lan", marker.InstallType)
			}
			if text := logs.String(); strings.Contains(text, "write install marker") {
				t.Fatalf("marker write was refused:\n%s", text)
			}
			select {
			case <-got.torrent.Closed():
			default:
				t.Fatal("torrent is still live: it would fetch the removed marker back")
			}
		})
	}
}

func keepEngineModes(t *testing.T, fn func(context.Context, string) error) {
	t.Helper()
	orig := clearReadOnly
	t.Cleanup(func() { clearReadOnly = orig })
	clearReadOnly = fn
}

func TestReceivedFilesAreReadOnly(t *testing.T) {
	keepEngineModes(t, func(context.Context, string) error { return nil })
	got := runMarkerTransfer(t, 46009, nil, shareLikeOldLauncher)
	if got.final.Status != TransferCompleted {
		t.Fatalf("transfer status = %s, error = %s", got.final.Status, got.final.Error)
	}
	game, err := got.lib.Find(got.final.GameID)
	if err != nil {
		t.Fatalf("find received game: %v", err)
	}
	info, err := os.Stat(game.Executable)
	if err != nil {
		t.Fatalf("stat received file: %v", err)
	}
	if info.Mode().Perm()&0o200 != 0 {
		t.Fatalf("received file mode = %v, the engine no longer marks promoted files read-only", info.Mode())
	}
}

func TestReceivedFilesAreWritableAndShareable(t *testing.T) {
	got := runMarkerTransfer(t, 46013, nil, shareWhole)
	if got.final.Status != TransferCompleted {
		t.Fatalf("transfer status = %s, error = %s", got.final.Status, got.final.Error)
	}
	game, err := got.lib.Find(got.final.GameID)
	if err != nil {
		t.Fatalf("find received game: %v", err)
	}

	err = filepath.WalkDir(game.InstallDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || !d.Type().IsRegular() {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.Mode().Perm()&0o200 == 0 {
			t.Errorf("%s stayed read-only: %v", path, info.Mode())
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk received game: %v", err)
	}

	data, err := got.sender.store.readTorrent(got.share.InfoHash)
	if err != nil {
		t.Fatalf("read torrent: %v", err)
	}
	mi, err := metainfo.Load(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("parse torrent: %v", err)
	}
	sent, err := mi.UnmarshalInfo()
	if err != nil {
		t.Fatalf("parse info: %v", err)
	}
	again, err := BuildInfo(context.Background(), game.InstallDir, nil)
	if err != nil {
		t.Fatalf("share received game back: %v", err)
	}
	if !bytes.Equal(again.Pieces, sent.Pieces) {
		t.Fatal("received files no longer hash to what was sent")
	}
}

func TestReceiveFailsWhenFilesCannotBeMadeWritable(t *testing.T) {
	keepEngineModes(t, func(context.Context, string) error { return errors.New("access denied") })

	got := runMarkerTransfer(t, 46015, nil, shareWhole)

	if got.final.Status != TransferFailed {
		t.Fatalf("transfer status = %s, want failed", got.final.Status)
	}
	for _, want := range []string{"typhon:lan.finish_files_failed", "make received files writable", "access denied"} {
		if !strings.Contains(got.final.Error, want) {
			t.Fatalf("transfer error = %q, want it to contain %q", got.final.Error, want)
		}
	}
	if games := got.lib.GetGames(); len(games) != 0 {
		t.Fatalf("library took a game with read-only files: %+v", games)
	}
}

func stagingDir(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), receivingPrefix+"test")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestCompleteTransferSurvivesCancelledContext(t *testing.T) {
	dest := stagingDir(t)
	exe := filepath.Join(dest, "game.exe")
	marker := filepath.Join(dest, library.MarkerName)
	for _, path := range []string{exe, marker} {
		if err := os.WriteFile(path, []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(path, 0o400); err != nil {
			t.Fatal(err)
		}
	}
	lib, err := library.NewServiceAt(filepath.Join(t.TempDir(), "library.json"))
	if err != nil {
		t.Fatalf("new library: %v", err)
	}
	t.Cleanup(func() {
		if err := lib.ServiceShutdown(); err != nil {
			t.Errorf("library shutdown: %v", err)
		}
	})
	state := &transferState{transfer: Transfer{ID: "transfer", Status: TransferReceiving}}
	run := &runState{transfers: map[string]*transferState{"transfer": state}}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	newTestService(t, lib, nil).completeTransfer(ctx, run, "transfer", dest, Offer{Title: "Game", Exe: "game.exe"}, nil)

	if state.transfer.Status != TransferCompleted {
		t.Fatalf("a fully received game was lost to a closing launcher: %+v", state.transfer)
	}
	game, err := lib.Find(state.transfer.GameID)
	if err != nil {
		t.Fatalf("find game: %v", err)
	}
	if got, err := library.ReadMarker(game.InstallDir); err != nil || got.GameID != game.ID {
		t.Fatalf("marker = %+v, %v, want game %s", got, err, game.ID)
	}
	if _, err := os.Stat(dest); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("receiving folder stayed: %v", err)
	}
	info, err := os.Stat(game.Executable)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o200 == 0 {
		t.Fatal("executable stayed read-only")
	}
}

func TestCompleteTransferDiscardsFolderWhenExecutableIsRefused(t *testing.T) {
	for _, test := range []struct{ exe, code string }{
		{"../outside.exe", "lan.exe_outside_install"},
		{"missing.exe", "lan.executable_missing"},
	} {
		t.Run(test.code, func(t *testing.T) {
			dest := stagingDir(t)
			marker := filepath.Join(dest, library.MarkerName)
			if err := os.WriteFile(marker, []byte("{}"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(marker, 0o400); err != nil {
				t.Fatal(err)
			}
			state := &transferState{transfer: Transfer{ID: "transfer", Status: TransferReceiving}}
			run := &runState{transfers: map[string]*transferState{"transfer": state}}

			(&Service{}).completeTransfer(t.Context(), run, "transfer", dest, Offer{Exe: test.exe}, nil)

			if state.transfer.Status != TransferFailed || !strings.Contains(state.transfer.Error, "typhon:"+test.code) {
				t.Fatalf("transfer = %+v, want failed with %s", state.transfer, test.code)
			}
			if _, err := os.Stat(dest); !errors.Is(err, fs.ErrNotExist) {
				t.Fatalf("refused transfer's folder stayed: %v", err)
			}
		})
	}
}

// firstPieceStorage reports when piece 0, which holds the marker of an old
// sender, is promoted and holds only the last piece at the gate: a piece stuck
// at the gate occupies a hasher, and enough of them would starve piece 0.
type firstPieceStorage struct {
	tstorage.ClientImplCloser
	firstDone func()
	gate      func()
}

type reportingPiece struct {
	tstorage.PieceImpl
	done func()
}

func (p reportingPiece) MarkComplete() error {
	defer p.done()
	return p.PieceImpl.MarkComplete()
}

func (s firstPieceStorage) wrap(index, pieces int, piece tstorage.PieceImpl) tstorage.PieceImpl {
	switch index {
	case 0:
		return reportingPiece{PieceImpl: piece, done: s.firstDone}
	case pieces - 1:
		return gatedPiece{PieceImpl: piece, gate: s.gate}
	default:
		return piece
	}
}

func (s firstPieceStorage) OpenTorrent(ctx context.Context, info *metainfo.Info, infoHash metainfo.Hash) (tstorage.TorrentImpl, error) {
	ti, err := s.ClientImplCloser.OpenTorrent(ctx, info, infoHash)
	if err != nil {
		return ti, err
	}
	pieces := info.NumPieces()
	if piece := ti.Piece; piece != nil {
		ti.Piece = func(p metainfo.Piece) tstorage.PieceImpl { return s.wrap(p.Index(), pieces, piece(p)) }
	}
	if piece := ti.PieceWithHash; piece != nil {
		ti.PieceWithHash = func(p metainfo.Piece, hash g.Option[[]byte]) tstorage.PieceImpl {
			return s.wrap(p.Index(), pieces, piece(p, hash))
		}
	}
	return ti, nil
}

func TestCancelledTransferLeavesNoForeignMarker(t *testing.T) {
	first := make(chan struct{})
	release := make(chan struct{})
	var firstOnce, releaseOnce sync.Once

	orig := receiveStorage
	t.Cleanup(func() { receiveStorage = orig })
	receiveStorage = func(dest string) tstorage.ClientImplCloser {
		return firstPieceStorage{
			ClientImplCloser: flatStorage(dest, false),
			firstDone:        func() { firstOnce.Do(func() { close(first) }) },
			gate:             func() { <-release },
		}
	}

	addPayload := func(root string) {
		if err := os.WriteFile(filepath.Join(root, "zz.bin"), make([]byte, 4<<20), 0o644); err != nil {
			t.Fatalf("write payload: %v", err)
		}
	}
	p := newMarkerPair(t, 46017, addPayload, shareLikeOldLauncher)
	t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })

	tr, done := p.receive(t)
	select {
	case <-first:
	case <-time.After(30 * time.Second):
		t.Fatal("timed out waiting for the marker's piece")
	}
	staging := filepath.Join(p.svcB.gamesPath(), receivingPrefix+tr.ID)
	marker := filepath.Join(staging, library.MarkerName)
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("marker was not promoted before the cancel: %v", err)
	}

	if err := p.svcB.Cancel(tr.ID); err != nil {
		t.Fatalf("Cancel: %v", err)
	}

	final := waitTransfer(t, done)
	if final.Status != TransferCancelled {
		t.Fatalf("transfer status = %s, error = %s", final.Status, final.Error)
	}
	if _, err := os.Stat(marker); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("foreign marker stayed after the cancel: %v", err)
	}
	if _, err := os.Stat(p.dest); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("cancelled transfer reached the visible folder: %v", err)
	}
}

func TestFinishedTransferLeavesNoReceivingFolder(t *testing.T) {
	tests := []struct {
		name     string
		port     int
		occupied bool
	}{
		{name: "free name", port: 46041},
		{name: "taken name keeps the other folder", port: 46043, occupied: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := newMarkerPair(t, tc.port, nil, shareLikeOldLauncher)
			existing := filepath.Join(p.dest, "saves.dat")
			if tc.occupied {
				if err := os.MkdirAll(p.dest, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(existing, []byte("mine"), 0o644); err != nil {
					t.Fatal(err)
				}
			}

			_, done := p.receive(t)
			final := waitTransfer(t, done)

			if final.Status != TransferCompleted {
				t.Fatalf("transfer status = %s, error = %s", final.Status, final.Error)
			}
			game, err := p.libB.Find(final.GameID)
			if err != nil {
				t.Fatalf("find game: %v", err)
			}
			if !platform.Inside(game.InstallDir, game.Executable) {
				t.Fatalf("executable %s outside %s", game.Executable, game.InstallDir)
			}
			if strings.Contains(game.InstallDir, receivingPrefix) {
				t.Fatalf("game registered in the receiving folder: %s", game.InstallDir)
			}
			if tc.occupied {
				if platform.SamePath(game.InstallDir, p.dest) {
					t.Fatalf("game took an occupied folder: %s", game.InstallDir)
				}
				got, err := os.ReadFile(existing)
				if err != nil || string(got) != "mine" {
					t.Fatalf("existing folder changed: %q, %v", got, err)
				}
			}
			entries, err := os.ReadDir(p.svcB.gamesPath())
			if err != nil {
				t.Fatal(err)
			}
			for _, entry := range entries {
				if strings.HasPrefix(entry.Name(), receivingPrefix) {
					t.Fatalf("receiving folder left behind: %s", entry.Name())
				}
			}
		})
	}
}

func TestRenameFailureKeepsReceivedGame(t *testing.T) {
	orig := renameReceived
	t.Cleanup(func() { renameReceived = orig })
	renameReceived = func(string, string) error { return errors.New("sharing violation") }

	p := newMarkerPair(t, 46045, nil, shareWhole)
	tr, done := p.receive(t)
	final := waitTransfer(t, done)

	if final.Status != TransferFailed {
		t.Fatalf("transfer status = %s, want failed", final.Status)
	}
	for _, want := range []string{"typhon:lan.finish_files_failed", "sharing violation"} {
		if !strings.Contains(final.Error, want) {
			t.Fatalf("transfer error = %q, want it to contain %q", final.Error, want)
		}
	}
	if _, err := os.Stat(filepath.Join(p.svcB.gamesPath(), receivingPrefix+tr.ID, "game.exe")); err != nil {
		t.Fatalf("received game was lost: %v", err)
	}
	if _, err := os.Stat(p.dest); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("reserved folder stayed: %v", err)
	}
	if games := p.libB.GetGames(); len(games) != 0 {
		t.Fatalf("library took a game that never left the receiving folder: %+v", games)
	}
}

func TestSenderTitleCannotHideReceivedFolder(t *testing.T) {
	tests := []struct {
		name, title, gameID, want string
	}{
		{name: "receiving prefix", title: ".typhon-receiving-x", gameID: "g1", want: "typhon-receiving-x"},
		{name: "plain dot name", title: ".hidden", gameID: "g1", want: "hidden"},
		{name: "dots and spaces", title: " . .hidden", gameID: "g1", want: "hidden"},
		{name: "nothing left", title: "...", gameID: "g1", want: "lan-g1"},
		{name: "dot inside stays", title: "Half.Life", gameID: "g1", want: "Half.Life"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			staging := stagingDir(t)
			run := &runState{}

			dest, err := placeReceived(run, staging, Offer{Title: tc.title, GameID: tc.gameID})
			if err != nil {
				t.Fatalf("placeReceived: %v", err)
			}

			if got := filepath.Base(dest); got != tc.want {
				t.Fatalf("folder = %q, want %q", got, tc.want)
			}
			sweepReceiving(context.Background(), filepath.Dir(dest))
			if _, err := os.Stat(dest); err != nil {
				t.Fatalf("sweep took the received game: %v", err)
			}
		})
	}
}

func TestSweepReceiving(t *testing.T) {
	root := t.TempDir()
	leftover := filepath.Join(root, receivingPrefix+"dead")
	nested := filepath.Join(root, "Game", receivingPrefix+"inner")
	game := filepath.Join(root, "Game")
	lookalike := filepath.Join(root, "typhon-receiving-game")
	stray := filepath.Join(root, receivingPrefix+"file")
	for _, dir := range []string{leftover, nested, lookalike} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	readOnly := filepath.Join(leftover, "data.bin")
	if err := os.WriteFile(readOnly, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(readOnly, 0o400); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stray, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	sweepReceiving(context.Background(), root)

	if _, err := os.Stat(leftover); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("leftover stayed: %v", err)
	}
	for _, path := range []string{game, nested, lookalike, stray} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("%s must survive: %v", path, err)
		}
	}
}

func TestSweepReceivingLeavesEverythingOnCancelledContext(t *testing.T) {
	root := t.TempDir()
	leftover := filepath.Join(root, receivingPrefix+"dead")
	if err := os.Mkdir(leftover, 0o755); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	sweepReceiving(ctx, root)

	if _, err := os.Stat(leftover); err != nil {
		t.Fatalf("cancelled sweep still removed: %v", err)
	}
}

func TestSweepReceivingHandlesMissingOrEmptyRoot(t *testing.T) {
	sweepReceiving(context.Background(), "")
	sweepReceiving(context.Background(), filepath.Join(t.TempDir(), "gone"))
}

func TestSweepReceivingDoesNotFollowSymlink(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	keep := filepath.Join(outside, "keep.txt")
	if err := os.WriteFile(keep, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, receivingPrefix+"link")
	if runtime.GOOS == "windows" {
		//nolint:gosec // G204: аргументы — пути из t.TempDir(); внешнего ввода, который требует валидации по инварианту 32, здесь нет
		if out, err := exec.Command("cmd", "/c", "mklink", "/J", link, outside).CombinedOutput(); err != nil {
			t.Skipf("junction not available: %v (%s)", err, out)
		}
	} else if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink not supported on this system: %v", err)
	}

	sweepReceiving(context.Background(), root)

	if _, err := os.Stat(keep); err != nil {
		t.Fatalf("sweep followed the link: %v", err)
	}
	if _, err := os.Lstat(link); err != nil {
		t.Fatalf("link must be refused, not removed: %v", err)
	}
}

func TestRemoveReceivingRefusesForeignPaths(t *testing.T) {
	plain := t.TempDir()
	if err := removeReceiving(plain); err == nil {
		t.Fatal("removeReceiving accepted a folder without the prefix")
	}
	if _, err := os.Stat(plain); err != nil {
		t.Fatalf("folder was removed: %v", err)
	}
	file := filepath.Join(t.TempDir(), receivingPrefix+"file")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := removeReceiving(file); err == nil {
		t.Fatal("removeReceiving accepted a file")
	}
	if err := removeReceiving(filepath.Join(t.TempDir(), receivingPrefix+"gone")); err != nil {
		t.Fatalf("missing folder: %v", err)
	}
}

func TestEnableSweepsLeftoversAndScanCannotSeeThem(t *testing.T) {
	lib, err := library.NewServiceAt(filepath.Join(t.TempDir(), "library.json"))
	if err != nil {
		t.Fatalf("new library: %v", err)
	}
	t.Cleanup(func() {
		if err := lib.ServiceShutdown(); err != nil {
			t.Errorf("library shutdown: %v", err)
		}
	})
	svc := newTestService(t, lib, newTestSettings(t, t.TempDir()))
	gamesPath := svc.gamesPath()
	transport, _ := loopbackPair(t)
	svc.newTransport = transport
	svc.torrentPort = 46047

	leftover := filepath.Join(gamesPath, receivingPrefix+"crashed")
	if err := os.MkdirAll(leftover, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := library.WriteMarker(leftover, library.Marker{GameID: "sender", Title: "Foreign", Uninstall: library.Uninstall{}}); err != nil {
		t.Fatal(err)
	}

	if err := svc.enable(context.Background()); err != nil {
		t.Fatalf("enable: %v", err)
	}
	t.Cleanup(func() {
		if err := svc.ServiceShutdown(); err != nil {
			t.Errorf("shutdown: %v", err)
		}
	})

	if _, err := os.Stat(leftover); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("crash leftover survived start: %v", err)
	}
}

func TestReceiveRefusesGameSharedHere(t *testing.T) {
	p := newMarkerPair(t, 46019, nil, shareWhole)
	data, err := p.svcA.store.readTorrent(p.share.InfoHash)
	if err != nil {
		t.Fatalf("read torrent: %v", err)
	}
	if err := p.svcB.store.writeTorrent(p.share.InfoHash, data); err != nil {
		t.Fatalf("write torrent: %v", err)
	}
	if err := p.svcB.persistShare(p.share); err != nil {
		t.Fatalf("persist share: %v", err)
	}
	run := liveRun(p.svcB)
	if err := p.svcB.startSeeding(run, p.share); err != nil {
		t.Fatalf("start seeding: %v", err)
	}
	run.mu.Lock()
	seed := run.seeded[p.share.GameID]
	run.mu.Unlock()
	before := p.svcB.StatsOf().AnnouncesSent

	_, err = p.svcB.Receive(p.offer.InfoHash, p.offer.PeerID)

	if !errors.Is(err, errAlreadyHave) {
		t.Fatalf("Receive error = %v, want errAlreadyHave", err)
	}
	select {
	case <-seed.Closed():
		t.Fatal("Receive dropped the live seed")
	default:
	}
	run.mu.Lock()
	transfers := len(run.transfers)
	run.mu.Unlock()
	if transfers != 0 {
		t.Fatalf("refused Receive left %d transfers", transfers)
	}
	if _, err := os.Stat(p.dest); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("refused Receive touched the games folder: %v", err)
	}
	p.svcB.sendAnnounces(run)
	if after := p.svcB.StatsOf().AnnouncesSent; after <= before {
		t.Fatalf("announces sent = %d after %d: the seed is no longer announced", after, before)
	}
}

func TestConcurrentReceiveTakesOneTransfer(t *testing.T) {
	openGate := holdTransfers(t)
	p := newMarkerPair(t, 46021, nil, shareWhole)
	t.Cleanup(openGate)

	const callers = 8
	errs := make([]error, callers)
	var wg sync.WaitGroup
	for i := range callers {
		wg.Go(func() {
			_, errs[i] = p.svcB.Receive(p.offer.InfoHash, p.offer.PeerID)
		})
	}
	wg.Wait()

	accepted := 0
	for _, err := range errs {
		switch {
		case err == nil:
			accepted++
		case !errors.Is(err, errAlreadyHave):
			t.Fatalf("Receive error = %v, want errAlreadyHave", err)
		}
	}
	if accepted != 1 {
		t.Fatalf("accepted %d transfers of one game", accepted)
	}
	entries, err := os.ReadDir(p.svcB.gamesPath())
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("games folder holds %d folders, want 1: %v", len(entries), entries)
	}
}

// holdTransfers keeps every received piece at the gate, so a transfer stays
// running until the test's cleanup opens it.
func holdTransfers(t *testing.T) (openGate func()) {
	t.Helper()
	release := make(chan struct{})
	var once sync.Once
	orig := receiveStorage
	t.Cleanup(func() { receiveStorage = orig })
	receiveStorage = func(dest string) tstorage.ClientImplCloser {
		return gatedStorage{ClientImplCloser: flatStorage(dest, false), gate: func() { <-release }}
	}
	return func() { once.Do(func() { close(release) }) }
}

func TestReceiveIsOpenAgainAfterTransferEnds(t *testing.T) {
	missingExe := func(t *testing.T, svc *Service, game library.Game, root string) Share {
		t.Helper()
		return publishOldTorrent(t, svc, game, root, "missing.exe")
	}
	tests := []struct {
		name    string
		port    int
		publish func(*testing.T, *Service, library.Game, string) Share
		hold    bool
		want    TransferStatus
	}{
		{name: "completed", port: 46023, publish: shareWhole, want: TransferCompleted},
		{name: "failed", port: 46025, publish: missingExe, want: TransferFailed},
		{name: "cancelled", port: 46027, publish: shareWhole, hold: true, want: TransferCancelled},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			openGate := func() {}
			if tc.hold {
				openGate = holdTransfers(t)
			}
			p := newMarkerPair(t, tc.port, nil, tc.publish)
			t.Cleanup(openGate)

			tr, done := p.receive(t)
			if tc.hold {
				if err := p.svcB.Cancel(tr.ID); err != nil {
					t.Fatalf("Cancel: %v", err)
				}
			}
			if final := waitTransfer(t, done); final.Status != tc.want {
				t.Fatalf("transfer status = %s, want %s, error = %s", final.Status, tc.want, final.Error)
			}

			if _, err := p.svcB.Receive(p.offer.InfoHash, p.offer.PeerID); err != nil {
				t.Fatalf("second Receive after %s: %v", tc.want, err)
			}
		})
	}
}

// The engine dials a peer once and forgets it on failure, so a sender that was
// briefly unreachable used to leave the transfer waiting forever.
func TestTransferRedialsSenderAfterFailedConnection(t *testing.T) {
	p := newMarkerPair(t, 46049, nil, shareWhole)
	p.svcA.disable()

	_, done := p.receive(t)

	p.svcA.torrentPort = p.offer.Port
	if err := p.svcA.enable(context.Background()); err != nil {
		t.Fatalf("enable A: %v", err)
	}
	if final := waitTransfer(t, done); final.Status != TransferCompleted {
		t.Fatalf("transfer status = %s, error = %s", final.Status, final.Error)
	}
}

func TestReceiveFreesSlotWhenTransferCannotOpen(t *testing.T) {
	p := newMarkerPair(t, 46029, nil, shareWhole)
	gamesPath := p.svcB.gamesPath()
	if err := os.MkdirAll(filepath.Dir(gamesPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(gamesPath, []byte("not a folder"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := p.svcB.Receive(p.offer.InfoHash, p.offer.PeerID); err == nil {
		t.Fatal("Receive succeeded with a file in place of the games folder")
	} else if errors.Is(err, errAlreadyHave) {
		t.Fatalf("Receive error = %v, want the real cause", err)
	}

	run := liveRun(p.svcB)
	run.mu.Lock()
	transfers := len(run.transfers)
	run.mu.Unlock()
	if transfers != 0 {
		t.Fatalf("failed Receive left %d transfers", transfers)
	}
	if err := os.Remove(gamesPath); err != nil {
		t.Fatal(err)
	}
	if _, err := p.svcB.Receive(p.offer.InfoHash, p.offer.PeerID); err != nil {
		t.Fatalf("second Receive: %v", err)
	}
}

func TestReceiveRefusedWhileShuttingDown(t *testing.T) {
	p := newMarkerPair(t, 46031, nil, shareWhole)
	liveRun(p.svcB).cancel()

	_, err := p.svcB.Receive(p.offer.InfoHash, p.offer.PeerID)

	if !errors.Is(err, errSharingDisabled) {
		t.Fatalf("Receive error = %v, want errSharingDisabled", err)
	}
	if _, err := os.Stat(p.dest); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Receive touched the games folder while shutting down: %v", err)
	}
}

func TestReceiveAndDisableDoNotRace(t *testing.T) {
	openGate := holdTransfers(t)
	p := newMarkerPair(t, 46033, nil, shareWhole)
	t.Cleanup(openGate)

	var wg sync.WaitGroup
	var receiveErr error
	wg.Go(func() {
		_, receiveErr = p.svcB.Receive(p.offer.InfoHash, p.offer.PeerID)
	})
	wg.Go(p.svcB.disable)
	wg.Wait()

	if receiveErr != nil && !errors.Is(receiveErr, errSharingDisabled) {
		t.Fatalf("Receive error = %v, want nil or errSharingDisabled", receiveErr)
	}
}

func TestReceiveAndSeedingShareOneHashOnce(t *testing.T) {
	openGate := holdTransfers(t)
	p := newMarkerPair(t, 46035, nil, shareWhole)
	t.Cleanup(openGate)
	data, err := p.svcA.store.readTorrent(p.share.InfoHash)
	if err != nil {
		t.Fatalf("read torrent: %v", err)
	}
	if err := p.svcB.store.writeTorrent(p.share.InfoHash, data); err != nil {
		t.Fatalf("write torrent: %v", err)
	}
	run := liveRun(p.svcB)

	start := make(chan struct{})
	var receiveErr, seedErr error
	var wg sync.WaitGroup
	wg.Go(func() {
		<-start
		_, receiveErr = p.svcB.Receive(p.offer.InfoHash, p.offer.PeerID)
	})
	wg.Go(func() {
		<-start
		seedErr = p.svcB.startSeeding(run, p.share)
	})
	close(start)
	wg.Wait()

	if (receiveErr == nil) == (seedErr == nil) {
		t.Fatalf("both or neither took the hash: receive = %v, seed = %v", receiveErr, seedErr)
	}
	for _, err := range []error{receiveErr, seedErr} {
		if err != nil && !errors.Is(err, errAlreadyHave) {
			t.Fatalf("loser error = %v, want errAlreadyHave", err)
		}
	}
	run.mu.Lock()
	defer run.mu.Unlock()
	if seedErr == nil {
		select {
		case <-run.seeded[p.share.GameID].Closed():
			t.Fatal("the winning seed was dropped")
		default:
		}
		if len(run.transfers) != 0 {
			t.Fatalf("losing Receive left %d transfers", len(run.transfers))
		}
		return
	}
	if len(run.seeded) != 0 {
		t.Fatalf("losing seed stayed: %d", len(run.seeded))
	}
}

func TestShareFailsWhileSameGameIsReceived(t *testing.T) {
	tests := []struct {
		name        string
		port        int
		sharedFirst bool
	}{
		{name: "no earlier row", port: 46037},
		{name: "earlier row stays off", port: 46039, sharedFirst: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			openGate := holdTransfers(t)
			p := newMarkerPair(t, tc.port, nil, shareWhole)
			t.Cleanup(openGate)
			gameB, err := p.libB.RegisterInstalled(t.Context(), library.InstalledGame{
				Title:      p.gameA.Title,
				Executable: p.gameA.Executable,
				InstallDir: p.rootA,
			})
			if err != nil {
				t.Fatalf("register on B: %v", err)
			}
			if tc.sharedFirst {
				if _, err := p.svcB.Share(gameB.ID); err != nil {
					t.Fatalf("first Share: %v", err)
				}
				if err := p.svcB.Unshare(gameB.ID); err != nil {
					t.Fatalf("Unshare: %v", err)
				}
			}
			p.receive(t)

			_, err = p.svcB.Share(gameB.ID)

			if !errors.Is(err, errAlreadyHave) {
				t.Fatalf("Share error = %v, want errAlreadyHave", err)
			}
			shares := p.svcB.Shares()
			if !tc.sharedFirst {
				if len(shares) != 0 {
					t.Fatalf("refused Share left rows: %+v", shares)
				}
				return
			}
			if len(shares) != 1 || shares[0].Enabled {
				t.Fatalf("refused Share changed the row: %+v", shares)
			}
		})
	}
}

func TestShareStoreRemoveKeepsRowWhenSaveFails(t *testing.T) {
	dir := t.TempDir()
	store, err := newShareStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.load(); err != nil {
		t.Fatal(err)
	}
	if err := store.put(Share{GameID: "g1", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if err := store.remove("g1"); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if len(store.list()) != 0 {
		t.Fatalf("row stayed: %+v", store.list())
	}
	if err := store.remove("absent"); err != nil {
		t.Fatalf("remove of an absent row: %v", err)
	}

	if err := store.put(Share{GameID: "g1", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	store.path = filepath.Join(dir, "blocker.json", "shares.json")
	if err := os.WriteFile(filepath.Join(dir, "blocker.json"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := store.remove("g1"); err == nil {
		t.Fatal("remove reported success on a failed save")
	}
	if len(store.list()) != 1 {
		t.Fatalf("row lost after a failed save: %+v", store.list())
	}
}

func TestMakeWritable(t *testing.T) {
	write := func(t *testing.T, path string, mode os.FileMode) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(path, mode); err != nil {
			t.Fatal(err)
		}
	}
	writable := func(t *testing.T, path string) bool {
		t.Helper()
		info, err := os.Lstat(path)
		if err != nil {
			t.Fatal(err)
		}
		return info.Mode().Perm()&0o200 != 0
	}

	t.Run("read-only files at any depth, keeping mtime", func(t *testing.T) {
		root := t.TempDir()
		top, deep := filepath.Join(root, "game.exe"), filepath.Join(root, "data", "nested", "x.bin")
		write(t, top, 0o400)
		write(t, deep, 0o400)
		mtime := time.Unix(1_700_000_000, 0)
		if err := os.Chtimes(deep, mtime, mtime); err != nil {
			t.Fatal(err)
		}

		if err := makeWritable(context.Background(), root); err != nil {
			t.Fatalf("makeWritable: %v", err)
		}

		if !writable(t, top) || !writable(t, deep) {
			t.Fatal("file stayed read-only")
		}
		info, err := os.Stat(deep)
		if err != nil {
			t.Fatal(err)
		}
		if !info.ModTime().Equal(mtime) {
			t.Fatalf("mtime = %v, want %v: the share fingerprint would change", info.ModTime(), mtime)
		}
	})

	t.Run("cancelled ctx leaves files alone", func(t *testing.T) {
		root := t.TempDir()
		file := filepath.Join(root, "game.exe")
		write(t, file, 0o400)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err := makeWritable(ctx, root)

		if !errors.Is(err, context.Canceled) {
			t.Fatalf("makeWritable error = %v, want context.Canceled", err)
		}
		if writable(t, file) {
			t.Fatal("cancelled walk still changed a file")
		}
	})

	t.Run("missing root", func(t *testing.T) {
		err := makeWritable(context.Background(), filepath.Join(t.TempDir(), "gone"))
		if !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("makeWritable error = %v, want fs.ErrNotExist", err)
		}
	})

	t.Run("symlink target outside the root stays read-only", func(t *testing.T) {
		outside := filepath.Join(t.TempDir(), "outside.txt")
		write(t, outside, 0o400)
		root := t.TempDir()
		if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
			t.Skipf("symlink not supported on this system: %v", err)
		}

		if err := makeWritable(context.Background(), root); err != nil {
			t.Fatalf("makeWritable: %v", err)
		}

		if writable(t, outside) {
			t.Fatal("walk followed a symlink out of the root")
		}
	})
}

func TestReceiveFailsWhenForeignMarkerCannotBeRemoved(t *testing.T) {
	blockMarker := func(root string) {
		marker := filepath.Join(root, library.MarkerName)
		if err := os.Remove(marker); err != nil {
			t.Fatalf("remove sender's marker: %v", err)
		}
		if err := os.Mkdir(marker, 0o755); err != nil {
			t.Fatalf("create blocker: %v", err)
		}
		if err := os.WriteFile(filepath.Join(marker, "inside"), []byte("x"), 0o644); err != nil {
			t.Fatalf("fill blocker: %v", err)
		}
	}

	got := runMarkerTransfer(t, 46011, blockMarker, shareLikeOldLauncher)

	if got.final.Status != TransferFailed {
		t.Fatalf("transfer status = %s, want failed", got.final.Status)
	}
	for _, want := range []string{"typhon:lan.finish_files_failed", "remove foreign install marker"} {
		if !strings.Contains(got.final.Error, want) {
			t.Fatalf("transfer error = %q, want it to contain %q", got.final.Error, want)
		}
	}
	if games := got.lib.GetGames(); len(games) != 0 {
		t.Fatalf("library took a game that still holds a foreign marker: %+v", games)
	}
}

func TestRemoveForeignMarker(t *testing.T) {
	tests := []struct {
		name    string
		prepare func(t *testing.T, marker string)
		wantErr bool
	}{
		{name: "no marker", prepare: func(*testing.T, string) {}},
		{name: "marker in another case", prepare: func(t *testing.T, marker string) {
			upper := filepath.Join(filepath.Dir(marker), strings.ToUpper(library.MarkerName))
			if err := os.WriteFile(upper, []byte("{}"), 0o400); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "writable marker", prepare: func(t *testing.T, marker string) {
			if err := os.WriteFile(marker, []byte("{}"), 0o644); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "read-only marker", prepare: func(t *testing.T, marker string) {
			if err := os.WriteFile(marker, []byte("{}"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(marker, 0o400); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "directory in its place", wantErr: true, prepare: func(t *testing.T, marker string) {
			if err := os.Mkdir(marker, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(marker, "inside"), []byte("x"), 0o644); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			marker := filepath.Join(dir, library.MarkerName)
			nested := filepath.Join(dir, "data", library.MarkerName)
			if err := os.MkdirAll(filepath.Dir(nested), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(nested, []byte("game file"), 0o644); err != nil {
				t.Fatal(err)
			}
			tc.prepare(t, marker)

			err := removeForeignMarker(dir)

			if tc.wantErr {
				if err == nil {
					t.Fatal("removeForeignMarker returned nil for a marker that stayed")
				}
				return
			}
			if err != nil {
				t.Fatalf("removeForeignMarker: %v", err)
			}
			if _, err := os.Stat(marker); !errors.Is(err, fs.ErrNotExist) {
				t.Fatalf("marker still there: %v", err)
			}
			entries, err := os.ReadDir(dir)
			if err != nil {
				t.Fatal(err)
			}
			for _, entry := range entries {
				if strings.EqualFold(entry.Name(), library.MarkerName) {
					t.Fatalf("marker still there as %s", entry.Name())
				}
			}
			if _, err := os.Stat(nested); err != nil {
				t.Fatalf("nested file of the same name was touched: %v", err)
			}
		})
	}
}

func TestRemoveForeignMarkerRejectsEmptyDir(t *testing.T) {
	if err := removeForeignMarker(""); err == nil {
		t.Fatal("removeForeignMarker accepted an empty directory")
	}
}

func TestShareReplacesCachedTorrentThatCarriesMarker(t *testing.T) {
	lib, game, root := newTestLibraryWithGame(t)
	svc := newTestService(t, lib, nil)
	if err := svc.enable(context.Background()); err != nil {
		t.Fatalf("enable: %v", err)
	}
	t.Cleanup(func() {
		if err := svc.ServiceShutdown(); err != nil {
			t.Errorf("shutdown: %v", err)
		}
	})
	old := shareLikeOldLauncher(t, svc, game, root)
	if err := svc.Unshare(game.ID); err != nil {
		t.Fatalf("Unshare: %v", err)
	}

	rebuilt, err := svc.Share(game.ID)
	if err != nil {
		t.Fatalf("Share: %v", err)
	}

	if rebuilt.InfoHash == old.InfoHash {
		t.Fatal("Share reused the torrent that carries the marker")
	}
	data, err := svc.store.readTorrent(rebuilt.InfoHash)
	if err != nil {
		t.Fatalf("read torrent: %v", err)
	}
	mi, err := metainfo.Load(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("parse torrent: %v", err)
	}
	info, err := mi.UnmarshalInfo()
	if err != nil {
		t.Fatalf("parse info: %v", err)
	}
	if torrentHasPathPrefix(info, library.MarkerName) {
		t.Fatalf("rebuilt torrent still carries the marker: %+v", info.Files)
	}
}
