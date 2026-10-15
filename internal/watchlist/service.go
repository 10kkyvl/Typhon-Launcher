package watchlist

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"typhon/internal/settings"
	"typhon/internal/sources"
	"typhon/internal/storage"
	"typhon/internal/uierr"

	"github.com/wailsapp/wails/v3/pkg/application"
)

const watchlistVersion = 1

const maxAlerts = 200

const (
	eventAlert    = "watchlist:alert"
	eventUpdated  = "watchlist:updated"
	eventDegraded = "watchlist:degraded"
)

var (
	errEmptyID      = uierr.New("watchlist.empty_id", "не указана игра")
	errGameNotFound = uierr.New("watchlist.game_not_found", "игра не найдена в каталоге")
	errNotWatched   = uierr.New("watchlist.not_watched", "игра не отслеживается")
	errNoAlert      = uierr.New("watchlist.alert_not_found", "уведомление не найдено")
)

type releasePort interface {
	GetReleasesForGame(gameID string) []sources.ReleaseGroup
}

type titlePort interface {
	TitleOf(id string) string
}

type Entry struct {
	GameID  string    `json:"gameId"`
	Title   string    `json:"title"`
	AddedAt time.Time `json:"addedAt"`
	Known   []string  `json:"known"`
}

type Alert struct {
	ID           string    `json:"id"`
	GameID       string    `json:"gameId"`
	Title        string    `json:"title"`
	ReleaseID    string    `json:"releaseId"`
	ReleaseTitle string    `json:"releaseTitle"`
	Version      string    `json:"version"`
	SourceID     string    `json:"sourceId"`
	At           time.Time `json:"at"`
}

type Status struct {
	Degraded bool   `json:"degraded"`
	Message  string `json:"message"`
}

type state struct {
	Entries []Entry `json:"entries"`
	Alerts  []Alert `json:"alerts"`
}

type Service struct {
	mu       sync.Mutex
	path     string
	releases releasePort
	titles   titlePort
	now      func() time.Time
	data     state
	status   Status
}

func NewService(releases releasePort, titles titlePort) (*Service, error) {
	dir, err := settings.ConfigDir()
	if err != nil {
		return nil, fmt.Errorf("resolve config dir: %w", err)
	}
	if dir == "" {
		return nil, errors.New("config dir unavailable")
	}
	return NewServiceAt(filepath.Join(dir, "watchlist.json"), releases, titles)
}

//wails:ignore
func NewServiceAt(path string, releases releasePort, titles titlePort) (*Service, error) {
	if path == "" {
		return nil, errors.New("watchlist path unavailable")
	}
	if releases == nil {
		return nil, errors.New("watchlist releases port unavailable")
	}
	if titles == nil {
		return nil, errors.New("watchlist titles port unavailable")
	}
	s := &Service{path: path, releases: releases, titles: titles, now: time.Now}
	data, err := s.load()
	if err != nil {
		return nil, err
	}
	s.data = data
	return s, nil
}

func (s *Service) load() (state, error) {
	var data state
	err := storage.Load(s.path, watchlistVersion, nil, &data)
	if errors.Is(err, fs.ErrNotExist) {
		return state{}, nil
	}
	if err != nil {
		return state{}, fmt.Errorf("load watchlist: %w", err)
	}
	return data, nil
}

func (s *Service) persistLocked() error {
	return storage.Save(s.path, watchlistVersion, s.data)
}

func emit(name string, data any) {
	if app := application.Get(); app != nil {
		app.Event.Emit(name, data)
	}
}

func cloneState(d state) state {
	out := state{
		Entries: make([]Entry, len(d.Entries)),
		Alerts:  slices.Clone(d.Alerts),
	}
	for i, e := range d.Entries {
		e.Known = slices.Clone(e.Known)
		out.Entries[i] = e
	}
	return out
}

func (s *Service) entriesLocked() []Entry {
	return cloneState(s.data).Entries
}

func (s *Service) degradeLocked(err error) {
	s.status = Status{Degraded: true, Message: err.Error()}
	emit(eventDegraded, s.status)
}

// commitLocked persists s.data; on failure it restores previous, enters the
// degraded state and returns the error (invariants 4 and 5).
func (s *Service) commitLocked(previous state) error {
	if err := s.persistLocked(); err != nil {
		s.data = previous
		s.degradeLocked(err)
		return fmt.Errorf("save watchlist: %w", err)
	}
	s.status = Status{}
	return nil
}

func (s *Service) indexLocked(gameID string) int {
	return slices.IndexFunc(s.data.Entries, func(e Entry) bool { return e.GameID == gameID })
}

func visibleIDs(groups []sources.ReleaseGroup) []string {
	ids := make([]string, 0, len(groups))
	for _, g := range groups {
		if g.Release.Ignored || g.Release.ID == "" {
			continue
		}
		ids = append(ids, g.Release.ID)
	}
	return ids
}

func (s *Service) List() []Entry {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.entriesLocked()
}

func (s *Service) IsWatched(gameID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.indexLocked(gameID) >= 0
}

func (s *Service) Alerts() []Alert {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := slices.Clone(s.data.Alerts)
	if out == nil {
		out = []Alert{}
	}
	return out
}

func (s *Service) StatusOf() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status
}

