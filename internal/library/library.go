package library

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"typhon/internal/settings"

	"github.com/wailsapp/wails/v3/pkg/application"
)

type Game struct {
	ID               string     `json:"id"`
	Title            string     `json:"title"`
	Executable       string     `json:"executable"`
	LaunchArgs       []string   `json:"launchArgs,omitempty"`
	InstallDir       string     `json:"installDir"`
	Cover            string     `json:"cover"`
	Hero             string     `json:"hero"`
	Version          string     `json:"version"`
	SizeBytes        int64      `json:"sizeBytes"`
	LastPlayed       *time.Time `json:"lastPlayed"`
	PlaytimeSeconds  int64      `json:"playtimeSeconds"`
	InstalledAt      time.Time  `json:"installedAt"`
	SourceDownloadID string     `json:"sourceDownloadId,omitempty"`
	ReleaseID        string     `json:"releaseId,omitempty"`
	SourceID         string     `json:"sourceId,omitempty"`
	CanonicalGameID  string     `json:"canonicalGameId,omitempty"`
}

type InstalledGame struct {
	Title            string `json:"title"`
	Executable       string `json:"executable"`
	InstallDir       string `json:"installDir"`
	Version          string `json:"version"`
	SourceDownloadID string `json:"sourceDownloadId"`
	ReleaseID        string `json:"releaseId"`
	SourceID         string `json:"sourceId"`
	CanonicalGameID  string `json:"canonicalGameId"`
}

type Service struct {
	mu      sync.Mutex
	path    string
	games   []Game
	running map[string]*session
}

type session struct {
	process   *os.Process
	startedAt time.Time
}

func NewService() *Service {
	dir, err := settings.ConfigDir()
	if err != nil {
		slog.Error("resolve config dir", "error", err)
		return newServiceAt("")
	}
	return newServiceAt(filepath.Join(dir, "library.json"))
}

func newServiceAt(path string) *Service {
	s := &Service{path: path, running: map[string]*session{}}
	s.load()
	return s
}

func (s *Service) load() {
	if s.path == "" {
		return
	}
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return
	}
	if err != nil {
		slog.Error("read library", "path", s.path, "error", err)
		return
	}
	if err := json.Unmarshal(data, &s.games); err != nil {
		slog.Error("parse library", "path", s.path, "error", err)
	}
}

func (s *Service) persist() error {
	if s.path == "" {
		return errors.New("library path unavailable")
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s.games, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(s.path, data, 0o644); err != nil {
		slog.Error("write library", "path", s.path, "error", err)
		return err
	}
	return nil
}

func emit(name string, data any) {
	if app := application.Get(); app != nil {
		app.Event.Emit(name, data)
	}
}

func (s *Service) emitUpdated() {
	emit("library:updated", s.games)
}

func (s *Service) GetInstalledGames() []Game {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Game(nil), s.games...)
}

func (s *Service) GetRunningGames() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	ids := make([]string, 0, len(s.running))
	for id := range s.running {
		ids = append(ids, id)
	}
	return ids
}

func (s *Service) AddGame(executable, title string) (Game, error) {
	info, err := os.Stat(executable)
	if err != nil || info.IsDir() {
		return Game{}, errors.New("исполняемый файл не найден")
	}
	title = strings.TrimSpace(title)
	if title == "" {
		title = TitleFromExecutable(executable)
	}

	installDir := filepath.Dir(executable)
	game := Game{
		ID:          newID(),
		Title:       title,
		Executable:  executable,
		InstallDir:  installDir,
		SizeBytes:   dirSize(installDir),
		InstalledAt: time.Now(),
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.games {
		if strings.EqualFold(existing.Executable, executable) {
			return Game{}, errors.New("эта игра уже добавлена")
		}
	}
	s.games = append(s.games, game)
	if err := s.persist(); err != nil {
		s.games = s.games[:len(s.games)-1]
		return Game{}, fmt.Errorf("save library: %w", err)
	}
	slog.Info("game added", "id", game.ID, "title", game.Title)
	s.emitUpdated()
	return game, nil
}

func (s *Service) RegisterInstalled(g InstalledGame) (Game, error) {
	info, err := os.Stat(g.Executable)
	if err != nil || info.IsDir() {
		return Game{}, errors.New("исполняемый файл не найден")
	}
	installDir := strings.TrimSpace(g.InstallDir)
	if installDir == "" {
		installDir = filepath.Dir(g.Executable)
	}
	title := strings.TrimSpace(g.Title)

	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.games {
		if !strings.EqualFold(s.games[i].Executable, g.Executable) {
			continue
		}
		previous := s.games[i]
		if title != "" {
			s.games[i].Title = title
		}
		s.games[i].InstallDir = installDir
		s.games[i].Version = g.Version
		s.games[i].SizeBytes = dirSize(installDir)
		s.games[i].SourceDownloadID = g.SourceDownloadID
		if g.ReleaseID != "" {
			s.games[i].ReleaseID = g.ReleaseID
		}
		if g.SourceID != "" {
			s.games[i].SourceID = g.SourceID
		}
		if g.CanonicalGameID != "" {
			s.games[i].CanonicalGameID = g.CanonicalGameID
		}
		if err := s.persist(); err != nil {
			s.games[i] = previous
			return Game{}, fmt.Errorf("save library: %w", err)
		}
		slog.Info("game updated", "id", s.games[i].ID, "title", s.games[i].Title)
		s.emitUpdated()
		return s.games[i], nil
	}

	if title == "" {
		title = TitleFromExecutable(g.Executable)
	}
	game := Game{
		ID:               newID(),
		Title:            title,
		Executable:       g.Executable,
		InstallDir:       installDir,
		Version:          g.Version,
		SizeBytes:        dirSize(installDir),
		InstalledAt:      time.Now(),
		SourceDownloadID: g.SourceDownloadID,
		ReleaseID:        g.ReleaseID,
		SourceID:         g.SourceID,
		CanonicalGameID:  g.CanonicalGameID,
	}
	s.games = append(s.games, game)
	if err := s.persist(); err != nil {
		s.games = s.games[:len(s.games)-1]
		return Game{}, fmt.Errorf("save library: %w", err)
	}
	slog.Info("game installed", "id", game.ID, "title", game.Title)
	s.emitUpdated()
	return game, nil
}

func (s *Service) RemoveGame(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, game := range s.games {
		if game.ID == id {
			s.games = append(s.games[:i], s.games[i+1:]...)
			if err := s.persist(); err != nil {
				return err
			}
			slog.Info("game removed", "id", id, "title", game.Title)
			s.emitUpdated()
			return nil
		}
	}
	return errors.New("игра не найдена")
}

func TitleFromExecutable(executable string) string {
	name := strings.TrimSuffix(filepath.Base(executable), filepath.Ext(executable))
	name = strings.NewReplacer("_", " ", "-", " ", ".", " ").Replace(name)
	return strings.TrimSpace(name)
}

func newID() string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("g%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
}

func dirSize(dir string) int64 {
	var total int64
	_ = filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() {
			if info, err := d.Info(); err == nil {
				total += info.Size()
			}
		}
		return nil
	})
	return total
}
