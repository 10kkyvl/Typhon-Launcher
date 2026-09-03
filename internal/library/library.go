package library

import (
	"context"
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

	"typhon/internal/history"
	"typhon/internal/platform"
	"typhon/internal/procs"
	"typhon/internal/settings"
	"typhon/internal/storage"
	"typhon/internal/usagestats"

	"github.com/wailsapp/wails/v3/pkg/application"
)

type Game struct {
	ID                string     `json:"id"`
	Title             string     `json:"title"`
	Executable        string     `json:"executable"`
	LaunchArgs        []string   `json:"launchArgs,omitempty"`
	InstallDir        string     `json:"installDir"`
	Cover             string     `json:"cover"`
	Version           string     `json:"version"`
	VersionSource     string     `json:"versionSource,omitempty"`
	VersionConfidence float64    `json:"versionConfidence,omitempty"`
	SizeBytes         int64      `json:"sizeBytes"`
	SizeUnknown       bool       `json:"sizeUnknown,omitempty"`
	LastPlayed        *time.Time `json:"lastPlayed"`
	PlaytimeSeconds   int64      `json:"playtimeSeconds"`
	InstalledAt       time.Time  `json:"installedAt"`
	SourceDownloadID  string     `json:"sourceDownloadId,omitempty"`
	ReleaseID         string     `json:"releaseId,omitempty"`
	SourceID          string     `json:"sourceId,omitempty"`
	CanonicalGameID   string     `json:"canonicalGameId,omitempty"`
	Source            string     `json:"source,omitempty"`
	InstallType       string     `json:"installType,omitempty"`
	Owned             bool       `json:"owned,omitempty"`
	Uninstall         Uninstall  `json:"uninstall,omitzero"`
	UninstallUnknown  bool       `json:"uninstallUnknown,omitempty"`
	Uninstalled       bool       `json:"uninstalled,omitempty"`
	ShortcutPath      string     `json:"shortcutPath,omitempty"`
	SavesDir          string     `json:"savesDir,omitempty"`
	Favorite          bool       `json:"favorite,omitempty"`
	Status            string     `json:"status,omitempty"`
	StatusAt          *time.Time `json:"statusAt,omitempty"`
}

type Uninstall struct {
	Key          string `json:"key,omitempty"`
	Command      string `json:"command,omitempty"`
	QuietCommand string `json:"quietCommand,omitempty"`
	ProductCode  string `json:"productCode,omitempty"`
}

func (u Uninstall) Empty() bool {
	return u.Command == "" && u.QuietCommand == "" && u.ProductCode == ""
}

const (
	SourceManaged    = "managed"
	SourceDiscovered = "discovered"
)

type InstalledGame struct {
	Title            string    `json:"title"`
	Executable       string    `json:"executable"`
	InstallDir       string    `json:"installDir"`
	Version          string    `json:"version"`
	VersionSource    string    `json:"versionSource"`
	SourceDownloadID string    `json:"sourceDownloadId"`
	ReleaseID        string    `json:"releaseId"`
	SourceID         string    `json:"sourceId"`
	CanonicalGameID  string    `json:"canonicalGameId"`
	InstallType      string    `json:"installType"`
	Owned            bool      `json:"owned"`
	Uninstall        Uninstall `json:"uninstall,omitzero"`
	UninstallUnknown bool      `json:"uninstallUnknown"`
}

type InstalledUpdate struct {
	ID            string `json:"id"`
	Executable    string `json:"executable"`
	InstallDir    string `json:"installDir"`
	Version       string `json:"version"`
	VersionSource string `json:"versionSource"`
	ReleaseID     string `json:"releaseId"`
	SourceID      string `json:"sourceId"`
}

var (
	errEmptyInstallDir      = errors.New("каталог установки не задан")
	errNotFound             = errors.New("игра не найдена")
	errEmptyCanonicalGameID = errors.New("не указан идентификатор игры каталога")
	errEmptyCatalogTitle    = errors.New("не указано название игры")
)

const MaxFavorites = 6

const (
	StatusPlaying   = "playing"
	StatusCompleted = "completed"
	StatusDropped   = "dropped"
	StatusBacklog   = "backlog"
	StatusPaused    = "paused"
)

