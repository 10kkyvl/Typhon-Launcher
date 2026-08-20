package download

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"typhon/internal/platform"
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

var (
	errNotFound    = errors.New("загрузка не найдена")
	errUnavailable = errors.New("недоступно для этой загрузки")
	errNoClient    = errors.New("торрент-клиент недоступен")
	errNoMetadata  = errors.New("не удалось получить метаданные торрента")
)

type pending struct {
	torrent *liveTorrent
	source  string
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

	client *client
	max    int

	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
	unsubscribe func()
	lastPersist time.Time
}

func NewManager(settingsService *settings.Service) *Manager {
	dir, err := settings.ConfigDir()
	if err != nil {
		slog.Error("resolve config dir", "error", err)
		dir = ""
	}
	return newManagerAt(dir, settingsService)
}

func newManagerAt(dir string, settingsService *settings.Service) *Manager {
	m := &Manager{
		settings: settingsService,
		store:    newStore(dir),
		engines:  map[string]engineTorrent{},
		rates:    map[string]*rateState{},
		pending:  map[string]*pending{},
	}
	if dir != "" {
		m.metaDir = filepath.Join(dir, "meta")
	}
	m.max = maxActive(m.config())
	return m
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
	cl, err := newClient(cfg, m.metaDir)
	if err != nil {
		slog.Error("start torrent client", "error", err)
	} else {
		m.client = cl
	}
	m.loadLocked()
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

func (m *Manager) loadLocked() {
	for _, r := range m.store.load() {
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
		slog.Error("parse torrent source", "source", source, "error", err)
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

	lt, err := cl.add(spec, cl.metaDir)
	if err != nil {
		slog.Error("add torrent for metadata", "source", source, "error", err)
		return TorrentInfo{}, errors.New("не удалось добавить торрент")
	}

	select {
	case <-lt.t.GotInfo():
	case <-ctx.Done():
		lt.drop()
		return TorrentInfo{}, errNoMetadata
	case <-time.After(metadataTimeout):
		lt.drop()
		slog.Warn("metadata timeout", "source", source, "infoHash", infoHash)
		return TorrentInfo{}, errNoMetadata
	}

	info := lt.t.Info()
	if err := validateInfo(info); err != nil {
		lt.drop()
		slog.Warn("unsafe torrent paths", "infoHash", infoHash)
		return TorrentInfo{}, err
	}

	mi := lt.t.Metainfo()
	if err := m.store.saveMetainfo(infoHash, &mi); err != nil {
		slog.Warn("save metainfo", "infoHash", infoHash, "error", err)
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
}

func (m *Manager) StartDownload(infoHash, destination string, selectedIndices []int) (Download, error) {
	m.mu.Lock()
	p := m.pending[infoHash]
	cl := m.client
	m.mu.Unlock()
	if p == nil || cl == nil {
		return Download{}, errors.New("сначала получите метаданные торрента")
	}

	destination = strings.TrimSpace(destination)
	if destination == "" {
		return Download{}, errors.New("укажите папку назначения")
	}
	if err := os.MkdirAll(destination, 0o755); err != nil {
		slog.Error("create destination", "path", destination, "error", err)
		if errors.Is(err, fs.ErrPermission) {
			return Download{}, errors.New("нет доступа к папке назначения")
		}
		return Download{}, errors.New("папка назначения недоступна")
	}

	info := p.torrent.t.Info()
	files := fileStates(info, selectedIndices)
	needed := selectedTotal(files)
	if needed == 0 {
		return Download{}, errors.New("не выбрано ни одного файла")
	}
	if st, err := platform.GetStorageInfo(destination); err == nil && st.FreeBytes < uint64(needed) {
		return Download{}, fmt.Errorf("недостаточно места на диске: нужно %s, свободно %s",
			humanSize(needed), humanSize(int64(st.FreeBytes)))
	}

	mi := p.torrent.t.Metainfo()
	p.torrent.drop()
	m.mu.Lock()
	delete(m.pending, infoHash)
	m.mu.Unlock()

	lt, err := cl.addMetainfo(&mi, destination)
	if err != nil {
		slog.Error("add torrent", "infoHash", infoHash, "error", err)
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
		AddedAt:     time.Now(),
	}
	lt.setPriorities(selectionOf(d))
	m.watchWriteErrors(d.ID, lt)

	m.mu.Lock()
	defer m.mu.Unlock()
	m.items = append(m.items, d)
	m.engines[d.ID] = lt
	m.persistLocked()
	slog.Info("download added", "id", d.ID, "name", d.Name, "infoHash", infoHash)
	emit(eventAdded, snapshot(d))
	m.schedule()
	return snapshot(d), nil
}

func (m *Manager) watchWriteErrors(id string, lt *liveTorrent) {
	lt.t.SetOnWriteChunkError(func(err error) {
		slog.Error("torrent write error", "id", id, "error", err)
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
	d.Status = StatusQueued
	d.Error = ""
	if !m.startLocked(d) {
		emit(eventUpdated, snapshot(d))
	}
	m.persistLocked()
	return nil
}

func (m *Manager) Cancel(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	d := m.findLocked(id)
	if d == nil {
		return errNotFound
	}
	if eng := m.engines[id]; eng != nil {
		eng.drop()
		delete(m.engines, id)
	}
	if d.Status != StatusCompleted {
		removeContent(d)
	}
	m.store.removeMetainfo(d.InfoHash)
	m.dropLocked(id)
	slog.Info("download cancelled", "id", id, "name", d.Name)
	emit(eventRemoved, RemovedEvent{ID: id})
	m.schedule()
	return nil
}

func (m *Manager) Remove(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	d := m.findLocked(id)
	if d == nil {
		return errNotFound
	}
	if eng := m.engines[id]; eng != nil {
		eng.drop()
		delete(m.engines, id)
	}
	m.store.removeMetainfo(d.InfoHash)
	m.dropLocked(id)
	slog.Info("download removed", "id", id, "name", d.Name)
	emit(eventRemoved, RemovedEvent{ID: id})
	m.schedule()
	return nil
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
	eng.allowUpload()
	d.Status = StatusDownloading
	d.Error = ""
	d.ETASeconds = -1
	m.rates[d.ID] = newRateState()
	slog.Info("download started", "id", d.ID, "name", d.Name)
	emit(eventUpdated, snapshot(d))
	return true
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
		}
	}
	m.persistLocked()
	slog.Info("download completed", "id", d.ID, "name", d.Name)
	emit(eventCompleted, snapshot(d))
	emit(eventUpdated, snapshot(d))
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
		if d.Status != StatusCompleted || d.Seeding == next.SeedAfterDownload {
			continue
		}
		d.Seeding = next.SeedAfterDownload
		if eng := m.engines[d.ID]; eng != nil {
			if d.Seeding {
				eng.allowUpload()
			} else {
				eng.disallowUpload()
			}
		}
		emit(eventUpdated, snapshot(d))
	}
	m.persistLocked()
	m.schedule()
}

type restoreJob struct {
	id       string
	infoHash string
	source   string
	dest     string
	paused   bool
	complete bool
	seeding  bool
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
			paused:   d.Status == StatusPaused,
			complete: d.Status == StatusCompleted,
			seeding:  d.Seeding,
		})
	}
	m.mu.Unlock()
	if cl == nil {
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

func (m *Manager) restoreOne(ctx context.Context, cl *client, j restoreJob) {
	lt, err := m.reattach(ctx, cl, j)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		slog.Error("restore download", "id", j.id, "infoHash", j.infoHash, "error", err)
		m.markFailed(j.id, "не удалось восстановить загрузку")
		return
	}

	m.mu.Lock()
	d := m.findLocked(j.id)
	if d == nil {
		m.mu.Unlock()
		lt.drop()
		return
	}
	if len(d.Files) == 0 {
		if info := lt.t.Info(); info != nil {
			d.Files = fileStates(info, nil)
			d.Total = selectedTotal(d.Files)
		}
	}
	m.engines[j.id] = lt
	lt.setPriorities(selectionOf(d))
	if j.complete {
		d.Seeding = true
		lt.allowUpload()
		emit(eventUpdated, snapshot(d))
		m.mu.Unlock()
		return
	}
	d.Status = StatusVerifying
	emit(eventUpdated, snapshot(d))
	m.mu.Unlock()

	m.watchWriteErrors(j.id, lt)
	if err := lt.verify(ctx); err != nil {
		if ctx.Err() != nil {
			return
		}
		slog.Warn("verify download", "id", j.id, "error", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	d = m.findLocked(j.id)
	if d == nil {
		return
	}
	m.updateLocked(d, lt, time.Now())
	if j.paused {
		lt.disallowDownload()
		lt.disallowUpload()
		m.idleLocked(d, StatusPaused)
	} else {
		d.Status = StatusQueued
	}
	emit(eventUpdated, snapshot(d))
	m.persistLocked()
	m.schedule()
}

func (m *Manager) reattach(ctx context.Context, cl *client, j restoreJob) (*liveTorrent, error) {
	if mi, err := m.store.loadMetainfo(j.infoHash); err == nil {
		return cl.addMetainfo(mi, j.dest)
	}
	if !strings.HasPrefix(j.source, "magnet:") {
		return nil, errors.New("metainfo unavailable")
	}

	m.setStatus(j.id, StatusMetadata)
	lt, err := cl.addMagnet(j.source, j.dest)
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
	mi := lt.t.Metainfo()
	if err := m.store.saveMetainfo(j.infoHash, &mi); err != nil {
		slog.Warn("save metainfo", "infoHash", j.infoHash, "error", err)
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

func removeContent(d *Download) {
	if d.Destination == "" || !isSafeTorrentPath(d.Name) {
		return
	}
	root := filepath.Join(d.Destination, d.Name)
	for _, path := range []string{root, root + ".part"} {
		if err := os.RemoveAll(path); err != nil {
			slog.Warn("remove download data", "path", path, "error", err)
		}
	}
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
