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
	"time"

	"typhon/internal/history"
	"typhon/internal/library"
	"typhon/internal/platform"
	"typhon/internal/settings"
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

	mu        sync.Mutex
	seeded    map[string]*torrent.Torrent
	transfers map[string]*transferState
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

	if run != nil && run.cl != nil {
		run.cl.applyLimits(next.DownloadRateLimit, next.UploadRateLimit)
	}

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
	shares := s.store.list()
	s.mu.Unlock()

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
	s.mu.Lock()
	run := s.run
	s.run = nil
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
			cached.Enabled = true
			cached.Exe = filepath.ToSlash(exeRel)
			cached.Title = game.Title
			cached.Version = game.Version
			if err := s.persistShare(cached); err != nil {
				return Share{}, err
			}
			if err := s.startSeeding(run, cached); err != nil {
				slog.Error("resume lan seed", "gameId", gameID, "error", err)
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
		slog.Error("start lan seed", "gameId", gameID, "error", err)
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
	run.mu.Lock()
	_, exists := run.seeded[sh.GameID]
	run.mu.Unlock()
	if exists {
		return nil
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

	s.mu.Lock()
	run := s.run
	s.mu.Unlock()
	if run == nil {
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

	dest := filepath.Join(gamesPath, sanitizeFolderName(offer.Title, offer.GameID))
	if entries, err := os.ReadDir(dest); err == nil && len(entries) > 0 {
		dest = filepath.Join(gamesPath, sanitizeFolderName(offer.Title, offer.GameID)+"-"+offer.InfoHash[:8])
	} else if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return Transfer{}, fmt.Errorf("lan: check destination %s: %w", dest, err)
	}
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return Transfer{}, fmt.Errorf("lan: create %s: %w", dest, err)
	}

	t, err := run.cl.addForReceiving(hash, offer.Title, dest)
	if err != nil {
		return Transfer{}, fmt.Errorf("lan: add receiving torrent: %w", err)
	}
	t.AddPeers([]torrent.PeerInfo{{
		Addr:    &net.TCPAddr{IP: net.IP(srcAddr.AsSlice()), Port: offer.Port},
		Source:  torrent.PeerSourceDirect,
		Trusted: true,
	}})

	id, err := randomID()
	if err != nil {
		t.Drop()
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
	transferCtx, cancel := context.WithCancel(run.ctx)

	run.mu.Lock()
	run.transfers[id] = &transferState{transfer: tr, t: t, cancel: cancel}
	run.mu.Unlock()

	run.wg.Add(1)
	go s.watchTransfer(run, transferCtx, id, dest, offer)

	s.emit("lan:transfer", tr)
	return tr, nil
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

	select {
	case <-t.GotInfo():
	case <-ctx.Done():
		s.finishTransfer(run, id, TransferCancelled, ctx.Err())
		t.Drop()
		return
	}

	total := t.Info().TotalLength()
	t.DownloadAll()
	s.updateTransfer(run, id, func(tr *Transfer) { tr.Total = total })

	ticker := time.NewTicker(transferSampleInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			s.finishTransfer(run, id, TransferCancelled, ctx.Err())
			t.Drop()
			return
		case <-ticker.C:
			completed := t.BytesCompleted()
			s.updateTransfer(run, id, func(tr *Transfer) { tr.Downloaded = completed })
			if completed >= total {
				s.completeTransfer(run, id, dest, offer, t)
				return
			}
		}
	}
}

func (s *Service) completeTransfer(run *runState, id, dest string, offer Offer, t *torrent.Torrent) {
	exePath := filepath.Join(dest, filepath.FromSlash(offer.Exe))
	if !platform.Inside(dest, exePath) {
		s.finishTransfer(run, id, TransferFailed, errors.New("lan: received executable escapes destination"))
		return
	}
	info, err := os.Stat(exePath)
	if err != nil || info.IsDir() {
		s.finishTransfer(run, id, TransferFailed, fmt.Errorf("lan: executable missing after transfer: %s", exePath))
		return
	}

	game, err := s.lib.RegisterInstalled(library.InstalledGame{
		Title:         offer.Title,
		Executable:    exePath,
		InstallDir:    dest,
		VersionSource: "lan",
		InstallType:   "lan",
	})
	if err != nil {
		s.finishTransfer(run, id, TransferFailed, fmt.Errorf("lan: register game: %w", err))
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
	out := strings.TrimRight(strings.TrimSpace(b.String()), " .")
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
