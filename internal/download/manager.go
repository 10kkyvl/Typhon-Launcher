package download

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	// classicio must be initialized before anacrolix storage reads
	// TORRENT_STORAGE_DEFAULT_FILE_IO: mmap file IO never releases mappings,
	// which keeps files locked on Windows.
	_ "typhon/internal/download/classicio"
	"typhon/internal/history"
	"typhon/internal/platform"
	"typhon/internal/redact"
	"typhon/internal/settings"
	"typhon/internal/uierr"
	"typhon/internal/usagestats"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/anacrolix/torrent/storage"
	"github.com/wailsapp/wails/v3/pkg/application"
)

const (
	eventAdded     = "download:added"
	eventUpdated   = "download:updated"
	eventCompleted = "download:completed"
	eventFailed    = "download:failed"
	eventRemoved   = "download:removed"
	eventDegraded  = "download:degraded"

	metadataTimeout = 90 * time.Second
	tickInterval    = 250 * time.Millisecond
	persistInterval = 5 * time.Second
)

// degradedStatus mirrors history.Status: it is not exported so that fixing
// manager.go:persistLocked's swallowed error does not add a new wails
// binding. The frontend learns about it only through eventDegraded.
type degradedStatus struct {
	Degraded bool   `json:"degraded"`
	Message  string `json:"message"`
}

// stallAfter is a var so tests can shrink the grace period instead of
// sleeping for it.
var stallAfter = 2 * time.Minute

const restoreFailedMessage = "не удалось восстановить загрузку"

var ErrNotFound = uierr.New("download.not_found", "загрузка не найдена")

var (
	errNotFound          = ErrNotFound
	errUnavailable       = uierr.New("download.unavailable", "недоступно для этой загрузки")
	errNoClient          = uierr.New("download.no_client", "торрент-клиент недоступен")
	errNoMetadata        = uierr.New("download.no_metadata", "не удалось получить метаданные торрента")
	errNoRestore         = uierr.New("download.restore_failed", restoreFailedMessage)
	errSeeding           = uierr.New("download.seeding", "файлы сейчас раздаются — сначала остановите раздачу")
	errBadSizes          = uierr.New("download.bad_sizes", "недопустимые размеры файлов в торренте")
	errNoFreeSpace       = uierr.New("download.no_free_space", "не удалось определить свободное место на диске")
	errNotEnoughSpace    = uierr.New("download.not_enough_space", "недостаточно места на диске")
	errDiskWriteFailed   = uierr.New("download.disk_write_failed", "ошибка записи на диск")
	errEmptySource       = uierr.New("download.empty_source", "укажите magnet-ссылку или torrent-файл")
	errDuplicateDownload = uierr.New("download.duplicate_download", "эта загрузка уже добавлена")
	errInvalidMagnet     = uierr.New("download.invalid_magnet", "некорректная magnet-ссылка")
	errTorrentReadFailed = uierr.New("download.torrent_read_failed", "не удалось прочитать torrent-файл")
	errMetadataRequired  = uierr.New("download.metadata_required", "сначала получите метаданные торрента")
	errEmptyDestination  = uierr.New("download.empty_destination", "укажите папку назначения")
	errDestPermission    = uierr.New("download.destination_permission", "нет доступа к папке назначения")
	errDestUnavailable   = uierr.New("download.destination_unavailable", "папка назначения недоступна")
	errNoFilesSelected   = uierr.New("download.no_files_selected", "не выбрано ни одного файла")
	errAddTorrentFailed  = uierr.New("download.add_torrent_failed", "не удалось добавить торрент")
)

type pending struct {
	torrent *liveTorrent
	source  string
}

type jobState struct {
	cancel context.CancelFunc
	done   chan struct{}
}

type Manager struct {
	mu              sync.Mutex
	settings        *settings.Service
	store           *store
	metaDir         string
	pieceCompletion storage.PieceCompletion

	items    []*Download
	engines  map[string]engineTorrent
	rates    map[string]*rateState
	pending  map[string]*pending
	jobs     map[string]*jobState
	reserved map[string]bool

	client          *client
	max             int
	onCompleted     func(Download)
	usageRecorder   func(usagestats.Event)
	historyRecorder func(history.Record) error

	ctx         context.Context
	cancel      context.CancelFunc
	closing     bool
	wg          sync.WaitGroup
	unsubscribe func()
	lastPersist time.Time
	degraded    degradedStatus
}

func NewManager(settingsService *settings.Service) (*Manager, error) {
	dir, err := settings.ConfigDir()
	if err != nil {
		return nil, fmt.Errorf("resolve config dir: %w", err)
	}
	return newManagerAt(dir, settingsService)
}

func newManagerAt(dir string, settingsService *settings.Service) (*Manager, error) {
	if dir == "" {
		return nil, errors.New("downloads path unavailable")
	}
	m := &Manager{
		settings: settingsService,
		store:    newStore(dir),
		engines:  map[string]engineTorrent{},
		rates:    map[string]*rateState{},
		pending:  map[string]*pending{},
		jobs:     map[string]*jobState{},
		reserved: map[string]bool{},
	}
	m.metaDir = filepath.Join(dir, "meta")
	completion, err := openPieceCompletion(m.metaDir)
	if err != nil {
		return nil, err
	}
	m.pieceCompletion = completion
	m.max = maxActive(m.config())
	return m, nil
}

func (m *Manager) config() settings.Settings {
	if m.settings == nil {
		return settings.Defaults()
	}
	return m.settings.GetSettings()
}

func maxActive(cfg settings.Settings) int {
	if cfg.MaxActiveDownloads < 1 {
		return 1
	}
	if cfg.MaxActiveDownloads > 10 {
		return 10
	}
	return cfg.MaxActiveDownloads
}

func (m *Manager) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	m.mu.Lock()
	m.ctx, m.cancel = context.WithCancel(ctx)
	cfg := m.config()
	m.max = maxActive(cfg)
	if err := m.loadLocked(); err != nil {
		cancel := m.cancel
		m.cancel = nil
		m.mu.Unlock()
		cancel()
		return err
	}
	cl, err := newClient(cfg, m.metaDir, m.pieceCompletion)
	if err != nil {
		slog.Error("start torrent client", "error", err)
	} else {
		m.client = cl
	}
	known := make(map[string]bool, len(m.items))
	for _, d := range m.items {
		known[strings.ToLower(d.InfoHash)] = true
	}
	m.store.sweepMetainfo(known)
	m.mu.Unlock()

	if m.settings != nil {
		m.unsubscribe = m.settings.Subscribe(m.applySettings)
	}
	m.wg.Add(2)
	go m.restore()
	go m.tick()
	return nil
}

func (m *Manager) ServiceShutdown() error {
	m.mu.Lock()
	m.closing = true
	cancel := m.cancel
	unsubscribe := m.unsubscribe
	m.mu.Unlock()

	if unsubscribe != nil {
		unsubscribe()
	}
	if cancel != nil {
		cancel()
	}
	m.wg.Wait()

	m.mu.Lock()
	persistErr := m.persistLocked()
	cl := m.client
	m.client = nil
	pc := m.pieceCompletion
	m.pieceCompletion = nil
	m.mu.Unlock()

	if cl != nil {
		cl.close()
	}
	if pc != nil {
		if err := pc.Close(); err != nil {
			slog.Error("close piece completion db", "error", err)
		}
	}
	if persistErr != nil {
		return fmt.Errorf("shut down downloads: %w", persistErr)
	}
	return nil
}