func (s *Service) Watch(gameID string) (Entry, error) {
	gameID = strings.TrimSpace(gameID)
	if gameID == "" {
		return Entry{}, errEmptyID
	}
	s.mu.Lock()
	if i := s.indexLocked(gameID); i >= 0 {
		e := s.entriesLocked()[i]
		s.mu.Unlock()
		return e, nil
	}
	s.mu.Unlock()

	title := s.titles.TitleOf(gameID)
	if title == "" {
		return Entry{}, errGameNotFound
	}
	known := visibleIDs(s.releases.GetReleasesForGame(gameID))

	s.mu.Lock()
	defer s.mu.Unlock()
	if i := s.indexLocked(gameID); i >= 0 {
		return s.entriesLocked()[i], nil
	}
	previous := cloneState(s.data)
	entry := Entry{GameID: gameID, Title: title, AddedAt: s.now(), Known: known}
	s.data.Entries = append(s.data.Entries, entry)
	if err := s.commitLocked(previous); err != nil {
		return Entry{}, uierr.Wrap("watchlist.save_failed", err)
	}
	emit(eventUpdated, s.entriesLocked())
	return entry, nil
}

func (s *Service) Unwatch(gameID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := s.indexLocked(gameID)
	if i < 0 {
		return errNotWatched
	}
	previous := cloneState(s.data)
	s.data.Entries = slices.Delete(slices.Clone(s.data.Entries), i, i+1)
	s.data.Alerts = slices.DeleteFunc(slices.Clone(s.data.Alerts), func(a Alert) bool { return a.GameID == gameID })
	if err := s.commitLocked(previous); err != nil {
		return uierr.Wrap("watchlist.save_failed", err)
	}
	emit(eventUpdated, s.entriesLocked())
	return nil
}

func (s *Service) DismissAlert(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !slices.ContainsFunc(s.data.Alerts, func(a Alert) bool { return a.ID == id }) {
		return errNoAlert
	}
	previous := cloneState(s.data)
	s.data.Alerts = slices.DeleteFunc(slices.Clone(s.data.Alerts), func(a Alert) bool { return a.ID == id })
	if err := s.commitLocked(previous); err != nil {
		return uierr.Wrap("watchlist.save_failed", err)
	}
	emit(eventUpdated, s.entriesLocked())
	return nil
}

func (s *Service) DismissAlerts(gameID string) error {
	if strings.TrimSpace(gameID) == "" {
		return errEmptyID
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !slices.ContainsFunc(s.data.Alerts, func(a Alert) bool { return a.GameID == gameID }) {
		return nil
	}
	previous := cloneState(s.data)
	s.data.Alerts = slices.DeleteFunc(slices.Clone(s.data.Alerts), func(a Alert) bool { return a.GameID == gameID })
	if err := s.commitLocked(previous); err != nil {
		return uierr.Wrap("watchlist.save_failed", err)
	}
	emit(eventUpdated, s.entriesLocked())
	return nil
}

// HandleSourcesRefreshed finds releases of watched games that were not seen
// before. sources.Service.AddRefreshListener fixes the callback signature
// without an error result, so a persist failure cannot be handed back to a
// synchronous caller: memory is rolled back, the service goes degraded
// (watchlist:degraded) and the failure is logged. The same releases are
// found again on the next refresh because Known is not advanced.
func (s *Service) HandleSourcesRefreshed(ctx context.Context) {
	s.mu.Lock()
	ids := make([]string, len(s.data.Entries))
	for i, e := range s.data.Entries {
		ids[i] = e.GameID
	}
	s.mu.Unlock()

	found := make(map[string][]sources.ReleaseGroup, len(ids))
	for _, id := range ids {
		if ctx.Err() != nil {
			return
		}
		found[id] = s.releases.GetReleasesForGame(id)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if ctx.Err() != nil {
		return
	}
	previous := cloneState(s.data)
	now := s.now()
	var fresh []Alert
	for i := range s.data.Entries {
		entry := &s.data.Entries[i]
		groups, ok := found[entry.GameID]
		if !ok {
			continue
		}
		known := make(map[string]bool, len(entry.Known))
		for _, id := range entry.Known {
			known[id] = true
		}
		for _, g := range groups {
			r := g.Release
			if r.Ignored || r.ID == "" || known[r.ID] {
				continue
			}
			known[r.ID] = true
			entry.Known = append(entry.Known, r.ID)
			title := r.Title
			if title == "" {
				title = r.RawTitle
			}
			fresh = append(fresh, Alert{
				ID: newID(), GameID: entry.GameID, Title: entry.Title,
				ReleaseID: r.ID, ReleaseTitle: title, Version: r.Version,
				SourceID: r.SourceID, At: now,
			})
		}
	}
	if len(fresh) == 0 {
		return
	}
	s.data.Alerts = append(slices.Clone(s.data.Alerts), fresh...)
	if over := len(s.data.Alerts) - maxAlerts; over > 0 {
		s.data.Alerts = slices.Clone(s.data.Alerts[over:])
	}
	if err := s.commitLocked(previous); err != nil {
		slog.Error("watchlist: save new releases", "err", err)
		return
	}
	emit(eventAlert, fresh)
	emit(eventUpdated, s.entriesLocked())
}

func newID() string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("w%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
}