func ValidStatus(s string) bool {
	switch s {
	case "", StatusPlaying, StatusCompleted, StatusDropped, StatusBacklog, StatusPaused:
		return true
	default:
		return false
	}
}

var ErrTooManyFavorites = errors.New("favorites limit reached")

var ErrInvalidStatus = errors.New("invalid game status")

type Service struct {
	mu            sync.Mutex
	path          string
	excludedPath  string
	games         []Game
	excluded      []string
	running       map[string]*session
	onSession     func(gameID string, seconds int64)
	playRecord    func(gameID string, startedAt, endedAt time.Time)
	watchers      []SessionWatcher
	usageRecord   func(ev usagestats.Event)
	historyRecord func(r history.Record) error
	wg            sync.WaitGroup
	sessionWG     sync.WaitGroup
	scan          func(context.Context) ([]procs.Process, error)
	watchInterval time.Duration
	now           func() time.Time
	ctx           context.Context
	cancel        context.CancelFunc
	closed        bool
	watching      bool
	shortcuts     shortcutBackend
	launcherPath  func() (string, error)
	saveRoots     func() ([]platform.SaveRoot, error)
	start         gameStarter
}

type SessionWatcher interface {
	SessionStarted(game Game)
	SessionStopped(gameID string)
}

type session struct {
	process   gameProcess // nil у сессии, обнаруженной в системе, а не запущенной нами
	pid       uint32
	createdAt time.Time // время старта процесса по данным ОС; нулевое — неизвестно
	startedAt time.Time // с этого момента считается наигранное время
	lastSeen  time.Time
	external  bool
}

func NewService() (*Service, error) {
	dir, err := settings.ConfigDir()
	if err != nil {
		return nil, fmt.Errorf("resolve config dir: %w", err)
	}
	if dir == "" {
		return nil, errors.New("config dir unavailable")
	}
	return NewServiceAt(filepath.Join(dir, "library.json"))
}

//wails:ignore
func NewServiceAt(path string) (*Service, error) {
	if path == "" {
		return nil, errors.New("library path unavailable")
	}
	excludedPath, err := excludedPathFor(path)
	if err != nil {
		return nil, err
	}
	s := &Service{
		path:          path,
		excludedPath:  excludedPath,
		running:       map[string]*session{},
		scan:          procs.List,
		watchInterval: defaultWatchInterval,
		now:           time.Now,
		shortcuts:     systemShortcuts{},
		launcherPath:  os.Executable,
		saveRoots:     platform.SaveRoots,
		start:         newGameStarter(),
	}
	games, err := s.load()
	if err != nil {
		return nil, err
	}
	excluded, err := loadExcluded(excludedPath)
	if err != nil {
		return nil, err
	}
	s.games = games
	s.excluded = excluded
	return s, nil
}

type legacyGame struct {
	Game
	Completed   bool       `json:"completed"`
	CompletedAt *time.Time `json:"completedAt"`
}