func (m *Manager) loadLocked() error {
	records, err := m.store.load()
	if err != nil {
		return err
	}
	for _, r := range records {
		d := &Download{
			ID:          r.ID,
			Name:        r.Name,
			Type:        r.Type,
			Source:      r.Source,
			InfoHash:    r.InfoHash,
			Destination: r.Destination,
			Status:      r.Status,
			Downloaded:  r.Downloaded,
			Total:       r.Total,
			ETASeconds:  -1,
			Seeding:     r.Seeding,
			Flat:        r.Flat,
			InPlace:     r.InPlace,
			Origin:      r.Origin,
			AddedAt:     r.AddedAt,
			CompletedAt: r.CompletedAt,
			Error:       r.Error,
		}
		if d.Type == "" {
			d.Type = TypeTorrent
		}
		if mi, err := m.store.loadMetainfo(r.InfoHash); err == nil {
			if info, err := mi.UnmarshalInfo(); err == nil {
				d.Files = fileStates(&info, r.Selected)
				d.Total = selectedTotal(d.Files)
			}
		}
		if occupiesSlot(d.Status) {
			d.Status = StatusQueued
		}
		d.Progress = ratio(d.Downloaded, d.Total)
		m.items = append(m.items, d)
	}
	return nil
}

// persistLocked writes the current in-memory queue to disk. On failure it
// flips the manager into a degraded state and emits eventDegraded so the
// frontend learns about it even from call sites that have no error to return
// to (see the callers below); on success it clears that state, mirroring
// history.Service.Record. The caller decides, based on what it just mutated,
// whether to also roll that change back — persistLocked only knows about
// records, not about which Download field motivated this call.
func (m *Manager) persistLocked() error {
	records := make([]record, 0, len(m.items))
	for _, d := range m.items {
		records = append(records, record{
			ID:          d.ID,
			Name:        d.Name,
			Type:        d.Type,
			Source:      d.Source,
			InfoHash:    d.InfoHash,
			Destination: d.Destination,
			Status:      d.Status,
			Selected:    selectedIndices(d),
			Downloaded:  d.Downloaded,
			Total:       d.Total,
			Seeding:     d.Seeding,
			Flat:        d.Flat,
			InPlace:     d.InPlace,
			Origin:      d.Origin,
			AddedAt:     d.AddedAt,
			CompletedAt: d.CompletedAt,
			Error:       d.Error,
		})
	}
	if err := m.store.save(records); err != nil {
		m.degraded = degradedStatus{Degraded: true, Message: err.Error()}
		emit(eventDegraded, m.degraded)
		return fmt.Errorf("persist downloads: %w", err)
	}
	m.degraded = degradedStatus{}
	return nil
}

func emit(name string, data any) {
	if app := application.Get(); app != nil {
		app.Event.Emit(name, data)
	}
}

func (m *Manager) findLocked(id string) *Download {
	for _, d := range m.items {
		if d.ID == id {
			return d
		}
	}
	return nil
}

func (m *Manager) findByHashLocked(infoHash string) *Download {
	for _, d := range m.items {
		if strings.EqualFold(d.InfoHash, infoHash) {
			return d
		}
	}
	return nil
}

func (m *Manager) List() []Download {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Download, 0, len(m.items))
	for _, d := range m.items {
		out = append(out, snapshot(d))
	}
	return out
}

func (m *Manager) Get(id string) (Download, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	d := m.findLocked(id)
	if d == nil {
		return Download{}, errNotFound
	}
	return snapshot(d), nil
}

func (m *Manager) AddTorrentSelectFile() (string, error) {
	dialog := application.Get().Dialog.OpenFile().
		SetTitle("Выберите torrent-файл").
		CanChooseFiles(true).
		AddFilter("Torrent-файлы (*.torrent)", "*.torrent").
		AddFilter("Все файлы", "*.*")
	path, err := dialog.PromptForSingleSelection()
	if err != nil {
		slog.Warn("select torrent file", "error", err)
		return "", err
	}
	return path, nil
}

func (m *Manager) FetchMetadata(source string) (TorrentInfo, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		return TorrentInfo{}, errEmptySource
	}

	m.mu.Lock()
	cl := m.client
	ctx := m.ctx
	m.mu.Unlock()
	if cl == nil || ctx == nil {
		return TorrentInfo{}, errNoClient
	}

	spec, err := buildSpec(source)
	if err != nil {
		slog.Error("parse torrent source", "operation", "fetch_metadata", "source", redact.Source(source), "error", err)
		return TorrentInfo{}, err
	}
	infoHash := spec.InfoHash.HexString()

	// The busy check and the reservation that closes the gap until
	// m.pending[infoHash] is set below happen under the same lock
	// (invariant 17): a second FetchMetadata for the same hash must see
	// either the duplicate-download, the already-pending, or the reserved
	// outcome, never a false "free" in between.
	m.mu.Lock()
	if m.findByHashLocked(infoHash) != nil {
		m.mu.Unlock()
		return TorrentInfo{}, errDuplicateDownload
	}
	if existing, ok := m.pending[infoHash]; ok {
		m.mu.Unlock()
		info := existing.torrent.t.Info()
		if info != nil {
			return torrentInfoOf(infoHash, info), nil
		}
		return TorrentInfo{}, errNoMetadata
	}
	if m.hashBusyLocked(infoHash) {
		m.mu.Unlock()
		return TorrentInfo{}, errHashBusy
	}
	m.reserved[infoHash] = true
	m.mu.Unlock()
	reserved := true
	defer func() {
		if reserved {
			m.releaseHash(infoHash)
		}
	}()

	lt, err := cl.add(spec, cl.metaDir, storageOpts{})
	if err != nil {
		slog.Error("add torrent for metadata", "operation", "fetch_metadata", "source", redact.Source(source), "error", err)
		return TorrentInfo{}, fmt.Errorf("%w: %w", errAddTorrentFailed, err)
	}

	select {
	case <-lt.t.GotInfo():
	case <-ctx.Done():
		lt.drop()
		return TorrentInfo{}, errNoMetadata
	case <-time.After(metadataTimeout):
		lt.drop()
		slog.Warn("metadata timeout", "operation", "fetch_metadata", "source", redact.Source(source))
		return TorrentInfo{}, errNoMetadata
	}

	info := lt.t.Info()
	if err := validateInfo(info); err != nil {
		lt.drop()
		slog.Warn("unsafe torrent paths", "operation", "fetch_metadata")
		return TorrentInfo{}, err
	}

	mi := lt.t.Metainfo()
	if err := m.store.saveMetainfo(infoHash, &mi); err != nil {
		slog.Warn("save metainfo", "operation", "fetch_metadata", "error", err)
	}

	m.mu.Lock()
	m.pending[infoHash] = &pending{torrent: lt, source: source}
	delete(m.reserved, infoHash)
	m.mu.Unlock()
	reserved = false

	return torrentInfoOf(infoHash, info), nil
}

