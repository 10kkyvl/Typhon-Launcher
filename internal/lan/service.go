package lan

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"typhon/internal/history"
	"typhon/internal/library"
	"typhon/internal/platform"
	"typhon/internal/settings"
	"typhon/internal/storage"
	"typhon/internal/uierr"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/wailsapp/wails/v3/pkg/application"
)

const transferSampleInterval = 500 * time.Millisecond

var (
	errEmptyGameID          = uierr.New("lan.empty_game_id", "lan: empty game id")
	errLibraryUnavailable   = uierr.New("lan.library_unavailable", "lan: library service unavailable")
	errNoInstallDir         = uierr.New("lan.no_install_dir", "lan: game has no install directory")
	errExeOutsideInstall    = uierr.New("lan.exe_outside_install", "lan: executable is outside the install directory")
	errSharingDisabled      = uierr.New("lan.sharing_disabled", "lan: sharing is disabled")
	errGamesPathUnavailable = uierr.New("lan.games_path_unavailable", "lan: games path unavailable")
	errOfferNotFound        = uierr.New("lan.offer_not_found", "lan: offer not found or expired")
	errUnknownTransfer      = uierr.New("lan.unknown_transfer", "lan: unknown transfer")
	errAlreadyHave          = uierr.New("lan.already_have", "lan: this game is already shared or being received")
)

// transferState is one in-flight or finished Receive, guarded by runState.mu.
type transferState struct {
	transfer Transfer
	t        *torrent.Torrent
	cancel   context.CancelFunc
}

// runState holds everything that exists only while LAN sharing is enabled.
// It is swapped in and out of Service.run as settings.LANSharing toggles.
type runState struct {
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	cl        *lanClient
	transport transport
	table     *peerTable
	limiter   *sourceLimiter
	stats     *statCounters
	changed   chan struct{}

	// seedMu serializes startSeeding, so a second Share of one game finds the
	// first one's seed instead of racing it to the client.
	seedMu sync.Mutex
	// placeMu keeps two finishing transfers of one title from claiming the
	// same final folder between its reservation and the rename.
	placeMu sync.Mutex

	mu        sync.Mutex
	seeded    map[string]*torrent.Torrent
	transfers map[string]*transferState
}

// hashHeldLocked tells whether a seed or a running transfer owns the infohash.
// The client hands back the existing torrent for a known infohash, so a second
// owner would share it and drop it from under the first. A seed being added
// right now is not visible here: lanClient refuses that one by itself.
func (r *runState) hashHeldLocked(infoHash string) bool {
	for _, t := range r.seeded {
		if t.InfoHash().HexString() == infoHash {
			return true
		}
	}
	for _, ts := range r.transfers {
		if ts.transfer.InfoHash == infoHash && ts.transfer.Status == TransferReceiving {
			return true
		}
	}
	return false
}

func (r *runState) signalChanged() {
	select {
	case r.changed <- struct{}{}:
	default:
	}
}

// Service is the LAN game-sharing service. It is only ever networked while
// settings.Settings.LANSharing is true; it otherwise sits idle so that
// constructing and binding it costs nothing for the vast majority of users
// who never turn the feature on.
type Service struct {
	mu sync.Mutex
	// enableMu covers a whole enable or disable: the startup sweep must not
	// run while a stopping instance is still finishing its transfers.
	enableMu sync.Mutex

	settingsSvc *settings.Service
	lib         *library.Service

	dir   string
	store *shareStore

	selfID   string
	hostname string

	recorder func(history.Record) error
	hook     func(name string, data any)

	newTransport func() (transport, error)
	// torrentPort overrides listenPort; zero means use the default. Tests
	// set this to avoid two Services in one process racing for the same
	// fixed port and falling through to the OS-assigned-port retry path.
	torrentPort int

	rootCtx     context.Context
	unsubscribe func()
	run         *runState

	// Atomic and limitsMu instead of mu: SetGamesRunning runs under the
	// library mutex and must not wait on mu, which is held while the client starts.
	playing    atomic.Bool
	liveClient atomic.Pointer[lanClient]
	limitsMu   sync.Mutex
}

//wails:ignore
func (s *Service) SetGamesRunning(running bool) {
	s.playing.Store(running)
	s.applyLimits(s.currentSettings())
}

func (s *Service) applyLimits(cfg settings.Settings) {
	s.limitsMu.Lock()
	defer s.limitsMu.Unlock()
	if cl := s.liveClient.Load(); cl != nil {
		cl.applyLimits(cfg.PlayRates(s.playing.Load()))
	}
}

func NewService(settingsSvc *settings.Service, lib *library.Service) (*Service, error) {
	dir, err := settings.ConfigDir()
	if err != nil {
		return nil, fmt.Errorf("resolve config dir: %w", err)
	}
	return NewServiceAt(dir, settingsSvc, lib)
}

