package download

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"strings"
	"time"

	"typhon/internal/usagestats"
)

var errDuplicateTask = errors.New("такая загрузка уже добавлена")

type AddRequest struct {
	Source      string `json:"source"`
	InfoHash    string `json:"infoHash"`
	Destination string `json:"destination"`
	Name        string `json:"name"`
	Origin      Origin `json:"origin"`
	Flat        bool   `json:"flat"`
	InPlace     bool   `json:"inPlace"`
	Verify      bool   `json:"verify"`
}

// AddTask registers a download without going through the interactive metadata
// step. Updates and repairs reuse the regular queue through it.
//
//wails:ignore
func (m *Manager) AddTask(ctx context.Context, req AddRequest) (Download, error) {
	cl, base, err := m.engine()
	if err != nil {
		return Download{}, err
	}
	if ctx == nil {
		ctx = base
	}
	destination := strings.TrimSpace(req.Destination)
	if destination == "" {
		return Download{}, errors.New("укажите папку назначения")
	}
	if err := os.MkdirAll(destination, 0o755); err != nil {
		slog.Error("create destination", "path", destination, "error", err)
		return Download{}, errors.New("папка назначения недоступна")
	}

	mi, err := m.metainfoFor(ctx, cl, req.Source, req.InfoHash)
	if err != nil {
		return Download{}, err
	}
	info, err := mi.UnmarshalInfo()
	if err != nil {
		return Download{}, errNoMetadata
	}
	if err := validateInfo(&info); err != nil {
		return Download{}, err
	}
	infoHash := mi.HashInfoBytes().HexString()
	if m.hashBusy(infoHash) {
		return Download{}, errDuplicateTask
	}

	files := fileStates(&info, nil)
	needed, err := requiredBytes(files)
	if err != nil {
		return Download{}, err
	}
	if !req.InPlace {
		if err := checkFreeSpace(destination, needed); err != nil {
			return Download{}, err
		}
	}

	opts := storageOpts{flat: req.Flat, inPlace: req.InPlace}
	lt, err := cl.addMetainfo(mi, destination, opts)
	if err != nil {
		slog.Error("add torrent", "operation", "start_task", "error", err)
		return Download{}, errors.New("не удалось добавить торрент")
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = info.BestName()
	}
	d := &Download{
		ID:          newID(),
		Name:        name,
		Type:        TypeTorrent,
		Source:      req.Source,
		InfoHash:    infoHash,
		Destination: destination,
		Status:      StatusQueued,
		Total:       needed,
		ETASeconds:  -1,
		Files:       files,
		Flat:        req.Flat,
		InPlace:     req.InPlace,
		Origin:      req.Origin,
		AddedAt:     time.Now(),
	}
	lt.setPriorities(selectionOf(d))
	m.watchWriteErrors(d.ID, lt)

	m.mu.Lock()
	if m.closing {
		m.mu.Unlock()
		lt.drop()
		return Download{}, errNoClient
	}
	m.items = append(m.items, d)
	if err := m.store.saveMetainfo(infoHash, mi); err != nil {
		slog.Warn("save metainfo", "download_id", d.ID, "error", err)
	}
	if !req.Verify {
		m.engines[d.ID] = lt
	}
	m.persistLocked()
	m.recordUsage(usagestats.Event{
		Type:      usagestats.TypeDownloadStarted,
		Timestamp: time.Now(),
		Properties: usagestats.Properties{
			GameID: d.Origin.GameID,
		},
	})
	snap := snapshot(d)
	if req.Verify {
		m.spawnSettleLocked(d.ID, lt)
	} else {
		m.schedule()
	}
	m.mu.Unlock()

	slog.Info("download task added", "download_id", d.ID, "name", d.Name,
		"purpose", req.Origin.Purpose, "inPlace", req.InPlace)
	emit(eventAdded, snap)
	return snap, nil
}

func (m *Manager) spawnSettleLocked(id string, lt *liveTorrent) {
	ctx := m.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		jobCtx, started := m.beginJob(ctx, id)
		if !started {
			lt.drop()
			return
		}
		defer m.endJob(id)
		m.settleRestored(jobCtx, restoreJob{id: id}, lt, lt.t.Info())
	}()
}

// ByOrigin returns the downloads started for a given game and purpose.
//
//wails:ignore
func (m *Manager) ByOrigin(gameID string, purpose Purpose) []Download {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Download, 0, 2)
	for _, d := range m.items {
		if d.Origin.Purpose == purpose && (d.Origin.LibraryID == gameID || d.Origin.GameID == gameID) {
			out = append(out, snapshot(d))
		}
	}
	return out
}