func buildSpec(source string) (*torrent.TorrentSpec, error) {
	if strings.HasPrefix(source, "magnet:") {
		spec, err := magnetSpec(source)
		if err != nil {
			return nil, errInvalidMagnet
		}
		return spec, nil
	}
	mi, err := metainfo.LoadFromFile(source)
	if err != nil {
		return nil, errTorrentReadFailed
	}
	spec, err := torrent.TorrentSpecFromMetaInfoErr(mi)
	if err != nil || spec.InfoHash.IsZero() {
		return nil, errTorrentReadFailed
	}
	return spec, nil
}

func torrentInfoOf(infoHash string, info *metainfo.Info) TorrentInfo {
	files := fileStates(info, nil)
	return TorrentInfo{
		InfoHash:   infoHash,
		Name:       info.BestName(),
		TotalBytes: selectedTotal(files),
		Files:      files,
	}
}

func fileStates(info *metainfo.Info, selected []int) []FileState {
	all := selected == nil
	chosen := make(map[int]bool, len(selected))
	for _, i := range selected {
		chosen[i] = true
	}
	upverted := info.UpvertedFiles()
	states := make([]FileState, 0, len(upverted))
	for i := range upverted {
		fi := upverted[i]
		states = append(states, FileState{
			Path:     fi.DisplayPath(info),
			Size:     fi.Length,
			Selected: all || chosen[i],
		})
	}
	return states
}

func (m *Manager) DiscardMetadata(infoHash string) {
	m.mu.Lock()
	p := m.pending[infoHash]
	delete(m.pending, infoHash)
	m.mu.Unlock()
	if p != nil {
		p.torrent.drop()
	}
	m.discardMetainfo(infoHash)
}

func (m *Manager) discardMetainfo(infoHash string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.findByHashLocked(infoHash) != nil || m.pending[infoHash] != nil {
		return
	}
	m.store.removeMetainfo(infoHash)
}

// returnPending hands a fetched-metadata torrent back to m.pending after a
// failed StartDownloadFrom attempt, so the caller can retry without
// re-fetching. It also releases the infohash reservation that
// StartDownloadFrom took when it pulled p out of m.pending: once p is back in
// m.pending (or dropped because someone else raced in and left a different
// entry there), hashBusyLocked's own pending check covers the hash again.
func (m *Manager) returnPending(infoHash string, p *pending) {
	m.mu.Lock()
	delete(m.reserved, infoHash)
	if _, taken := m.pending[infoHash]; !taken {
		m.pending[infoHash] = p
		m.mu.Unlock()
		return
	}
	m.mu.Unlock()
	p.torrent.drop()
}

func (m *Manager) StartDownload(infoHash, destination string, selectedIndices []int) (Download, error) {
	return m.StartDownloadFrom(infoHash, destination, selectedIndices, Origin{})
}

// StartDownloadFrom takes over a torrent that FetchMetadata already fetched
// and turns it into a live download. Taking p out of m.pending removes the
// signal hashBusyLocked used to see this hash as busy, so the window until
// the new download is either committed to m.items/m.engines or handed back
// through returnPending is covered by reserving the hash here (invariant 17:
// this is not one of the four windows the audit named, but the same TOCTOU
// applies to it and anacrolix/torrent's own dedup cannot be relied on either).
func (m *Manager) StartDownloadFrom(infoHash, destination string, selectedIndices []int, origin Origin) (Download, error) {
	m.mu.Lock()
	p := m.pending[infoHash]
	if p != nil {
		delete(m.pending, infoHash)
		m.reserved[infoHash] = true
	}
	cl := m.client
	m.mu.Unlock()
	if p == nil {
		return Download{}, errMetadataRequired
	}
	if cl == nil {
		m.returnPending(infoHash, p)
		return Download{}, errNoClient
	}

	destination = strings.TrimSpace(destination)
	if destination == "" {
		m.returnPending(infoHash, p)
		return Download{}, errEmptyDestination
	}
	if err := os.MkdirAll(destination, 0o755); err != nil {
		m.returnPending(infoHash, p)
		slog.Error("create destination", "path", destination, "error", err)
		if errors.Is(err, fs.ErrPermission) {
			return Download{}, errDestPermission
		}
		return Download{}, errDestUnavailable
	}

	info := p.torrent.t.Info()
	files := fileStates(info, selectedIndices)
	needed, err := requiredBytes(files)
	if err != nil {
		m.returnPending(infoHash, p)
		return Download{}, err
	}
	if needed == 0 {
		m.returnPending(infoHash, p)
		return Download{}, errNoFilesSelected
	}
	if err := checkFreeSpace(destination, needed); err != nil {
		m.returnPending(infoHash, p)
		return Download{}, err
	}

	mi := p.torrent.t.Metainfo()
	p.torrent.drop()

	lt, err := cl.addMetainfo(&mi, destination, storageOpts{})
	if err != nil {
		// p.torrent is already gone, so there is nothing left to hand back
		// through returnPending; release the reservation directly.
		m.releaseHash(infoHash)
		slog.Error("add torrent", "operation", "start_download", "error", err)
		return Download{}, fmt.Errorf("%w: %w", errAddTorrentFailed, err)
	}

	d := &Download{
		ID:          newID(),
		Name:        info.BestName(),
		Type:        TypeTorrent,
		Source:      p.source,
		InfoHash:    infoHash,
		Destination: destination,
		Status:      StatusQueued,
		Total:       needed,
		ETASeconds:  -1,
		Files:       files,
		Origin:      origin,
		AddedAt:     time.Now(),
	}
	lt.setPriorities(selectionOf(d))
	m.watchWriteErrors(d.ID, lt)

	m.mu.Lock()
	m.items = append(m.items, d)
	m.engines[d.ID] = lt
	if err := m.store.saveMetainfo(infoHash, &mi); err != nil {
		slog.Warn("save metainfo", "download_id", d.ID, "error", err)
	}
	if err := m.persistLocked(); err != nil {
		m.items = m.items[:len(m.items)-1]
		delete(m.engines, d.ID)
		delete(m.reserved, infoHash)
		m.mu.Unlock()
		lt.drop()
		return Download{}, fmt.Errorf("добавить загрузку: %w", err)
	}
	delete(m.reserved, infoHash)
	slog.Info("download added", "download_id", d.ID, "name", d.Name)
	emit(eventAdded, snapshot(d))
	m.recordUsage(usagestats.Event{
		Type:      usagestats.TypeDownloadStarted,
		Timestamp: time.Now(),
		Properties: usagestats.Properties{
			GameID: origin.GameID,
		},
	})
	m.schedule()
	m.mu.Unlock()
	return snapshot(d), nil
}

// spawnTrackedLocked starts fn in a goroutine registered with m.wg, unless
// the manager is already closing (invariant 19): every goroutine
// ServiceShutdown might need to wait for is counted before it starts, never
// after, and none starts at all once Shutdown has begun. The caller must
// hold m.mu for the call itself; fn runs unlocked.
func (m *Manager) spawnTrackedLocked(fn func()) bool {
	if m.closing {
		return false
	}
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		fn()
	}()
	return true
}