// NewServiceAt is NewService with an explicit config directory, so tests can
// point it at a t.TempDir() without a real settings.ConfigDir().
func NewServiceAt(dir string, settingsSvc *settings.Service, lib *library.Service) (*Service, error) {
	store, err := newShareStore(dir)
	if err != nil {
		return nil, err
	}
	if err := store.load(); err != nil {
		return nil, err
	}
	selfID, err := randomID()
	if err != nil {
		return nil, fmt.Errorf("generate lan peer id: %w", err)
	}
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "typhon"
	}
	s := &Service{
		settingsSvc: settingsSvc,
		lib:         lib,
		dir:         dir,
		store:       store,
		selfID:      selfID,
		hostname:    sanitizeText(hostname, 63),
	}
	s.newTransport = func() (transport, error) {
		return newMulticast(multicastGroup, netInterfaces())
	}
	return s, nil
}

func randomID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("read random bytes: %w", err)
	}
	return hex.EncodeToString(b), nil
}

func netInterfaces() []net.Interface {
	ifaces, err := net.Interfaces()
	if err != nil {
		slog.Warn("list network interfaces", "error", err)
		return nil
	}
	return ifaces
}

func (s *Service) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	s.mu.Lock()
	s.rootCtx = ctx
	s.mu.Unlock()

	if s.settingsSvc != nil {
		s.unsubscribe = s.settingsSvc.Subscribe(s.onSettingsChanged)
	}
	if s.settingsEnabled() {
		if err := s.enable(ctx); err != nil {
			slog.Error("start lan sharing", "error", err)
		}
	}
	return nil
}

func (s *Service) ServiceShutdown() error {
	s.mu.Lock()
	unsubscribe := s.unsubscribe
	s.mu.Unlock()
	if unsubscribe != nil {
		unsubscribe()
	}
	s.disable()
	return nil
}

func (s *Service) settingsEnabled() bool {
	if s.settingsSvc == nil {
		return false
	}
	return s.settingsSvc.GetSettings().LANSharing
}

func (s *Service) currentSettings() settings.Settings {
	if s.settingsSvc == nil {
		return settings.Defaults()
	}
	return s.settingsSvc.GetSettings()
}

func (s *Service) gamesPath() string {
	return s.currentSettings().GamesPath
}

func (s *Service) onSettingsChanged(next settings.Settings) {
	s.mu.Lock()
	run := s.run
	root := s.rootCtx
	s.mu.Unlock()

	s.applyLimits(next)

	running := run != nil
	if next.LANSharing == running {
		return
	}
	if next.LANSharing {
		if root == nil {
			return
		}
		if err := s.enable(root); err != nil {
			slog.Error("enable lan sharing", "error", err)
		}
		return
	}
	s.disable()
}

// enable starts the LAN client and its background goroutines. It is a
// no-op if already running, and safe to call directly from tests that skip
// ServiceStartup's settings wiring.
func (s *Service) enable(parent context.Context) error {
	s.enableMu.Lock()
	defer s.enableMu.Unlock()

	s.mu.Lock()
	running := s.run != nil
	s.mu.Unlock()
	if running {
		return nil
	}
	// Before s.run is set, so no Receive of this process can own a folder yet.
	sweepReceiving(parent, s.gamesPath())

	s.mu.Lock()
	if s.run != nil {
		s.mu.Unlock()
		return nil
	}
	cfg := s.currentSettings()
	dataDir := filepath.Join(s.dir, "lan", "data")
	port := listenPort
	if s.torrentPort != 0 {
		port = s.torrentPort
	}
	cfg.DownloadRateLimit, cfg.UploadRateLimit = cfg.PlayRates(s.playing.Load())
	cl, err := newLANClient(cfg, dataDir, port)
	if err != nil {
		s.mu.Unlock()
		return err
	}
	tp, err := s.newTransport()
	if err != nil {
		cl.close()
		s.mu.Unlock()
		return err
	}
	runCtx, cancel := context.WithCancel(parent)
	run := &runState{
		ctx:       runCtx,
		cancel:    cancel,
		cl:        cl,
		transport: tp,
		table:     newPeerTable(),
		limiter:   newSourceLimiter(),
		stats:     newStatCounters(),
		changed:   make(chan struct{}, 1),
		seeded:    map[string]*torrent.Torrent{},
		transfers: map[string]*transferState{},
	}
	s.run = run
	s.liveClient.Store(cl)
	shares := s.store.list()
	s.mu.Unlock()
	s.applyLimits(s.currentSettings())

	for _, sh := range shares {
		if !sh.Enabled {
			continue
		}
		if err := s.startSeeding(run, sh); err != nil {
			slog.Error("resume lan seed", "gameId", sh.GameID, "error", err)
		}
	}

	run.wg.Add(2)
	go s.announceLoop(run)
	go s.receiveLoop(run)
	return nil
}

