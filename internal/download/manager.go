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
	"typhon/internal/platform"
	"typhon/internal/redact"
	"typhon/internal/settings"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/wailsapp/wails/v3/pkg/application"
)

const (
	eventAdded     = "download:added"
	eventUpdated   = "download:updated"
	eventCompleted = "download:completed"
	eventFailed    = "download:failed"
	eventRemoved   = "download:removed"

	metadataTimeout = 90 * time.Second
	tickInterval    = 250 * time.Millisecond
	persistInterval = 5 * time.Second
)

const restoreFailedMessage = "не удалось восстановить загрузку"

var ErrNotFound = errors.New("загрузка не найдена")

var (
	errNotFound    = ErrNotFound
	errUnavailable = errors.New("недоступно для этой загрузки")
	errNoClient    = errors.New("торрент-клиент недоступен")
	errNoMetadata  = errors.New("не удалось получить метаданные торрента")
	errNoRestore   = errors.New(restoreFailedMessage)
	errSeeding     = errors.New("файлы сейчас раздаются — сначала остановите раздачу")
	errBadSizes    = errors.New("недопустимые размеры файлов в торренте")
	errNoFreeSpace = errors.New("не удалось определить свободное место на диске")
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
	mu       sync.Mutex
	settings *settings.Service
	store    *store
	metaDir  string

	items   []*Download
	engines map[string]engineTorrent
	rates   map[string]*rateState
	pending map[string]*pending
	jobs    map[string]*jobState

	client      *client
	max         int
	onCompleted func(Download)

	ctx         context.Context
	cancel      context.CancelFunc
	closing     bool
	wg          sync.WaitGroup
	unsubscribe func()
	lastPersist time.Time
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
	}
	m.metaDir = filepath.Join(dir, "meta")
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
	cl, err := newClient(cfg, m.metaDir)
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
	m.persistLocked()
	cl := m.client
	m.client = nil
	m.mu.Unlock()

	if cl != nil {
		cl.close()
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

func (m *Manager) persistLocked() {
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
		slog.Error("persist downloads", "error", err)
	}
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
		return TorrentInfo{}, errors.New("укажите magnet-ссылку или torrent-файл")
	}

	m.mu.Lock()
	cl := m.client
	ctx := m.ctx
	m.mu.Unlock()
	if cl == nil {
		return TorrentInfo{}, errNoClient
	}
	if ctx == nil {
		ctx = context.Background()
	}

	spec, err := buildSpec(source)
	if err != nil {
		slog.Error("parse torrent source", "operation", "fetch_metadata", "source", redact.Source(source), "error", err)
		return TorrentInfo{}, err
	}
	infoHash := spec.InfoHash.HexString()

	m.mu.Lock()
	if m.findByHashLocked(infoHash) != nil {
		m.mu.Unlock()
		return TorrentInfo{}, errors.New("эта загрузка уже добавлена")
	}
	if existing, ok := m.pending[infoHash]; ok {
		m.mu.Unlock()
		info := existing.torrent.t.Info()
		if info != nil {
			return torrentInfoOf(infoHash, info), nil
		}
		return TorrentInfo{}, errNoMetadata
	}
	m.mu.Unlock()

	lt, err := cl.add(spec, cl.metaDir, storageOpts{})
	if err != nil {
		slog.Error("add torrent for metadata", "operation", "fetch_metadata", "source", redact.Source(source), "error", err)
		return TorrentInfo{}, errors.New("не удалось добавить торрент")
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
	m.mu.Unlock()

	return torrentInfoOf(infoHash, info), nil
}