func (m *Manager) watchWriteErrors(id string, lt *liveTorrent) {
	lt.t.SetOnWriteChunkError(func(err error) {
		slog.Error("torrent write error", "download_id", id, "error", err)
		// The engine calls this from its own goroutine at an arbitrary time,
		// including possibly after ServiceShutdown has started, so the
		// spawn decision happens under m.mu (invariant 17/19): a write error
		// is exactly the moment persisting a StatusFailed is most likely to
		// matter, but it must not race Shutdown's wg.Wait.
		m.mu.Lock()
		started := m.spawnTrackedLocked(func() {
			m.markFailed(id, errDiskWriteFailed.Error(), err)
		})
		m.mu.Unlock()
		if !started {
			slog.Warn("skipped write-error handling, manager is shutting down", "download_id", id)
		}
	})
}

func (m *Manager) Pause(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	d := m.findLocked(id)
	if d == nil {
		return errNotFound
	}
	if d.Status != StatusDownloading && d.Status != StatusQueued {
		return errUnavailable
	}
	before := *d
	m.idleLocked(d, StatusPaused)
	if err := m.persistLocked(); err != nil {
		*d = before
		emit(eventUpdated, snapshot(d))
		return fmt.Errorf("поставить загрузку на паузу: %w", err)
	}
	// Gating the engine only after a successful persist keeps the two in
	// sync: if the write had failed, the download would still be
	// downloading on disk, and nothing here would have told the engine
	// otherwise.
	if eng := m.engines[id]; eng != nil {
		eng.disallowDownload()
		eng.disallowUpload()
	}
	emit(eventUpdated, snapshot(d))
	m.schedule()
	return nil
}

func (m *Manager) Resume(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	d := m.findLocked(id)
	if d == nil {
		return errNotFound
	}
	if d.Status != StatusPaused && d.Status != StatusFailed {
		return errUnavailable
	}
	if m.engines[id] == nil {
		return m.reattachLocked(d, false)
	}
	before := *d
	d.Status = StatusQueued
	d.Error = ""
	if err := m.persistLocked(); err != nil {
		*d = before
		emit(eventUpdated, snapshot(d))
		return fmt.Errorf("возобновить загрузку: %w", err)
	}
	emit(eventUpdated, snapshot(d))
	m.schedule()
	return nil
}

func (m *Manager) ForceStart(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	d := m.findLocked(id)
	if d == nil {
		return errNotFound
	}
	if d.Status != StatusQueued && d.Status != StatusPaused && d.Status != StatusFailed {
		return errUnavailable
	}
	if m.engines[id] == nil {
		return m.reattachLocked(d, true)
	}
	before := *d
	d.Status = StatusQueued
	d.Error = ""
	started := m.startLocked(d)
	if err := m.persistLocked(); err != nil {
		// startLocked may have already gated the engine on and emitted an
		// optimistic eventUpdated; there is no clean way to un-gate it from
		// here (schedule() shares startLocked without persisting at all), so
		// the best this can do is restore the Download record and correct
		// the frontend with a second, accurate event.
		*d = before
		emit(eventUpdated, snapshot(d))
		return fmt.Errorf("запустить загрузку: %w", err)
	}
	if !started {
		emit(eventUpdated, snapshot(d))
	}
	return nil
}

func (m *Manager) reattachLocked(d *Download, force bool) error {
	if m.jobs[d.ID] != nil {
		return errUnavailable
	}
	if !m.store.hasMetainfo(d.InfoHash) && !strings.HasPrefix(d.Source, "magnet:") {
		slog.Warn("cannot reattach download", "download_id", d.ID)
		return errNoRestore
	}
	if m.client == nil || m.ctx == nil || m.closing {
		return errNoClient
	}

	job := restoreJob{
		id:       d.ID,
		infoHash: d.InfoHash,
		source:   d.Source,
		dest:     d.Destination,
		flat:     d.Flat,
		inPlace:  d.InPlace,
		force:    force,
	}
	before := *d
	d.Status = StatusVerifying
	d.Error = ""
	if err := m.persistLocked(); err != nil {
		*d = before
		emit(eventUpdated, snapshot(d))
		return fmt.Errorf("восстановить загрузку: %w", err)
	}
	emit(eventUpdated, snapshot(d))

	m.spawnRestoreLocked(job)
	return nil
}

func (m *Manager) spawnRestoreLocked(job restoreJob) {
	cl, ctx := m.client, m.ctx
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		m.restoreOne(ctx, cl, job)
	}()
}

//wails:ignore
func (m *Manager) SetOnCompleted(fn func(Download)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onCompleted = fn
}

//wails:ignore
func (m *Manager) SetUsageRecorder(rec func(usagestats.Event)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.usageRecorder = rec
}

// recordUsage assumes the caller already holds m.mu, same as the other
// *Locked helpers; every call site below records the event before
// releasing the lock it took for the surrounding state change.
func (m *Manager) recordUsage(ev usagestats.Event) {
	if m.usageRecorder == nil {
		return
	}
	m.usageRecorder(ev)
}

func (m *Manager) Cancel(id string) error { return m.discard(id, true) }

func (m *Manager) Remove(id string) error { return m.discard(id, false) }

func (m *Manager) DeleteData(id string) error {
	m.mu.Lock()
	d := m.findLocked(id)
	if d == nil {
		m.mu.Unlock()
		return errNotFound
	}
	if d.Status != StatusCompleted {
		m.mu.Unlock()
		return errUnavailable
	}
	if d.Seeding {
		m.mu.Unlock()
		return errSeeding
	}
	infoHash := d.InfoHash
	destination, name := d.Destination, d.Name

	// Removing the record is attempted, and can fail and roll back, before
	// any of the irreversible teardown below (cancelling the job, dropping
	// the engine, deleting files) starts.
	if err := m.dropLocked(id); err != nil {
		m.mu.Unlock()
		return fmt.Errorf("удалить данные загрузки: %w", err)
	}

	eng := m.engines[id]
	delete(m.engines, id)
	job := m.jobs[id]
	if job != nil {
		job.cancel()
	}

	slog.Info("download data deleted", "download_id", id, "name", name)
	emit(eventRemoved, RemovedEvent{ID: id})
	m.schedule()
	started := m.startTeardownLocked(job, eng, infoHash, func() { removeContent(destination, name) })
	m.mu.Unlock()
	if !started {
		slog.Warn("skipped download data teardown, manager is shutting down", "download_id", id)
	}
	return nil
}