func (s *Service) disable() {
	s.enableMu.Lock()
	defer s.enableMu.Unlock()

	s.mu.Lock()
	run := s.run
	s.run = nil
	s.liveClient.Store(nil)
	s.mu.Unlock()
	if run == nil {
		return
	}
	run.cancel()
	run.wg.Wait()
	if err := run.transport.Close(); err != nil {
		slog.Warn("close lan transport", "error", err)
	}
	run.cl.close()
}

// setHook installs a test-only observer for every emit call, guarded by the
// same mutex emit reads it under so tests can swap hooks mid-run under -race.
func (s *Service) setHook(fn func(name string, data any)) {
	s.mu.Lock()
	s.hook = fn
	s.mu.Unlock()
}

func (s *Service) emit(name string, data any) {
	if app := application.Get(); app != nil {
		app.Event.Emit(name, data)
	}
	s.mu.Lock()
	hook := s.hook
	s.mu.Unlock()
	if hook != nil {
		hook(name, data)
	}
}

func (s *Service) emitHashProgress(gameID string, p Progress, done bool, errMsg string) {
	s.emit("lan:hashing", HashProgress{
		GameID:         gameID,
		ProcessedBytes: p.ProcessedBytes,
		TotalBytes:     p.TotalBytes,
		CurrentFile:    p.CurrentFile,
		Done:           done,
		Error:          errMsg,
	})
}

// SetHistoryRecorder wires the journal entry a completed Receive leaves
// behind. history.Service is owned by another package; a nil fn is a valid
// "don't record" state during startup ordering.
//
//wails:ignore
func (s *Service) SetHistoryRecorder(fn func(history.Record) error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.recorder = fn
}

func (s *Service) Shares() []Share {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.store.list()
}

func (s *Service) persistShare(sh Share) error {
	s.mu.Lock()
	err := s.store.put(sh)
	list := s.store.list()
	s.mu.Unlock()
	if err != nil {
		return err
	}
	s.emit("lan:shares", list)
	return nil
}

// undoShare puts the store back after a share was saved but could not start:
// a row marked enabled that nothing seeds would be announced as nothing and
// resumed on every start.
func (s *Service) undoShare(gameID string, previous Share, had bool, cause error) error {
	s.mu.Lock()
	var err error
	if had {
		previous.Enabled = false
		err = s.store.put(previous)
	} else {
		err = s.store.remove(gameID)
	}
	list := s.store.list()
	s.mu.Unlock()
	if err != nil {
		return errors.Join(cause, fmt.Errorf("lan: undo share: %w", err))
	}
	s.emit("lan:shares", list)
	return cause
}