func buildSpec(source string) (*torrent.TorrentSpec, error) {
	if strings.HasPrefix(source, "magnet:") {
		spec, err := magnetSpec(source)
		if err != nil {
			return nil, errors.New("некорректная magnet-ссылка")
		}
		return spec, nil
	}
	mi, err := metainfo.LoadFromFile(source)
	if err != nil {
		return nil, errors.New("не удалось прочитать torrent-файл")
	}
	spec, err := torrent.TorrentSpecFromMetaInfoErr(mi)
	if err != nil || spec.InfoHash.IsZero() {
		return nil, errors.New("не удалось прочитать torrent-файл")
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

func (m *Manager) returnPending(infoHash string, p *pending) {
	m.mu.Lock()
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

func (m *Manager) StartDownloadFrom(infoHash, destination string, selectedIndices []int, origin Origin) (Download, error) {
	m.mu.Lock()
	p := m.pending[infoHash]
	delete(m.pending, infoHash)
	cl := m.client
	m.mu.Unlock()
	if p == nil {
		return Download{}, errors.New("сначала получите метаданные торрента")
	}
	if cl == nil {
		m.returnPending(infoHash, p)
		return Download{}, errNoClient
	}

	destination = strings.TrimSpace(destination)
	if destination == "" {
		m.returnPending(infoHash, p)
		return Download{}, errors.New("укажите папку назначения")
	}
	if err := os.MkdirAll(destination, 0o755); err != nil {
		m.returnPending(infoHash, p)
		slog.Error("create destination", "path", destination, "error", err)
		if errors.Is(err, fs.ErrPermission) {
			return Download{}, errors.New("нет доступа к папке назначения")
		}
		return Download{}, errors.New("папка назначения недоступна")
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
		return Download{}, errors.New("не выбрано ни одного файла")
	}
	if err := checkFreeSpace(destination, needed); err != nil {
		m.returnPending(infoHash, p)
		return Download{}, err
	}

	mi := p.torrent.t.Metainfo()
	p.torrent.drop()

	lt, err := cl.addMetainfo(&mi, destination, storageOpts{})
	if err != nil {
		slog.Error("add torrent", "operation", "start_download", "error", err)
		return Download{}, errors.New("не удалось добавить торрент")
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
	defer m.mu.Unlock()
	m.items = append(m.items, d)
	m.engines[d.ID] = lt
	if err := m.store.saveMetainfo(infoHash, &mi); err != nil {
		slog.Warn("save metainfo", "download_id", d.ID, "error", err)
	}
	m.persistLocked()
	slog.Info("download added", "download_id", d.ID, "name", d.Name)
	emit(eventAdded, snapshot(d))
	m.schedule()
	return snapshot(d), nil
}

func (m *Manager) watchWriteErrors(id string, lt *liveTorrent) {
	lt.t.SetOnWriteChunkError(func(err error) {
		slog.Error("torrent write error", "download_id", id, "error", err)
		go m.markFailed(id, "ошибка записи на диск")
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
	if eng := m.engines[id]; eng != nil {
		eng.disallowDownload()
		eng.disallowUpload()
	}
	m.idleLocked(d, StatusPaused)
	m.persistLocked()
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
	d.Status = StatusQueued
	d.Error = ""
	m.persistLocked()
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
	d.Status = StatusQueued
	d.Error = ""
	if !m.startLocked(d) {
		emit(eventUpdated, snapshot(d))
	}
	m.persistLocked()
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
	d.Status = StatusVerifying
	d.Error = ""
	m.persistLocked()
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
	eng := m.engines[id]
	delete(m.engines, id)

	job := m.jobs[id]
	if job != nil {
		job.cancel()
	}

	infoHash := d.InfoHash
	destination, name := d.Destination, d.Name

	m.dropLocked(id)
	slog.Info("download data deleted", "download_id", id, "name", name)
	emit(eventRemoved, RemovedEvent{ID: id})
	m.schedule()
	m.mu.Unlock()

	go func() {
		if job != nil {
			<-job.done
		}
		if eng != nil {
			eng.drop()
		}
		m.discardMetainfo(infoHash)
		removeContent(destination, name)
	}()
	return nil
}

func (m *Manager) discard(id string, deleteData bool) error {
	m.mu.Lock()
	d := m.findLocked(id)
	if d == nil {
		m.mu.Unlock()
		return errNotFound
	}
	eng := m.engines[id]
	delete(m.engines, id)

	job := m.jobs[id]
	if job != nil {
		job.cancel()
	}

	infoHash := d.InfoHash
	destination, name := d.Destination, d.Name
	purge := deleteData && d.Status != StatusCompleted

	m.dropLocked(id)
	if deleteData {
		slog.Info("download cancelled", "download_id", id, "name", name)
	} else {
		slog.Info("download removed", "download_id", id, "name", name)
	}
	emit(eventRemoved, RemovedEvent{ID: id})
	m.schedule()
	m.mu.Unlock()

	go func() {
		if job != nil {
			<-job.done
		}
		if eng != nil {
			eng.drop()
		}
		m.discardMetainfo(infoHash)
		if purge {
			removeContent(destination, name)
		}
	}()
	return nil
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

func (m *Manager) dropLocked(id string) {
	for i, d := range m.items {
		if d.ID == id {
			m.items = append(m.items[:i], m.items[i+1:]...)
			break
		}
	}
	delete(m.rates, id)
	m.persistLocked()
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
	m.persistLocked()
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
	delete(m.rates, d.ID)
}

func (m *Manager) markFailed(id, message string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	d := m.findLocked(id)
	if d == nil || d.Status == StatusFailed {
		return
	}
	if eng := m.engines[id]; eng != nil {
		eng.disallowDownload()
	}
	m.idleLocked(d, StatusFailed)
	d.Error = message
	m.persistLocked()
	emit(eventFailed, snapshot(d))
	emit(eventUpdated, snapshot(d))
	m.schedule()
}

func (m *Manager) tick() {
	defer m.wg.Done()
	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()
	for {
		select {
		case <-m.ctx.Done():
			return
		case now := <-ticker.C:
			m.sample(now)
		}
	}
}

func (m *Manager) sample(now time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	changed := false
	for _, d := range m.items {
		eng := m.engines[d.ID]
		if eng == nil {
			continue
		}
		if d.Status != StatusDownloading && !(d.Status == StatusCompleted && d.Seeding) {
			continue
		}
		before := *d
		m.updateLocked(d, eng, now)
		if d.Status == StatusDownloading && d.Total > 0 && d.Downloaded >= d.Total {
			m.completeLocked(d)
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
		m.persistLocked()
	}
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
			eng.disallowUpload()
			delete(m.engines, d.ID)
			delete(m.rates, d.ID)
			go eng.drop()
		}
	}
	m.persistLocked()
	slog.Info("download completed", "download_id", d.ID, "name", d.Name)
	emit(eventCompleted, snapshot(d))
	emit(eventUpdated, snapshot(d))
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
		a.Progress != b.Progress
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
			if eng != nil && d.Status == StatusDownloading {
				applyUpload(eng, next.UploadWhileDownloading)
			}
			continue
		}
		switch {
		case !next.SeedAfterDownload:
			if eng != nil {
				eng.disallowUpload()
			}
			if d.Seeding {
				d.Seeding = false
				emit(eventUpdated, snapshot(d))
			}
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
	m.persistLocked()
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
		if j.complete && !(j.seeding && seed) {
			m.setSeeding(j.id, false)
			continue
		}
		m.restoreOne(ctx, cl, j)
	}

	m.mu.Lock()
	m.persistLocked()
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
		emit(eventFailed, snapshot(d))
		emit(eventUpdated, snapshot(d))
	}
	m.persistLocked()
}

func (m *Manager) restoreOne(ctx context.Context, cl *client, j restoreJob) {
	jobCtx, started := m.beginJob(ctx, j.id)
	if !started {
		return
	}
	defer m.endJob(j.id)

	if m.hashInUse(j.infoHash, j.id) {
		slog.Warn("torrent already attached elsewhere", "download_id", j.id)
		if j.complete {
			m.setSeeding(j.id, false)
			return
		}
		m.markFailed(j.id, errHashBusy.Error())
		return
	}

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
		m.markFailed(j.id, restoreFailedMessage)
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
	eng.setPriorities(selectionOf(d))
	if j.complete {
		seed := m.config().SeedAfterDownload
		d.Seeding = seed
		applyUpload(eng, seed)
		emit(eventUpdated, snapshot(d))
		m.persistLocked()
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
	m.persistLocked()
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
	for _, path := range []string{root, root + ".part"} {
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
	return fmt.Errorf("недостаточно места на диске: нужно %s, свободно %s",
		humanSize(needed), humanSize(int64(st.FreeBytes)))
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