func (m *Manager) discard(id string, deleteData bool) error {
	m.mu.Lock()
	d := m.findLocked(id)
	if d == nil {
		m.mu.Unlock()
		return errNotFound
	}
	infoHash := d.InfoHash
	destination, name := d.Destination, d.Name
	purge := deleteData && d.Status != StatusCompleted

	if err := m.dropLocked(id); err != nil {
		m.mu.Unlock()
		return fmt.Errorf("удалить загрузку: %w", err)
	}

	eng := m.engines[id]
	delete(m.engines, id)
	job := m.jobs[id]
	if job != nil {
		job.cancel()
	}

	// Cancelling a finished download only deletes its data: it already reported
	// download_completed, and a second terminal event would double-count it.
	if purge {
		m.recordUsage(usagestats.Event{
			Type:      usagestats.TypeDownloadCancelled,
			Timestamp: time.Now(),
			Properties: usagestats.Properties{
				GameID:          d.Origin.GameID,
				DurationSeconds: int64(time.Since(d.AddedAt).Seconds()),
				BytesTotal:      d.Total,
			},
		})
	}

	if deleteData {
		slog.Info("download cancelled", "download_id", id, "name", name)
	} else {
		slog.Info("download removed", "download_id", id, "name", name)
	}
	emit(eventRemoved, RemovedEvent{ID: id})
	m.schedule()
	started := m.startTeardownLocked(job, eng, infoHash, func() {
		if purge {
			removeContent(destination, name)
		}
	})
	m.mu.Unlock()
	if !started {
		slog.Warn("skipped download teardown, manager is shutting down", "download_id", id)
	}
	return nil
}

// startTeardownLocked runs the disk/engine cleanup that follows a removal in
// a tracked goroutine, so ServiceShutdown's wg.Wait sees it (invariant 19).
// It refuses to start once the manager is closing: a teardown that outlives
// Shutdown could still be deleting a game's files, or dropping a torrent,
// after the process has decided to exit, and on Windows a half-finished
// RemoveAll leaves a locked directory behind that blocks reinstalling.
// Skipping it here is safe because dropLocked already removed the record
// from m.items before this is reached; nothing keeps referring to the
// engine or the files that would otherwise be cleaned up.
func (m *Manager) startTeardownLocked(job *jobState, eng engineTorrent, infoHash string, cleanup func()) bool {
	return m.spawnTrackedLocked(func() {
		if job != nil {
			<-job.done
		}
		if eng != nil {
			eng.drop()
		}
		m.discardMetainfo(infoHash)
		cleanup()
	})
}

func (m *Manager) beginJob(ctx context.Context, id string) (context.Context, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.findLocked(id) == nil || m.jobs[id] != nil {
		return nil, false
	}
	jobCtx, cancel := context.WithCancel(ctx)
	m.jobs[id] = &jobState{cancel: cancel, done: make(chan struct{})}
	return jobCtx, true
}

func (m *Manager) endJob(id string) {
	m.mu.Lock()
	job := m.jobs[id]
	delete(m.jobs, id)
	m.mu.Unlock()
	if job == nil {
		return
	}
	job.cancel()
	close(job.done)
}

// detachEngineLocked gates a torrent off and removes it from the client
// altogether. Leaving it attached with upload merely disallowed would send no
// data, but the torrent keeps announcing itself to the tracker (wantPeers
// counts peers, not seeding), so the user stays listed in the swarm of
// something they turned off. Dropping is the only way out of the swarm.
// detachEngineLocked отпускает движок завершённой загрузки. Дроп идёт
// отдельной горутиной (он ждёт остановки торрента), но учтённой в m.wg:
// иначе ServiceShutdown вернулся бы раньше, чем закрылось хранилище торрента,
// и на Windows остался бы заблокированный каталог. При закрытии менеджера
// дроп не запускается — движок всё равно закроется через cl.close().
func (m *Manager) detachEngineLocked(id string, eng engineTorrent) {
	eng.disallowUpload()
	delete(m.engines, id)
	delete(m.rates, id)
	m.spawnTrackedLocked(eng.drop)
}

// dropLocked removes id from the queue and persists that. On a persist
// failure it re-inserts the item at its original index and returns the
// error, so the caller can decide whether to still tear down the engine/job
// side effects that go with a removal (it must not: those are irreversible).
func (m *Manager) dropLocked(id string) error {
	index := -1
	var removed *Download
	for i, d := range m.items {
		if d.ID == id {
			index, removed = i, d
			break
		}
	}
	if index < 0 {
		return nil
	}
	m.items = append(m.items[:index], m.items[index+1:]...)
	delete(m.rates, id)
	if err := m.persistLocked(); err != nil {
		restored := make([]*Download, 0, len(m.items)+1)
		restored = append(restored, m.items[:index]...)
		restored = append(restored, removed)
		restored = append(restored, m.items[index:]...)
		m.items = restored
		return err
	}
	return nil
}

func (m *Manager) MoveUp(id string) error   { return m.move(id, -1) }
func (m *Manager) MoveDown(id string) error { return m.move(id, 1) }

func (m *Manager) move(id string, step int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	index := -1
	for i, d := range m.items {
		if d.ID == id {
			index = i
			break
		}
	}
	if index < 0 {
		return errNotFound
	}
	if m.items[index].Status != StatusQueued {
		return errUnavailable
	}
	target := index + step
	for target >= 0 && target < len(m.items) && m.items[target].Status != StatusQueued {
		target += step
	}
	if target < 0 || target >= len(m.items) {
		return nil
	}
	m.items[index], m.items[target] = m.items[target], m.items[index]
	if err := m.persistLocked(); err != nil {
		m.items[index], m.items[target] = m.items[target], m.items[index]
		return fmt.Errorf("изменить порядок загрузок: %w", err)
	}
	emit(eventUpdated, snapshot(m.items[index]))
	emit(eventUpdated, snapshot(m.items[target]))
	return nil
}

func (m *Manager) schedule() {
	active := 0
	for _, d := range m.items {
		if occupiesSlot(d.Status) {
			active++
		}
	}
	for _, d := range m.items {
		if active >= m.max {
			return
		}
		if d.Status != StatusQueued {
			continue
		}
		if m.startLocked(d) {
			active++
		}
	}
}

func (m *Manager) startLocked(d *Download) bool {
	eng := m.engines[d.ID]
	if eng == nil {
		return false
	}
	eng.setPriorities(selectionOf(d))
	eng.allowDownload()
	applyUpload(eng, m.config().UploadWhileDownloading)
	d.Status = StatusDownloading
	d.Error = ""
	d.ETASeconds = -1
	d.Stalled = false
	d.StalledSince = nil
	m.rates[d.ID] = newRateState()
	slog.Info("download started", "download_id", d.ID, "name", d.Name)
	emit(eventUpdated, snapshot(d))
	return true
}

func applyUpload(eng engineTorrent, allow bool) {
	if allow {
		eng.allowUpload()
		return
	}
	eng.disallowUpload()
}

func (m *Manager) idleLocked(d *Download, status Status) {
	d.Status = status
	d.DownloadSpeed = 0
	d.UploadSpeed = 0
	d.ETASeconds = -1
	d.Stalled = false
	d.StalledSince = nil
	delete(m.rates, d.ID)
}