// Share builds (or reuses a cached build of) the torrent for an installed
// game and starts seeding it. Hashing runs outside Service.mu, tracked in
// the running instance's WaitGroup and cancelled through its ctx, so a
// multi-minute hash of a large install neither blocks Peers()/Unshare() of
// another game nor outlives Shutdown.
func (s *Service) Share(gameID string) (Share, error) {
	gameID = strings.TrimSpace(gameID)
	if gameID == "" {
		return Share{}, errEmptyGameID
	}
	if s.lib == nil {
		return Share{}, errLibraryUnavailable
	}
	game, err := s.lib.Find(gameID)
	if err != nil {
		return Share{}, fmt.Errorf("lan: find game: %w", err)
	}
	installDir := strings.TrimSpace(game.InstallDir)
	if installDir == "" {
		return Share{}, errNoInstallDir
	}
	if !platform.Inside(installDir, game.Executable) {
		return Share{}, errExeOutsideInstall
	}
	exeRel, err := filepath.Rel(installDir, game.Executable)
	if err != nil {
		return Share{}, fmt.Errorf("lan: relative executable path: %w", err)
	}

	s.mu.Lock()
	run := s.run
	s.mu.Unlock()
	if run == nil {
		return Share{}, errSharingDisabled
	}

	run.wg.Add(1)
	defer run.wg.Done()
	ctx := run.ctx

	fp, err := fingerprint(ctx, installDir)
	if err != nil {
		return Share{}, fmt.Errorf("lan: fingerprint %s: %w", installDir, err)
	}

	s.mu.Lock()
	cached, had := s.store.shares[gameID]
	s.mu.Unlock()

	if had && cached.Fingerprint == fp && cached.Root == installDir {
		if _, err := s.store.readTorrent(cached.InfoHash); err == nil {
			previous := cached
			cached.Enabled = true
			cached.Exe = filepath.ToSlash(exeRel)
			cached.Title = game.Title
			cached.Version = game.Version
			if err := s.persistShare(cached); err != nil {
				return Share{}, err
			}
			if err := s.startSeeding(run, cached); err != nil {
				return Share{}, s.undoShare(gameID, previous, true, err)
			}
			run.signalChanged()
			return cached, nil
		}
	}

	s.emitHashProgress(gameID, Progress{}, false, "")
	info, err := BuildInfo(ctx, installDir, func(p Progress) {
		s.emitHashProgress(gameID, p, false, "")
	})
	if err != nil {
		s.emitHashProgress(gameID, Progress{}, true, err.Error())
		return Share{}, fmt.Errorf("lan: build torrent info: %w", err)
	}
	infoHashHex, torrentBytes, err := buildTorrent(*info)
	if err != nil {
		s.emitHashProgress(gameID, Progress{}, true, err.Error())
		return Share{}, err
	}
	if err := s.store.writeTorrent(infoHashHex, torrentBytes); err != nil {
		s.emitHashProgress(gameID, Progress{}, true, err.Error())
		return Share{}, err
	}

	share := Share{
		GameID:      gameID,
		Title:       game.Title,
		Version:     game.Version,
		Exe:         filepath.ToSlash(exeRel),
		Root:        installDir,
		InfoHash:    infoHashHex,
		SizeBytes:   info.TotalLength(),
		Fingerprint: fp,
		BuiltAt:     time.Now(),
		Enabled:     true,
	}
	if err := s.persistShare(share); err != nil {
		s.emitHashProgress(gameID, Progress{}, true, err.Error())
		return Share{}, err
	}
	s.emitHashProgress(gameID, Progress{ProcessedBytes: share.SizeBytes, TotalBytes: share.SizeBytes}, true, "")

	if err := s.startSeeding(run, share); err != nil {
		return Share{}, s.undoShare(gameID, cached, had, err)
	}
	run.signalChanged()
	return share, nil
}

func (s *Service) Unshare(gameID string) error {
	gameID = strings.TrimSpace(gameID)
	s.mu.Lock()
	sh, had := s.store.shares[gameID]
	if !had {
		s.mu.Unlock()
		return nil
	}
	sh.Enabled = false
	if err := s.store.put(sh); err != nil {
		s.mu.Unlock()
		return err
	}
	list := s.store.list()
	run := s.run
	s.mu.Unlock()
	s.emit("lan:shares", list)

	if run == nil {
		return nil
	}
	run.mu.Lock()
	t, exists := run.seeded[gameID]
	delete(run.seeded, gameID)
	run.mu.Unlock()
	if exists {
		t.Drop()
	}
	run.signalChanged()
	return nil
}

func (s *Service) startSeeding(run *runState, sh Share) error {
	run.seedMu.Lock()
	defer run.seedMu.Unlock()

	run.mu.Lock()
	_, exists := run.seeded[sh.GameID]
	held := run.hashHeldLocked(sh.InfoHash)
	run.mu.Unlock()
	if exists {
		return nil
	}
	if held {
		return errAlreadyHave
	}

	data, err := s.store.readTorrent(sh.InfoHash)
	if err != nil {
		return err
	}
	mi, err := metainfo.Load(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("lan: parse cached torrent %s: %w", sh.InfoHash, err)
	}
	t, err := run.cl.addForSeeding(mi, sh.Root)
	if err != nil {
		return fmt.Errorf("lan: add seed torrent: %w", err)
	}
	run.mu.Lock()
	run.seeded[sh.GameID] = t
	run.mu.Unlock()
	return nil
}

func (s *Service) Peers() []Peer {
	s.mu.Lock()
	run := s.run
	s.mu.Unlock()
	if run == nil {
		return []Peer{}
	}
	return run.table.list(time.Now())
}

func (s *Service) Available() []Offer {
	s.mu.Lock()
	run := s.run
	s.mu.Unlock()
	if run == nil {
		return []Offer{}
	}
	return run.table.available(time.Now())
}

func (s *Service) StatsOf() Stats {
	s.mu.Lock()
	run := s.run
	shares := s.store.list()
	s.mu.Unlock()

	active := 0
	for _, sh := range shares {
		if sh.Enabled {
			active++
		}
	}
	if run == nil {
		return Stats{Rejected: map[string]int64{}, SharesActive: active}
	}
	sent, received, rejected := run.stats.snapshot()
	now := time.Now()
	return Stats{
		AnnouncesSent:     sent,
		AnnouncesReceived: received,
		Rejected:          rejected,
		PeersKnown:        len(run.table.list(now)),
		OffersKnown:       len(run.table.available(now)),
		SharesActive:      active,
	}
}