func (s *Service) load() ([]Game, error) {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read library %s: %w", s.path, err)
	}
	var stored []legacyGame
	if err := json.Unmarshal(data, &stored); err != nil {
		return nil, fmt.Errorf("parse library %s: %w", s.path, err)
	}
	games := make([]Game, 0, len(stored))
	for _, entry := range stored {
		g := entry.Game
		if entry.Completed && g.Status == "" {
			g.Status = StatusCompleted
			g.StatusAt = entry.CompletedAt
		}
		games = append(games, g)
	}
	return games, nil
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
	if err := storage.WriteAtomic(s.path, data); err != nil {
		return fmt.Errorf("write library %s: %w", s.path, err)
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

func (s *Service) GetGames() []Game {
	s.mu.Lock()
	defer s.mu.Unlock()
	games := make([]Game, len(s.games))
	copy(games, s.games)
	return games
}

func (s *Service) GetInstalledGames() []Game {
	s.mu.Lock()
	defer s.mu.Unlock()
	games := make([]Game, 0, len(s.games))
	for _, game := range s.games {
		if game.Uninstalled {
			continue
		}
		games = append(games, game)
	}
	return games
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
	provided := strings.TrimSpace(title)
	title = provided
	if title == "" {
		title = TitleFromExecutable(executable)
	}

	installDir := strings.TrimSpace(filepath.Dir(executable))
	if installDir == "" {
		return Game{}, errEmptyInstallDir
	}
	size, unknown := measureInstall("", installDir)
	game := Game{
		ID:          newID(),
		Title:       title,
		Executable:  executable,
		InstallDir:  installDir,
		SizeBytes:   size,
		SizeUnknown: unknown,
		InstalledAt: time.Now(),
		Source:      SourceManaged,
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.games {
		if !strings.EqualFold(s.games[i].Executable, executable) {
			continue
		}
		if !s.games[i].Uninstalled {
			return Game{}, errors.New("эта игра уже добавлена")
		}
		return s.reviveLocked(i, provided, installDir, size, unknown)
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

// matchRegisteredLocked ищет запись, которую переустанавливают. Совпадение по
// исполняемому файлу — прежнее правило; запись, снятую с ПК, дополнительно ловим
// по игре каталога, иначе установка в другую папку заводит вторую карточку той
// же игры рядом с первой.
func (s *Service) matchRegisteredLocked(g InstalledGame) int {
	match := -1
	for i := range s.games {
		if strings.EqualFold(s.games[i].Executable, g.Executable) {
			return i
		}
		if match < 0 && s.games[i].Uninstalled && g.CanonicalGameID != "" && s.games[i].CanonicalGameID == g.CanonicalGameID {
			match = i
		}
	}
	return match
}

func (s *Service) reviveLocked(pos int, title, installDir string, size int64, unknown bool) (Game, error) {
	previous := s.games[pos]
	s.games[pos].Uninstalled = false
	s.games[pos].InstallDir = installDir
	s.games[pos].SizeBytes = size
	s.games[pos].SizeUnknown = unknown
	if title != "" {
		s.games[pos].Title = title
	}
	if err := s.persist(); err != nil {
		s.games[pos] = previous
		return Game{}, fmt.Errorf("save library: %w", err)
	}
	slog.Info("game reinstalled", "id", s.games[pos].ID, "title", s.games[pos].Title)
	s.emitUpdated()
	return s.games[pos], nil
}

func (s *Service) RegisterInstalled(g InstalledGame) (Game, error) {
	info, err := os.Stat(g.Executable)
	if err != nil || info.IsDir() {
		return Game{}, errors.New("исполняемый файл не найден")
	}
	installDir := strings.TrimSpace(g.InstallDir)
	if installDir == "" {
		return Game{}, errEmptyInstallDir
	}
	title := strings.TrimSpace(g.Title)

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.allowLocked(installDir); err != nil {
		return Game{}, err
	}
	if i := s.matchRegisteredLocked(g); i >= 0 {
		previous := s.games[i]
		if title != "" {
			s.games[i].Title = title
		}
		s.games[i].Executable = g.Executable
		s.games[i].Uninstalled = false
		s.games[i].InstallDir = installDir
		s.games[i].Version = g.Version
		s.games[i].VersionSource = g.VersionSource
		s.games[i].SizeBytes, s.games[i].SizeUnknown = measureInstall(s.games[i].ID, installDir)
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
		s.games[i].Source = SourceManaged
		s.games[i].InstallType = g.InstallType
		s.games[i].Owned = g.Owned
		s.games[i].Uninstall = g.Uninstall
		s.games[i].UninstallUnknown = g.UninstallUnknown
		if err := s.persist(); err != nil {
			s.games[i] = previous
			return Game{}, fmt.Errorf("save library: %w", err)
		}
		markInstalled(s.games[i])
		slog.Info("game updated", "id", s.games[i].ID, "title", s.games[i].Title)
		s.emitUpdated()
		return s.games[i], nil
	}

	if title == "" {
		title = TitleFromExecutable(g.Executable)
	}
	size, unknown := measureInstall("", installDir)
	game := Game{
		ID:               newID(),
		Title:            title,
		Executable:       g.Executable,
		InstallDir:       installDir,
		Version:          g.Version,
		VersionSource:    g.VersionSource,
		SizeBytes:        size,
		SizeUnknown:      unknown,
		InstalledAt:      time.Now(),
		SourceDownloadID: g.SourceDownloadID,
		ReleaseID:        g.ReleaseID,
		SourceID:         g.SourceID,
		CanonicalGameID:  g.CanonicalGameID,
		Source:           SourceManaged,
		InstallType:      g.InstallType,
		Owned:            g.Owned,
		Uninstall:        g.Uninstall,
		UninstallUnknown: g.UninstallUnknown,
	}
	s.games = append(s.games, game)
	if err := s.persist(); err != nil {
		s.games = s.games[:len(s.games)-1]
		return Game{}, fmt.Errorf("save library: %w", err)
	}
	markInstalled(game)
	slog.Info("game installed", "id", game.ID, "title", game.Title)
	s.emitUpdated()
	return game, nil
}

//wails:ignore
func (s *Service) SetOnSessionEnded(fn func(gameID string, seconds int64)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onSession = fn
}

//wails:ignore
func (s *Service) SetPlayRecorder(fn func(gameID string, startedAt, endedAt time.Time)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.playRecord = fn
}

//wails:ignore
func (s *Service) AddSessionWatcher(w SessionWatcher) {
	if w == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.watchers = append(s.watchers, w)
}

//wails:ignore
func (s *Service) SetUsageRecorder(rec func(ev usagestats.Event)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.usageRecord = rec
}

func (s *Service) recordUsage(ev usagestats.Event) {
	if s.usageRecord == nil {
		return
	}
	s.usageRecord(ev)
}

//wails:ignore
func (s *Service) ApplyInstalledUpdate(u InstalledUpdate) (Game, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.games {
		if s.games[i].ID != u.ID {
			continue
		}
		previous := s.games[i]
		if u.Executable != "" {
			s.games[i].Executable = u.Executable
		}
		if u.InstallDir != "" {
			s.games[i].InstallDir = u.InstallDir
		}
		if s.games[i].InstallDir == "" {
			return Game{}, errEmptyInstallDir
		}
		s.games[i].Version = u.Version
		s.games[i].VersionSource = u.VersionSource
		s.games[i].SizeBytes, s.games[i].SizeUnknown = measureInstall(s.games[i].ID, s.games[i].InstallDir)
		if u.ReleaseID != "" {
			s.games[i].ReleaseID = u.ReleaseID
		}
		if u.SourceID != "" {
			s.games[i].SourceID = u.SourceID
		}
		if err := s.persist(); err != nil {
			s.games[i] = previous
			return Game{}, fmt.Errorf("save library: %w", err)
		}
		slog.Info("game version updated", "id", u.ID, "version", u.Version)
		s.emitUpdated()
		return s.games[i], nil
	}
	return Game{}, errors.New("игра не найдена")
}

func (s *Service) RemoveGame(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, game := range s.games {
		if game.ID != id {
			continue
		}
		if game.InstallDir != "" {
			_, err := os.Stat(game.InstallDir)
			switch {
			case err == nil:
				if err := s.excludeLocked(game.InstallDir); err != nil {
					return err
				}
			case !errors.Is(err, fs.ErrNotExist):
				return fmt.Errorf("stat %s: %w", game.InstallDir, err)
			}
		}
		s.dropShortcutLocked(&s.games[i])
		previous := append([]Game(nil), s.games...)
		s.games = append(s.games[:i:i], s.games[i+1:]...)
		if err := s.persist(); err != nil {
			s.games = previous
			if rollback := s.allowLocked(game.InstallDir); rollback != nil {
				return errors.Join(err, rollback)
			}
			return err
		}
		slog.Info("game removed", "id", id, "title", game.Title)
		s.emitUpdated()
		// Игра уже удалена и сохранена. Вернуть отсюда ошибку журнала значит
		// сказать пользователю «не удалось удалить» про удалённую игру; о сбое
		// самого журнала он узнаёт из его признака degraded.
		if s.historyRecord != nil {
			if err := s.historyRecord(history.Record{
				Kind:       history.KindRemoved,
				GameID:     game.ID,
				Title:      game.Title,
				Bytes:      game.SizeBytes,
				BytesKnown: !game.SizeUnknown,
			}); err != nil {
				slog.Error("record history", "kind", history.KindRemoved, "id", id, "error", err)
			}
		}
		return nil
	}
	return errNotFound
}

// MarkUninstalled оставляет карточку в библиотеке, но снимает с неё всё, что
// описывает пропавшую установку: игры на диске больше нет, а наигранное время,
// привязка к каталогу и путь для повторной установки остаются.
//
//wails:ignore
func (s *Service) MarkUninstalled(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.games {
		if s.games[i].ID != id {
			continue
		}
		s.dropShortcutLocked(&s.games[i])
		previous := s.games[i]
		s.games[i].Uninstalled = true
		s.games[i].SizeBytes = 0
		s.games[i].SizeUnknown = false
		s.games[i].Version = ""
		s.games[i].VersionSource = ""
		s.games[i].VersionConfidence = 0
		s.games[i].InstallType = ""
		s.games[i].Owned = false
		s.games[i].Uninstall = Uninstall{}
		s.games[i].UninstallUnknown = false
		if err := s.persist(); err != nil {
			s.games[i] = previous
			return fmt.Errorf("save library: %w", err)
		}
		slog.Info("game uninstalled", "id", id, "title", s.games[i].Title)
		s.emitUpdated()
		// Запись уже обновлена и сохранена — см. комментарий в RemoveGame.
		if s.historyRecord != nil {
			if err := s.historyRecord(history.Record{
				Kind:       history.KindUninstalled,
				GameID:     previous.ID,
				Title:      previous.Title,
				Bytes:      previous.SizeBytes,
				BytesKnown: !previous.SizeUnknown,
			}); err != nil {
				slog.Error("record history", "kind", history.KindUninstalled, "id", id, "error", err)
			}
		}
		return nil
	}
	return errNotFound
}

func (s *Service) SetFavorite(id string, on bool) (Game, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	game := s.findLocked(id)
	if game == nil {
		return Game{}, errNotFound
	}
	if game.Favorite == on {
		return *game, nil
	}
	if on && s.favoriteCountLocked() >= MaxFavorites {
		return Game{}, ErrTooManyFavorites
	}
	previous := *game
	game.Favorite = on
	now := s.now()
	game.StatusAt = &now
	if err := s.persist(); err != nil {
		*game = previous
		return Game{}, fmt.Errorf("save library: %w", err)
	}
	s.emitUpdated()
	return *game, nil
}

func (s *Service) SetStatus(id, status string) (Game, error) {
	if !ValidStatus(status) {
		return Game{}, ErrInvalidStatus
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	game := s.findLocked(id)
	if game == nil {
		return Game{}, errNotFound
	}
	if game.Status == status {
		return *game, nil
	}
	previous := *game
	game.Status = status
	if status != "" {
		now := s.now()
		game.StatusAt = &now
	} else {
		game.StatusAt = nil
	}
	if err := s.persist(); err != nil {
		*game = previous
		return Game{}, fmt.Errorf("save library: %w", err)
	}
	s.emitUpdated()
	return *game, nil
}

func (s *Service) favoriteCountLocked() int {
	count := 0
	for i := range s.games {
		if s.games[i].Favorite {
			count++
		}
	}
	return count
}

//wails:ignore
func (s *Service) Find(id string) (Game, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.games {
		if s.games[i].ID == id {
			return s.games[i], nil
		}
	}
	return Game{}, errNotFound
}

//wails:ignore
func (s *Service) IsRunning(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.running[id]
	return ok
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

func dirSize(dir string) (int64, error) {
	if dir == "" {
		return 0, errEmptyInstallDir
	}
	var total int64
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return fmt.Errorf("stat %s: %w", path, err)
		}
		total += info.Size()
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("measure %s: %w", dir, err)
	}
	return total, nil
}

// measureInstall не роняет регистрацию игры: недоступный подкаталог не повод
// потерять запись, но и нулевой размер выдавать за настоящий нельзя.
func measureInstall(id, dir string) (int64, bool) {
	size, err := dirSize(dir)
	if err != nil {
		slog.Warn("measure install dir", "id", id, "error", err)
		return 0, true
	}
	return size, false
}