func (m *Manager) markFailed(id, message string, cause error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	d := m.findLocked(id)
	if d == nil || d.Status == StatusFailed {
		return
	}
	if eng := m.engines[id]; eng != nil {
		eng.disallowDownload()
		// A failed download is not downloading, so upload-while-downloading
		// no longer covers it; leaving upload on would keep serving data
		// from a torrent the user sees as stopped.
		eng.disallowUpload()
	}
	m.idleLocked(d, StatusFailed)
	d.Error = message
	if err := m.persistLocked(); err != nil {
		// Failed is itself the durable, user-actionable landing state (both
		// Resume and ForceStart accept it); markFailed is reached from
		// several backgrounds paths with no caller of its own, so there is
		// nothing to roll this back to that would be safer than Failed, and
		// persistLocked has already surfaced the write failure through the
		// degraded state and eventDegraded.
		slog.Error("persist failed download", "download_id", id, "error", err)
	}
	errorCode := usagestats.Classify(cause)
	if cause == nil {
		errorCode = usagestats.CodeUnknown
	}
	m.recordUsage(usagestats.Event{
		Type:      usagestats.TypeDownloadFailed,
		Timestamp: time.Now(),
		Properties: usagestats.Properties{
			GameID:          d.Origin.GameID,
			DurationSeconds: int64(time.Since(d.AddedAt).Seconds()),
			BytesTotal:      d.Total,
			ErrorCode:       errorCode,
		},
	})
	emit(eventFailed, snapshot(d))
	emit(eventUpdated, snapshot(d))
	m.schedule()
}

func (m *Manager) tick() {
	defer m.wg.Done()
	ctx := m.ctx
	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			m.sample(ctx, now)
		}
	}
}

func (m *Manager) sample(ctx context.Context, now time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	changed := false
	for _, d := range m.items {
		eng := m.engines[d.ID]
		if eng == nil {
			continue
		}
		if d.Status != StatusDownloading && !seedingCompleted(d) {
			continue
		}
		before := *d
		m.updateLocked(d, eng, now)
		if d.Status == StatusDownloading && d.Total > 0 && d.Downloaded >= d.Total && m.jobs[d.ID] == nil && selectedHashed(d, eng) {
			d.Status = StatusVerifying
			id, dest := d.ID, d.Destination
			files := append([]FileState(nil), d.Files...)
			emit(eventUpdated, snapshot(d))
			m.spawnVerifyLocked(ctx, id, eng, dest, files)
			changed = true
			continue
		}
		if differs(&before, d) {
			emit(eventUpdated, snapshot(d))
			changed = true
		}
	}
	if changed && now.Sub(m.lastPersist) >= persistInterval {
		m.lastPersist = now
		if err := m.persistLocked(); err != nil {
			// Every field this tick touched (Downloaded, speeds, Status) is
			// re-derived from the live engines on the very next tick, 250ms
			// later, regardless of whether this write lands, so there is
			// nothing meaningful to roll back; persistLocked has already
			// raised eventDegraded for the frontend to act on.
			slog.Error("persist download progress", "error", err)
		}
	}
}

func seedingCompleted(d *Download) bool {
	return d.Status == StatusCompleted && d.Seeding
}

func selectedHashed(d *Download, eng engineTorrent) bool {
	hashed := eng.filesHashed()
	for i := range d.Files {
		if !d.Files[i].Selected {
			continue
		}
		if i >= len(hashed) || !hashed[i] {
			return false
		}
	}
	return true
}

func (m *Manager) spawnVerifyLocked(ctx context.Context, id string, eng engineTorrent, dest string, files []FileState) {
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		m.verifyCompletion(ctx, id, eng, dest, files)
	}()
}

func (m *Manager) verifyCompletion(ctx context.Context, id string, eng engineTorrent, dest string, files []FileState) {
	jobCtx, started := m.beginJob(ctx, id)
	if !started {
		return
	}
	defer m.endJob(id)

	err := verifyFilesOnDisk(jobCtx, files, eng.filePaths(dest))
	if err != nil {
		if jobCtx.Err() != nil {
			return
		}
		m.markFailed(id, err.Error(), err)
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	d := m.findLocked(id)
	if d == nil {
		return
	}
	m.completeLocked(d)
}

func (m *Manager) updateLocked(d *Download, eng engineTorrent, now time.Time) {
	bytes := eng.fileBytes()
	var done int64
	for i := range d.Files {
		if i < len(bytes) {
			d.Files[i].BytesDone = bytes[i]
		}
		if d.Files[i].Selected {
			done += d.Files[i].BytesDone
		}
	}
	st := eng.stats()
	r := m.rates[d.ID]
	if r == nil {
		r = newRateState()
		m.rates[d.ID] = r
	}
	r.down.add(now, st.downloaded)
	r.up.add(now, st.uploaded)

	d.Downloaded = done
	d.DownloadSpeed = r.down.rate()
	d.UploadSpeed = r.up.rate()
	d.Seeders = st.seeders
	d.Peers = st.peers
	d.Progress = ratio(done, d.Total)
	d.ETASeconds = etaSeconds(d.Total-done, d.DownloadSpeed)

	updateStallLocked(d, r, done, now)
}

func updateStallLocked(d *Download, r *rateState, done int64, now time.Time) {
	if r.lastChange.IsZero() || done > r.lastDownloaded {
		r.lastDownloaded = done
		r.lastChange = now
		d.Stalled = false
		d.StalledSince = nil
		return
	}
	if d.Status != StatusDownloading || d.Stalled || now.Sub(r.lastChange) < stallAfter {
		return
	}
	since := r.lastChange
	d.Stalled = true
	d.StalledSince = &since
}

func (m *Manager) completeLocked(d *Download) {
	now := time.Now()
	m.idleLocked(d, StatusCompleted)
	d.CompletedAt = &now
	d.Progress = 1
	d.ETASeconds = 0

	seed := m.config().SeedAfterDownload
	d.Seeding = seed
	if eng := m.engines[d.ID]; eng != nil {
		eng.disallowDownload()
		if seed {
			eng.allowUpload()
		} else {
			m.detachEngineLocked(d.ID, eng)
		}
	}
	if err := m.persistLocked(); err != nil {
		// The files are genuinely downloaded and verified by the time this
		// runs (verifyCompletion only calls completeLocked after a clean
		// verify), and the engine above may already have been irreversibly
		// dropped (detachEngineLocked), so rolling d back to Verifying would
		// describe a torrent that no longer exists and strand it with no
		// job left to move it forward. Recording the real outcome and
		// relying on persistLocked's degraded state/event is the honest
		// choice here.
		slog.Error("persist completed download", "download_id", d.ID, "error", err)
	}
	slog.Info("download completed", "download_id", d.ID, "name", d.Name)
	duration := int64(now.Sub(d.AddedAt).Seconds())
	var avgSpeed int64
	if duration > 0 {
		avgSpeed = d.Downloaded / duration
	}
	m.recordUsage(usagestats.Event{
		Type:      usagestats.TypeDownloadCompleted,
		Timestamp: now,
		Properties: usagestats.Properties{
			GameID:            d.Origin.GameID,
			DurationSeconds:   duration,
			BytesTotal:        d.Total,
			AverageSpeedBytes: avgSpeed,
		},
	})
	emit(eventCompleted, snapshot(d))
	emit(eventUpdated, snapshot(d))
	if m.historyRecorder != nil {
		if err := m.historyRecorder(history.Record{
			Kind:       history.KindDownloaded,
			GameID:     d.Origin.GameID,
			Title:      d.Name,
			Bytes:      d.Total,
			BytesKnown: true,
			RefID:      d.ID,
		}); err != nil {
			// completeLocked runs at the end of a background job with no
			// caller to return the error to; Record already flipped the
			// journal into a degraded state and emitted history:degraded,
			// so the user still learns about it, without failing a download
			// that actually completed.
			slog.Error("record download history", "download_id", d.ID, "error", err)
		}
	}
	if m.onCompleted != nil {
		notify, done := m.onCompleted, snapshot(d)
		go notify(done)
	}
	m.schedule()
}

func differs(a, b *Download) bool {
	return a.Status != b.Status ||
		a.Downloaded != b.Downloaded ||
		a.DownloadSpeed != b.DownloadSpeed ||
		a.UploadSpeed != b.UploadSpeed ||
		a.Seeders != b.Seeders ||
		a.Peers != b.Peers ||
		a.Progress != b.Progress ||
		a.Stalled != b.Stalled
}

func (m *Manager) applySettings(next settings.Settings) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.client != nil {
		m.client.applyLimits(next.DownloadRateLimit, next.UploadRateLimit)
	}
	m.max = maxActive(next)
	for _, d := range m.items {
		eng := m.engines[d.ID]
		if d.Status != StatusCompleted {
			// Only a running download is covered by upload-while-downloading;
			// anything queued, paused or failed keeps its engine attached and
			// must be gated off, or the toggle would never reach it.
			if eng != nil {
				applyUpload(eng, next.UploadWhileDownloading && d.Status == StatusDownloading)
			}
			continue
		}
		switch {
		case !next.SeedAfterDownload:
			if d.Seeding {
				d.Seeding = false
				emit(eventUpdated, snapshot(d))
			}
			if eng == nil {
				continue
			}
			// A job in flight owns this engine; gate it off now and let
			// settleRestored detach it when the job lands, rather than
			// dropping the torrent out from under it.
			if m.jobs[d.ID] != nil {
				eng.disallowUpload()
				continue
			}
			m.detachEngineLocked(d.ID, eng)
		case eng != nil:
			if !d.Seeding {
				d.Seeding = true
				eng.allowUpload()
				emit(eventUpdated, snapshot(d))
			}
		default:
			if d.Seeding {
				d.Seeding = false
				emit(eventUpdated, snapshot(d))
			}
			m.reseedLocked(d)
		}
	}
	if err := m.persistLocked(); err != nil {
		// This flushes Seeding flips already applied to the live engines
		// above (and already emitted individually); a settings change has
		// no caller to report a persist failure to, and re-deriving which
		// of possibly many items to roll back is not worth it when
		// persistLocked already raised eventDegraded.
		slog.Error("persist settings-driven seeding change", "error", err)
	}
	m.schedule()
}