// Receive starts downloading the share (infoHash) offered by peerID. The
// peer is added explicitly from the announce's own source address, never
// from the announce content, and the torrent carries no trackers or DHT:
// discovery happens once, over UDP, and the transfer itself is a direct
// BitTorrent connection to that one peer.
func (s *Service) Receive(infoHash, peerID string) (Transfer, error) {
	infoHash = strings.ToLower(strings.TrimSpace(infoHash))
	peerID = strings.ToLower(strings.TrimSpace(peerID))
	if !infoHashRe.MatchString(infoHash) {
		return Transfer{}, errAnnounceInfoHash
	}
	if !idRe.MatchString(peerID) {
		return Transfer{}, errAnnounceID
	}
	if s.lib == nil {
		return Transfer{}, errLibraryUnavailable
	}

	// Add under s.mu: disable clears s.run under it before it waits on wg.
	s.mu.Lock()
	run := s.run
	if run != nil {
		run.wg.Add(1)
	}
	s.mu.Unlock()
	if run == nil {
		return Transfer{}, errSharingDisabled
	}
	defer run.wg.Done()
	if run.ctx.Err() != nil {
		return Transfer{}, errSharingDisabled
	}

	gamesPath := strings.TrimSpace(s.gamesPath())
	if gamesPath == "" {
		return Transfer{}, errGamesPathUnavailable
	}

	offer, ok := run.table.find(peerID, infoHash, time.Now())
	if !ok {
		return Transfer{}, errOfferNotFound
	}

	srcAddr, err := netip.ParseAddr(offer.Addr)
	if err != nil {
		return Transfer{}, fmt.Errorf("lan: peer address: %w", err)
	}
	var hash metainfo.Hash
	if err := hash.FromHexString(offer.InfoHash); err != nil {
		return Transfer{}, fmt.Errorf("lan: infohash: %w", err)
	}

	id, err := randomID()
	if err != nil {
		return Transfer{}, err
	}
	now := time.Now()
	tr := Transfer{
		ID:        id,
		InfoHash:  offer.InfoHash,
		PeerID:    peerID,
		GameID:    offer.GameID,
		Title:     offer.Title,
		Total:     offer.SizeBytes,
		Status:    TransferReceiving,
		StartedAt: now,
		UpdatedAt: now,
	}

	// The slot is taken before the folder and the torrent exist: everything
	// after this point is slow, and the check must hold until it is done.
	run.mu.Lock()
	if run.hashHeldLocked(offer.InfoHash) {
		run.mu.Unlock()
		return Transfer{}, errAlreadyHave
	}
	ts := &transferState{transfer: tr}
	run.transfers[id] = ts
	run.mu.Unlock()

	t, dest, err := s.openTransfer(run, id, offer, hash, gamesPath)
	if err != nil {
		run.mu.Lock()
		delete(run.transfers, id)
		run.mu.Unlock()
		return Transfer{}, err
	}
	t.AddPeers([]torrent.PeerInfo{{
		Addr:    &net.TCPAddr{IP: net.IP(srcAddr.AsSlice()), Port: offer.Port},
		Source:  torrent.PeerSourceDirect,
		Trusted: true,
	}})

	transferCtx, cancel := context.WithCancel(run.ctx)

	run.mu.Lock()
	ts.t = t
	ts.cancel = cancel
	run.mu.Unlock()

	run.wg.Add(1)
	go s.watchTransfer(run, transferCtx, id, dest, offer)

	s.emit("lan:transfer", tr)
	return tr, nil
}

func (s *Service) openTransfer(run *runState, id string, offer Offer, hash metainfo.Hash, gamesPath string) (*torrent.Torrent, string, error) {
	if err := os.MkdirAll(gamesPath, 0o755); err != nil {
		return nil, "", err
	}
	dest := filepath.Join(gamesPath, receivingPrefix+id)
	if err := os.Mkdir(dest, 0o755); err != nil {
		return nil, "", fmt.Errorf("lan: create destination: %w", err)
	}
	t, err := run.cl.addForReceiving(hash, offer.Title, dest)
	if err != nil {
		err = fmt.Errorf("lan: add receiving torrent: %w", err)
		if rmErr := os.Remove(dest); rmErr != nil {
			return nil, "", errors.Join(err, fmt.Errorf("lan: remove unused destination: %w", rmErr))
		}
		return nil, "", err
	}
	return t, dest, nil
}

// receivingPrefix hides a transfer from "Find games", which skips dot folders:
// a process killed mid-transfer leaves the sender's marker in there, and a
// visible folder would be adopted with that marker's uninstall command.
const receivingPrefix = ".typhon-receiving-"

