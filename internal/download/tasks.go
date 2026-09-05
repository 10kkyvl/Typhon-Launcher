package download

import (
	"context"
	"errors"
	"fmt"
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
	if !m.reserveHash(infoHash) {
		return Download{}, errDuplicateTask
	}
	// Ownership of the reservation moves to spawnSettleLocked's goroutine
	// when req.Verify is set (its engine is not recorded in m.engines until
	// settleRestored runs); every other exit below releases it directly.
	transferred := false
	defer func() {
		if !transferred {
			m.releaseHash(infoHash)
		}
	}()

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
		return Download{}, fmt.Errorf("не удалось добавить торрент: %w", err)
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
	if err := m.persistLocked(); err != nil {
		m.items = m.items[:len(m.items)-1]
		if !req.Verify {
			delete(m.engines, d.ID)
		}
		m.mu.Unlock()
		lt.drop()
		return Download{}, fmt.Errorf("добавить загрузку: %w", err)
	}
	m.recordUsage(usagestats.Event{
		Type:      usagestats.TypeDownloadStarted,
		Timestamp: time.Now(),
		Properties: usagestats.Properties{
			GameID: d.Origin.GameID,
		},
	})
	snap := snapshot(d)
	if req.Verify {
		if err := m.spawnSettleLocked(d.ID, infoHash, lt); err != nil {
			m.items = m.items[:len(m.items)-1]
			if perr := m.persistLocked(); perr != nil {
				slog.Error("persist download rollback", "download_id", d.ID, "error", perr)
			}
			m.mu.Unlock()
			lt.drop()
			return Download{}, fmt.Errorf("добавить загрузку: %w", err)
		}
		transferred = true
	} else {
		transferred = true
		delete(m.reserved, infoHash)
		m.schedule()
	}
	m.mu.Unlock()

	slog.Info("download task added", "download_id", d.ID, "name", d.Name,
		"purpose", req.Origin.Purpose, "inPlace", req.InPlace)
	emit(eventAdded, snap)
	return snap, nil
}

// spawnSettleLocked hands a freshly added torrent to a background job that
// verifies it before the download joins the schedule. It takes over the
// infohash reservation the caller holds (released once beginJob resolves,
// since m.jobs[id] or the item's removal then covers hashBusyLocked on its
// own) and returns an error instead of starting the goroutine if the manager
// has no live ctx to run it under (invariant 20: no context.Background
// fallback in a service).
func (m *Manager) spawnSettleLocked(id, infoHash string, lt *liveTorrent) error {
	ctx := m.ctx
	if ctx == nil {
		return errNoClient
	}
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		defer m.releaseHash(infoHash)
		jobCtx, started := m.beginJob(ctx, id)
		if !started {
			lt.drop()
			return
		}
		defer m.endJob(id)
		m.settleRestored(jobCtx, restoreJob{id: id, infoHash: infoHash}, lt, lt.t.Info())
	}()
	return nil
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