func (m *Manager) reseedLocked(d *Download) {
	if m.client == nil || m.ctx == nil || m.closing || m.jobs[d.ID] != nil {
		return
	}
	if !m.store.hasMetainfo(d.InfoHash) && !strings.HasPrefix(d.Source, "magnet:") {
		slog.Warn("cannot reseed download", "download_id", d.ID)
		return
	}
	m.spawnRestoreLocked(restoreJob{
		id:       d.ID,
		infoHash: d.InfoHash,
		source:   d.Source,
		dest:     d.Destination,
		flat:     d.Flat,
		inPlace:  d.InPlace,
		complete: true,
	})
}

type restoreJob struct {
	id       string
	infoHash string
	source   string
	dest     string
	flat     bool
	inPlace  bool
	paused   bool
	complete bool
	seeding  bool
	force    bool
}

func keepsSeeding(j restoreJob, seed bool) bool {
	return j.seeding && seed
}

func (m *Manager) restore() {
	defer m.wg.Done()

	m.mu.Lock()
	ctx := m.ctx
	cl := m.client
	seed := m.config().SeedAfterDownload
	jobs := make([]restoreJob, 0, len(m.items))
	for _, d := range m.items {
		jobs = append(jobs, restoreJob{
			id:       d.ID,
			infoHash: d.InfoHash,
			source:   d.Source,
			dest:     d.Destination,
			flat:     d.Flat,
			inPlace:  d.InPlace,
			paused:   d.Status == StatusPaused,
			complete: d.Status == StatusCompleted,
			seeding:  d.Seeding,
		})
	}
	m.mu.Unlock()
	if cl == nil {
		m.failWithoutClient()
		return
	}

	for _, j := range jobs {
		if ctx.Err() != nil {
			return
		}
		if j.complete && !keepsSeeding(j, seed) {
			m.setSeeding(j.id, false)
			continue
		}
		m.restoreOne(ctx, cl, j)
	}

	m.mu.Lock()
	if err := m.persistLocked(); err != nil {
		// Same reasoning as applySettings: this is the end-of-startup flush
		// for Seeding flips made by setSeeding during the loop above, with
		// no caller waiting on this background pass.
		slog.Error("persist restore pass", "error", err)
	}
	m.schedule()
	m.mu.Unlock()
}

func (m *Manager) failWithoutClient() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, d := range m.items {
		if d.Status == StatusCompleted {
			if d.Seeding {
				d.Seeding = false
				emit(eventUpdated, snapshot(d))
			}
			continue
		}
		if d.Status == StatusFailed || d.Status == StatusPaused {
			continue
		}
		m.idleLocked(d, StatusFailed)
		d.Error = errNoClient.Error()
		m.recordUsage(usagestats.Event{
			Type:      usagestats.TypeDownloadFailed,
			Timestamp: time.Now(),
			Properties: usagestats.Properties{
				GameID:          d.Origin.GameID,
				DurationSeconds: int64(time.Since(d.AddedAt).Seconds()),
				BytesTotal:      d.Total,
				ErrorCode:       usagestats.Classify(errNoClient),
			},
		})
		emit(eventFailed, snapshot(d))
		emit(eventUpdated, snapshot(d))
	}
	if err := m.persistLocked(); err != nil {
		// Same reasoning as markFailed: Failed is the safe landing state for
		// every item this loop touched, and this runs once at startup with
		// no caller to hand the error to.
		slog.Error("persist no-client failure pass", "error", err)
	}
}

func (m *Manager) restoreOne(ctx context.Context, cl *client, j restoreJob) {
	jobCtx, started := m.beginJob(ctx, j.id)
	if !started {
		return
	}
	defer m.endJob(j.id)

	if !m.reserveHashExcept(j.infoHash, j.id) {
		slog.Warn("torrent already attached elsewhere", "download_id", j.id)
		if j.complete {
			m.setSeeding(j.id, false)
			return
		}
		m.markFailed(j.id, errHashBusy.Error(), errHashBusy)
		return
	}
	// settleRestored releases this once it records the engine in m.engines;
	// this defer is only a safety net for the early-return paths below.
	defer m.releaseHash(j.infoHash)

	lt, err := m.reattach(jobCtx, cl, j)
	if err != nil {
		if jobCtx.Err() != nil {
			return
		}
		slog.Error("restore download", "download_id", j.id, "error", err)
		if j.complete {
			m.setSeeding(j.id, false)
			return
		}
		m.markFailed(j.id, errNoRestore.Error(), err)
		return
	}
	m.watchWriteErrors(j.id, lt)
	m.settleRestored(jobCtx, j, lt, lt.t.Info())
}