// removeReceiving refuses anything but a real directory with the prefix, so a
// wrong path or a planted link cannot turn this into a delete elsewhere.
func removeReceiving(dir string) error {
	if !strings.HasPrefix(filepath.Base(dir), receivingPrefix) {
		return fmt.Errorf("lan: %s is not a receiving folder", dir)
	}
	info, err := os.Lstat(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("lan: %s is not a directory", dir)
	}
	return os.RemoveAll(dir)
}

// discardReceiving is for a transfer headed to a failed or cancelled status:
// that status is what the user must see, and sweepReceiving retries on the next
// start, so a folder that would not go can only be logged.
func discardReceiving(dir string) {
	if err := removeReceiving(dir); err != nil {
		slog.Warn("remove receiving folder", "dir", dir, "error", err)
	}
}

// sweepReceiving deletes what a killed process left behind. Transfers do not
// resume, so such a folder is garbage. Everything here is logged only: leftovers
// are hidden from "Find games" and cannot stop sharing from starting.
func sweepReceiving(ctx context.Context, gamesPath string) {
	if strings.TrimSpace(gamesPath) == "" {
		return
	}
	entries, err := os.ReadDir(gamesPath)
	if errors.Is(err, fs.ErrNotExist) {
		return
	}
	if err != nil {
		slog.Warn("scan games folder for receiving leftovers", "dir", gamesPath, "error", err)
		return
	}
	for _, entry := range entries {
		if ctx.Err() != nil {
			return
		}
		if !strings.HasPrefix(entry.Name(), receivingPrefix) {
			continue
		}
		path := filepath.Join(gamesPath, entry.Name())
		// Windows reports a junction as irregular, not as a link.
		if entry.Type()&(fs.ModeSymlink|fs.ModeIrregular) != 0 || !entry.IsDir() {
			slog.Warn("refuse to sweep a receiving leftover that is not a folder", "path", path)
			continue
		}
		discardReceiving(path)
	}
}

// placeReceived moves a finished transfer to its visible folder. It never
// overwrites: a taken name gets the usual numbered fallback.
func placeReceived(run *runState, staging string, offer Offer) (string, error) {
	run.placeMu.Lock()
	defer run.placeMu.Unlock()

	base := filepath.Join(filepath.Dir(staging), sanitizeFolderName(offer.Title, offer.GameID))
	dest, err := reserveReceiveDirectory(base, offer.InfoHash)
	if err != nil {
		return "", err
	}
	// Windows refuses to rename onto a directory, even an empty one.
	if err := storage.Remove(dest); err != nil {
		return "", fmt.Errorf("lan: release reserved folder: %w", err)
	}
	if err := renameReceived(staging, dest); err != nil {
		return "", fmt.Errorf("lan: move received game: %w", err)
	}
	return dest, nil
}

// renameReceived is a variable so a test can make the final move fail.
var renameReceived = storage.Rename

func reserveReceiveDirectory(base, infoHash string) (string, error) {
	dest := base
	for suffix := 0; ; suffix++ {
		if suffix > 0 {
			dest = fmt.Sprintf("%s-%s-%d", base, infoHash[:8], suffix)
		}
		if err := os.Mkdir(dest, 0o755); err == nil {
			break
		} else if !errors.Is(err, fs.ErrExist) {
			return "", fmt.Errorf("lan: create destination: %w", err)
		}
	}

	return dest, nil
}

func (s *Service) Cancel(id string) error {
	s.mu.Lock()
	run := s.run
	s.mu.Unlock()
	if run == nil {
		return errSharingDisabled
	}
	run.mu.Lock()
	ts, ok := run.transfers[id]
	run.mu.Unlock()
	if !ok {
		return errUnknownTransfer
	}
	ts.cancel()
	return nil
}

func (s *Service) transferTorrent(run *runState, id string) *torrent.Torrent {
	run.mu.Lock()
	defer run.mu.Unlock()
	ts, ok := run.transfers[id]
	if !ok {
		return nil
	}
	return ts.t
}

func (s *Service) updateTransfer(run *runState, id string, mutate func(*Transfer)) {
	run.mu.Lock()
	ts, ok := run.transfers[id]
	if !ok {
		run.mu.Unlock()
		return
	}
	mutate(&ts.transfer)
	ts.transfer.UpdatedAt = time.Now()
	out := ts.transfer
	run.mu.Unlock()
	s.emit("lan:transfer", out)
}

func (s *Service) finishTransfer(run *runState, id string, status TransferStatus, err error) {
	run.mu.Lock()
	ts, ok := run.transfers[id]
	if !ok {
		run.mu.Unlock()
		return
	}
	ts.transfer.Status = status
	ts.transfer.UpdatedAt = time.Now()
	if err != nil {
		ts.transfer.Error = err.Error()
	}
	out := ts.transfer
	run.mu.Unlock()
	s.emit("lan:transfer", out)
}

func (s *Service) watchTransfer(run *runState, ctx context.Context, id, dest string, offer Offer) {
	defer run.wg.Done()
	t := s.transferTorrent(run, id)
	if t == nil {
		return
	}

	stop := func() {
		t.Drop()
		discardReceiving(dest)
		s.finishTransfer(run, id, TransferCancelled, ctx.Err())
	}

	select {
	case <-t.GotInfo():
	case <-ctx.Done():
		stop()
		return
	}

	total := t.Info().TotalLength()
	t.DownloadAll()
	s.updateTransfer(run, id, func(tr *Transfer) { tr.Total = total })

	finish := func() {
		s.updateTransfer(run, id, func(tr *Transfer) { tr.Downloaded = total })
		// A live torrent re-stats its files for piece completion: once the
		// marker is removed it would mark the piece missing and fetch it back.
		t.Drop()
		s.completeTransfer(ctx, run, id, dest, offer, t)
	}

	ticker := time.NewTicker(transferSampleInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			// select picks at random when both are ready, and a game that has
			// fully arrived is finished even if the launcher is closing.
			if t.Complete().Bool() {
				finish()
				return
			}
			stop()
			return
		// BytesCompleted counts written but not yet hashed chunks, so it hits
		// total before the last piece's part files are renamed into place;
		// Complete is set only after every piece's MarkComplete returned.
		case <-t.Complete().On():
			finish()
			return
		case <-ticker.C:
			completed := t.BytesCompleted()
			s.updateTransfer(run, id, func(tr *Transfer) { tr.Downloaded = completed })
		}
	}
}

// clearReadOnly is a variable so a test can observe what the engine left behind.
var clearReadOnly = makeWritable

// makeWritable undoes the read-only mode the torrent engine puts on promoted
// files: a game must be able to write next to its executable.
func makeWritable(ctx context.Context, dir string) (err error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer func() {
		err = errors.Join(err, root.Close())
	}()
	return fs.WalkDir(root.FS(), ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if !d.Type().IsRegular() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.Mode().Perm()&0o200 != 0 {
			return nil
		}
		return root.Chmod(filepath.FromSlash(path), info.Mode()|0o200)
	})
}