func (m *Manager) settleRestored(ctx context.Context, j restoreJob, eng engineTorrent, info *metainfo.Info) {
	m.mu.Lock()
	d := m.findLocked(j.id)
	if d == nil {
		m.mu.Unlock()
		eng.drop()
		return
	}
	if len(d.Files) == 0 && info != nil {
		d.Files = fileStates(info, nil)
		d.Total = selectedTotal(d.Files)
	}
	m.engines[j.id] = eng
	// The engine is now recorded, which is itself enough for
	// hashBusyLocked/hashInUseLocked to see this hash as taken, so the
	// reservation restoreOne (or AddTask's spawnSettleLocked) took can be
	// released here; j.infoHash is empty for the AddTask path, and deleting
	// an absent map key is a no-op.
	m.releaseHashLocked(j.infoHash)
	eng.setPriorities(selectionOf(d))
	if j.complete {
		// The setting can have been turned off while this job was running,
		// which is exactly the window applySettings hands over to us.
		seed := m.config().SeedAfterDownload
		d.Seeding = seed
		if seed {
			eng.allowUpload()
		} else {
			m.detachEngineLocked(j.id, eng)
		}
		emit(eventUpdated, snapshot(d))
		if err := m.persistLocked(); err != nil {
			// This branch only flips the Seeding flag to match the engine
			// state and config that were already applied above; there is no
			// earlier, still-actionable status to roll back to, and no
			// caller to return the error to. persistLocked already flipped
			// the manager into a degraded state and emitted eventDegraded.
			slog.Error("persist restored seeding state", "download_id", j.id, "error", err)
		}
		m.mu.Unlock()
		return
	}
	d.Status = StatusVerifying
	emit(eventUpdated, snapshot(d))
	m.mu.Unlock()

	if err := eng.verify(ctx); err != nil {
		if ctx.Err() != nil {
			return
		}
		slog.Warn("verify download", "download_id", j.id, "error", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	d = m.findLocked(j.id)
	if d == nil {
		return
	}
	m.updateLocked(d, eng, time.Now())
	switch {
	case j.paused:
		eng.disallowDownload()
		eng.disallowUpload()
		m.idleLocked(d, StatusPaused)
		emit(eventUpdated, snapshot(d))
	case j.force:
		d.Status = StatusQueued
		if !m.startLocked(d) {
			emit(eventUpdated, snapshot(d))
		}
	default:
		d.Status = StatusQueued
		emit(eventUpdated, snapshot(d))
	}
	if err := m.persistLocked(); err != nil {
		// Paused/Queued/Downloading are all stable, user-actionable states,
		// unlike the StatusVerifying this job started from; rolling back to
		// Verifying here would strand the download in a status neither
		// Resume nor ForceStart accept, with no job left to move it forward
		// until the next restart. persistLocked has already surfaced the
		// failure via the degraded state and eventDegraded.
		slog.Error("persist restored download", "download_id", j.id, "error", err)
	}
	m.schedule()
}

func (m *Manager) reattach(ctx context.Context, cl *client, j restoreJob) (*liveTorrent, error) {
	opts := storageOpts{flat: j.flat, inPlace: j.inPlace}
	if mi, err := m.store.loadMetainfo(j.infoHash); err == nil {
		return cl.addMetainfo(mi, j.dest, opts)
	}
	if !strings.HasPrefix(j.source, "magnet:") {
		return nil, errors.New("metainfo unavailable")
	}

	m.setStatus(j.id, StatusMetadata)
	lt, err := cl.addMagnet(j.source, j.dest, opts)
	if err != nil {
		return nil, err
	}
	select {
	case <-lt.t.GotInfo():
	case <-ctx.Done():
		lt.drop()
		return nil, ctx.Err()
	case <-time.After(metadataTimeout):
		lt.drop()
		return nil, errNoMetadata
	}
	if err := validateInfo(lt.t.Info()); err != nil {
		lt.drop()
		return nil, err
	}

	m.mu.Lock()
	stillTracked := m.findLocked(j.id) != nil
	m.mu.Unlock()
	if !stillTracked {
		lt.drop()
		return nil, errNotFound
	}

	mi := lt.t.Metainfo()
	if err := m.store.saveMetainfo(j.infoHash, &mi); err != nil {
		slog.Warn("save metainfo", "download_id", j.id, "error", err)
	}
	return lt, nil
}

func (m *Manager) setStatus(id string, status Status) {
	m.mu.Lock()
	defer m.mu.Unlock()
	d := m.findLocked(id)
	if d == nil {
		return
	}
	d.Status = status
	emit(eventUpdated, snapshot(d))
}

func (m *Manager) setSeeding(id string, seeding bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	d := m.findLocked(id)
	if d == nil || d.Seeding == seeding {
		return
	}
	d.Seeding = seeding
	emit(eventUpdated, snapshot(d))
}

func removeContent(destination, name string) {
	if destination == "" || !isSafeTorrentPath(name) {
		return
	}
	root := filepath.Join(destination, name)
	for _, path := range []string{root, root + PartFileSuffix} {
		if err := os.RemoveAll(path); err != nil {
			slog.Warn("remove download data", "path", path, "error", err)
		}
	}
}

func requiredBytes(files []FileState) (int64, error) {
	var total int64
	for _, f := range files {
		if !f.Selected {
			continue
		}
		if f.Size < 0 || total > math.MaxInt64-f.Size {
			return 0, errBadSizes
		}
		total += f.Size
	}
	return total, nil
}

func checkFreeSpace(destination string, needed int64) error {
	if needed < 0 {
		return errBadSizes
	}
	st, err := platform.GetStorageInfo(destination)
	if err != nil {
		slog.Error("storage info", "path", destination, "error", err)
		return fmt.Errorf("%w: %w", errNoFreeSpace, err)
	}
	//nolint:gosec // G115: needed >= 0 проверено выше, конверсия int64 -> uint64 точная
	if st.FreeBytes >= uint64(needed) {
		return nil
	}
	//nolint:gosec // G115: в этой ветке FreeBytes < needed <= MaxInt64
	return fmt.Errorf("%w: нужно %s, свободно %s",
		errNotEnoughSpace, humanSize(needed), humanSize(int64(st.FreeBytes)))
}

func humanSize(bytes int64) string {
	switch {
	case bytes >= 1<<30:
		return fmt.Sprintf("%.1f ГБ", float64(bytes)/(1<<30))
	case bytes >= 1<<20:
		return fmt.Sprintf("%.1f МБ", float64(bytes)/(1<<20))
	case bytes >= 1<<10:
		return fmt.Sprintf("%.1f КБ", float64(bytes)/(1<<10))
	default:
		return fmt.Sprintf("%d Б", bytes)
	}
}

func newID() string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("d%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
}