// removeForeignMarker deletes the marker a sender older than this version put
// into the transfer: its game id and uninstall command describe the sender's
// install, and "Find games" would adopt the folder by it.
func removeForeignMarker(dir string) error {
	if _, err := library.MarkerPath(dir); err != nil {
		return fmt.Errorf("lan: foreign install marker: %w", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("lan: read received folder: %w", err)
	}
	// A case-insensitive volume finds the marker under any spelling a torrent
	// carried, and a folder can be moved from a case-sensitive one to such.
	for _, entry := range entries {
		if !strings.EqualFold(entry.Name(), library.MarkerName) {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		if err := storage.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("lan: remove foreign install marker: %w", err)
		}
	}
	return nil
}

func receivedExecutable(dest, exe string) (string, error) {
	exePath := filepath.Join(dest, filepath.FromSlash(exe))
	if !platform.Inside(dest, exePath) {
		return "", errExeOutsideInstall
	}
	info, err := os.Stat(exePath)
	if err != nil || info.IsDir() {
		return "", uierr.Wrap("lan.executable_missing", fmt.Errorf("lan: executable missing after transfer: %s", exePath))
	}
	return exePath, nil
}

func (s *Service) completeTransfer(ctx context.Context, run *runState, id, staging string, offer Offer, t *torrent.Torrent) {
	if _, err := receivedExecutable(staging, offer.Exe); err != nil {
		discardReceiving(staging)
		s.finishTransfer(run, id, TransferFailed, err)
		return
	}
	if err := removeForeignMarker(staging); err != nil {
		s.finishTransfer(run, id, TransferFailed, uierr.Wrap("lan.finish_files_failed", err))
		return
	}
	// Every byte is on disk by now: closing the launcher must not lose a fully
	// received game. run.wg still holds Shutdown until this returns.
	if err := clearReadOnly(context.WithoutCancel(ctx), staging); err != nil {
		s.finishTransfer(run, id, TransferFailed, uierr.Wrap("lan.finish_files_failed", fmt.Errorf("lan: make received files writable: %w", err)))
		return
	}
	dest, err := placeReceived(run, staging, offer)
	if err != nil {
		s.finishTransfer(run, id, TransferFailed, uierr.Wrap("lan.finish_files_failed", err))
		return
	}
	exePath, err := receivedExecutable(dest, offer.Exe)
	if err != nil {
		s.finishTransfer(run, id, TransferFailed, err)
		return
	}

	game, err := s.lib.RegisterInstalled(ctx, library.InstalledGame{
		Title:         offer.Title,
		Executable:    exePath,
		InstallDir:    dest,
		VersionSource: "lan",
		InstallType:   "lan",
	})
	if err != nil {
		s.finishTransfer(run, id, TransferFailed, uierr.Wrap("lan.register_game_failed", err))
		return
	}

	run.mu.Lock()
	ts, ok := run.transfers[id]
	var out Transfer
	if ok {
		ts.transfer.Status = TransferCompleted
		ts.transfer.Downloaded = ts.transfer.Total
		ts.transfer.UpdatedAt = time.Now()
		ts.transfer.GameID = game.ID
		out = ts.transfer
	}
	run.mu.Unlock()
	if ok {
		s.emit("lan:transfer", out)
	}

	s.mu.Lock()
	recorder := s.recorder
	s.mu.Unlock()
	if recorder == nil {
		return
	}
	if err := recorder(history.Record{
		Kind:       history.KindLanReceived,
		GameID:     game.ID,
		Title:      game.Title,
		ToVersion:  game.Version,
		Bytes:      offer.SizeBytes,
		BytesKnown: true,
		Detail:     "lan:" + offer.Host,
	}); err != nil {
		slog.Error("record lan transfer history", "error", err)
	}
}

func sanitizeFolderName(title, gameID string) string {
	name := strings.TrimSpace(title)
	if name == "" {
		name = gameID
	}
	var b strings.Builder
	for _, r := range name {
		switch {
		case r < 0x20:
			continue
		case strings.ContainsRune(`<>:"/\|?*`, r):
			b.WriteRune('_')
		default:
			b.WriteRune(r)
		}
	}
	// A leading dot would hide the game from "Find games", and a sender's title
	// must not be able to pose as a receiving folder the startup sweep deletes.
	out := strings.Trim(strings.TrimSpace(b.String()), " .")
	if out == "" {
		out = "lan-" + gameID
	}
	if len(out) > 100 {
		out = out[:100]
	}
	return out
}

func (s *Service) announceLoop(run *runState) {
	defer run.wg.Done()
	s.sendAnnounces(run)
	ticker := time.NewTicker(announceInterval)
	defer ticker.Stop()
	for {
		select {
		case <-run.ctx.Done():
			return
		case <-ticker.C:
			s.sendAnnounces(run)
		case <-run.changed:
			s.sendAnnounces(run)
		}
	}
}

func (s *Service) sendAnnounces(run *runState) {
	run.mu.Lock()
	ids := make([]string, 0, len(run.seeded))
	for id := range run.seeded {
		ids = append(ids, id)
	}
	run.mu.Unlock()
	if len(ids) == 0 {
		return
	}

	shares := s.Shares()
	byID := make(map[string]Share, len(shares))
	for _, sh := range shares {
		byID[sh.GameID] = sh
	}
	port := run.cl.localPort()
	now := time.Now()

	for _, id := range ids {
		sh, ok := byID[id]
		if !ok || !sh.Enabled {
			continue
		}
		payload, err := encodeAnnounce(announceMsg{
			V:        1,
			ID:       s.selfID,
			Host:     s.hostname,
			Port:     port,
			GameID:   sh.GameID,
			Title:    sh.Title,
			Version:  sh.Version,
			Exe:      sh.Exe,
			Size:     sh.SizeBytes,
			InfoHash: sh.InfoHash,
			TS:       now.Unix(),
		})
		if err != nil {
			slog.Error("encode lan announce", "gameId", id, "error", err)
			continue
		}
		if err := run.transport.Send(run.ctx, payload); err != nil {
			if run.ctx.Err() != nil {
				return
			}
			slog.Warn("send lan announce", "error", err)
			continue
		}
		run.stats.addSent(1)
	}
	s.emit("lan:stats", s.StatsOf())
}

func (s *Service) receiveLoop(run *runState) {
	defer run.wg.Done()
	for {
		payload, addr, err := run.transport.Recv(run.ctx)
		if err != nil {
			if run.ctx.Err() != nil {
				return
			}
			slog.Warn("receive lan announce", "error", err)
			continue
		}
		now := time.Now()
		run.limiter.prune(now)
		if !run.limiter.allow(addr, now) {
			run.stats.reject("rate_limited")
			s.emit("lan:stats", s.StatsOf())
			continue
		}
		msg, err := decodeAnnounce(payload, addr, s.selfID, now)
		if err != nil {
			run.stats.reject(reasonFor(err))
			s.emit("lan:stats", s.StatsOf())
			continue
		}
		run.stats.addReceived()
		if !run.table.observe(msg, addr, now) {
			run.stats.reject("capacity")
			s.emit("lan:stats", s.StatsOf())
			continue
		}
		s.emit("lan:peers", run.table.list(now))
		s.emit("lan:stats", s.StatsOf())
	}
}
