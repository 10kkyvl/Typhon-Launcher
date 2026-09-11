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
	"typhon/internal/uierr"
	"typhon/internal/usagestats"

	"github.com/wailsapp/wails/v3/pkg/application"
)

type Game struct {
	Archived   bool     `json:"archived,omitempty"`
	ID         string   `json:"id"`
	Title      string   `json:"title"`
	Executable string   `json:"executable"`
	LaunchArgs []string `json:"launchArgs,omitempty"`
	// RequiresSteam — запускать игру в общем бутыле CrossOver со Steam, а не в
	// собственном изолированном. nil значит «на усмотрение лаунчера»: общий
	// бутыль выбирается, когда он есть, потому что Steam Overlay и Steam API
	// работают только когда игра и Steam живут в одном wine-префиксе. Явный
	// false — единственный способ потребовать изолированный бутыль.
	RequiresSteam     *bool      `json:"requiresSteam,omitempty"`
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
	DistributionID    string     `json:"distributionId,omitempty"`
	ReleaseUploadedAt *time.Time `json:"releaseUploadedAt,omitempty"`
	CanonicalGameID   string     `json:"canonicalGameId,omitempty"`
	Repacker          string     `json:"repacker,omitempty"`
	ReleaseVersion    string     `json:"releaseVersion,omitempty"`
	Source            string     `json:"source,omitempty"`
	InstallType       string     `json:"installType,omitempty"`
	Owned             bool       `json:"owned,omitempty"`
	Uninstall         Uninstall  `json:"uninstall,omitzero"`
	UninstallUnknown  bool       `json:"uninstallUnknown,omitempty"`
	Uninstalled       bool       `json:"uninstalled,omitempty"`
	ShortcutPath      string     `json:"shortcutPath,omitempty"`
	SavesDir          string     `json:"savesDir,omitempty"`
	Favorite          bool       `json:"favorite,omitempty"`
	FavoriteAt        *time.Time `json:"favoriteAt,omitempty"`
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

// UsesSharedBottle отвечает, разрешено ли игре ехать в общий бутыль. По
// умолчанию разрешено: изоляция — исключение, которое пользователь включает
// руками.
func (g Game) UsesSharedBottle() bool { return g.RequiresSteam == nil || *g.RequiresSteam }

const (
	SourceManaged    = "managed"
	SourceDiscovered = "discovered"
)

type InstalledGame struct {
	Title             string     `json:"title"`
	Executable        string     `json:"executable"`
	InstallDir        string     `json:"installDir"`
	Version           string     `json:"version"`
	VersionSource     string     `json:"versionSource"`
	SourceDownloadID  string     `json:"sourceDownloadId"`
	ReleaseID         string     `json:"releaseId"`
	SourceID          string     `json:"sourceId"`
	DistributionID    string     `json:"distributionId"`
	ReleaseUploadedAt *time.Time `json:"releaseUploadedAt"`
	CanonicalGameID   string     `json:"canonicalGameId"`
	Repacker          string     `json:"repacker"`
	ReleaseVersion    string     `json:"releaseVersion"`
	InstallType       string     `json:"installType"`
	Owned             bool       `json:"owned"`
	Uninstall         Uninstall  `json:"uninstall,omitzero"`
	UninstallUnknown  bool       `json:"uninstallUnknown"`
}

type InstalledUpdate struct {
	ID                string     `json:"id"`
	Executable        string     `json:"executable"`
	InstallDir        string     `json:"installDir"`
	Version           string     `json:"version"`
	VersionSource     string     `json:"versionSource"`
	ReleaseID         string     `json:"releaseId"`
	SourceID          string     `json:"sourceId"`
	DistributionID    string     `json:"distributionId"`
	ReleaseUploadedAt *time.Time `json:"releaseUploadedAt"`
}

var (
	errEmptyInstallDir      = uierr.New("library.no_install_dir", "каталог установки не задан")
	errNotFound             = uierr.New("library.game_not_found", "игра не найдена")
	errEmptyCanonicalGameID = uierr.New("library.no_canonical_id", "не указан идентификатор игры каталога")
	errEmptyCatalogTitle    = uierr.New("library.no_catalog_title", "не указано название игры")
	errInstallationChanged  = uierr.New("library.installation_changed", "установка изменилась во время обновления")
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

var ErrTooManyFavorites = uierr.New("library.too_many_favorites", "favorites limit reached")

var ErrInvalidStatus = uierr.New("library.invalid_status", "invalid game status")

type Service struct {
	starting      map[string]context.CancelFunc
	sameCanonical func(string, string) bool
	mu            sync.Mutex
	path          string
	excludedPath  string
	games         []Game
	archived      []Game
	excluded      []string
	running       map[string]*session
	onSession     func(gameID string, seconds int64)
	// onOutcome получает исход запуска: сколько играли и закрыли ли игру
	// сами. Отдельно от onSession, потому что у того другой смысл — учёт
	// наигранного времени.
	onOutcome     func(gameID string, played time.Duration, stoppedByUser bool)
	onLaunchFail  func(gameID, code, reason string)
	playRecord    func(gameID string, startedAt, endedAt time.Time)
	watchers      []SessionWatcher
	usageRecord   func(ev usagestats.Event)
	historyRecord func(r history.Record) error
	wg            sync.WaitGroup
	sessionWG     sync.WaitGroup
	scan          func(context.Context) ([]procs.Process, bool, error)
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
	// prepare готовит окружение запуска. Поле, а не прямой вызов: на macOS
	// настоящая реализация заводит бутыль CrossOver, и тесты обязаны иметь
	// возможность её подменить, иначе прогон оставляет после себя
	// настоящие бутыли на машине разработчика.
	prepare func(ctx context.Context, req launch) error
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
	// stoppedByUser отделяет «игру закрыли» от «игра умерла сама»: для
	// журнала совместимости это разные события, а по одной длительности их
	// не различить.
	stoppedByUser bool
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
		prepare:       prepareRuntime,
	}
	games, err := s.load()
	if err != nil {
		return nil, err
	}
	excluded, err := loadExcluded(excludedPath)
	if err != nil {
		return nil, err
	}
	for _, game := range games {
		if game.Archived {
			s.archived = append(s.archived, game)
		} else {
			s.games = append(s.games, game)
		}
	}
	s.excluded = excluded
	return s, nil
}

type legacyGame struct {
	Game
	Completed   bool       `json:"completed"`
	CompletedAt *time.Time `json:"completedAt"`
}

func legacyStamp() time.Time { return time.Unix(0, 0).UTC() }

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
		if g.Owned && g.InstallType == "" {
			// Sync used to contaminate this flag. A typed on-disk marker is
			// local installation evidence; a bare cloud flag is not.
			m, markerErr := ReadMarker(g.InstallDir)
			switch {
			case markerErr == nil:
				g.Owned = m.InstallType != "" && m.Owned
			case errors.Is(markerErr, fs.ErrNotExist):
				g.Owned = false
			default:
				// Permission errors, an unavailable volume and malformed JSON do
				// not prove that the installation stopped being owned.
				// Keep the flag unchanged until the marker can be checked.
			}
		}
		if entry.Completed && g.Status == "" {
			g.Status = StatusCompleted
			g.StatusAt = entry.CompletedAt
		}
		if g.Status != "" && g.StatusAt == nil {
			stamp := legacyStamp()
			g.StatusAt = &stamp
		}
		if g.Favorite && g.FavoriteAt == nil {
			stamp := legacyStamp()
			g.FavoriteAt = &stamp
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
	data, err := json.MarshalIndent(append(append([]Game{}, s.games...), s.archived...), "", "  ")
	if err != nil {
		return err
	}
	if err := storage.WriteAtomic(s.path, data); err != nil {
		return fmt.Errorf("write library %s: %w", s.path, err)
	}
	return nil
}

const EventUpdated = "library:updated"

func emit(name string, data any) {
	if app := application.Get(); app != nil {
		app.Event.Emit(name, data)
	}
}

func (s *Service) emitUpdated() {
	emit(EventUpdated, s.games)
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
	for id := range s.starting {
		ids = append(ids, id)
	}
	return ids
}

func (s *Service) AddGame(executable, title string) (Game, error) {
	info, err := os.Stat(executable)
	if err != nil || info.IsDir() {
		return Game{}, uierr.New("library.no_executable", "исполняемый файл не найден")
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
			return Game{}, uierr.New("library.already_added", "эта игра уже добавлена")
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
		if match < 0 && s.games[i].Uninstalled && g.CanonicalGameID != "" && s.sameCanonicalLocked(s.games[i].CanonicalGameID, g.CanonicalGameID) {
			match = i
		}
	}
	return match
}

func (s *Service) reviveLocked(pos int, title, installDir string, size int64, unknown bool) (Game, error) {
	previous := s.games[pos]
	clearInstalledProvenance(&s.games[pos])
	s.games[pos].Owned = false
	s.games[pos].InstallType = ""
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
		return Game{}, uierr.New("library.no_executable", "исполняемый файл не найден")
	}
	installDir := strings.TrimSpace(g.InstallDir)
	if installDir == "" {
		return Game{}, errEmptyInstallDir
	}
	title := strings.TrimSpace(g.Title)
	size, unknown := measureInstall("", installDir)

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
		s.games[i].SizeBytes, s.games[i].SizeUnknown = size, unknown
		s.games[i].SourceDownloadID = g.SourceDownloadID
		s.games[i].ReleaseID = g.ReleaseID
		s.games[i].SourceID = g.SourceID
		s.games[i].DistributionID = g.DistributionID
		s.games[i].ReleaseUploadedAt = g.ReleaseUploadedAt
		if g.CanonicalGameID != "" {
			s.games[i].CanonicalGameID = g.CanonicalGameID
		}
		// Переустановка может принести другую сборку, и тогда прошлая больше
		// не описывает то, что лежит на диске.
		s.games[i].Repacker = g.Repacker
		s.games[i].ReleaseVersion = g.ReleaseVersion
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
	game := Game{
		ID:                newID(),
		Title:             title,
		Executable:        g.Executable,
		InstallDir:        installDir,
		Version:           g.Version,
		VersionSource:     g.VersionSource,
		SizeBytes:         size,
		SizeUnknown:       unknown,
		InstalledAt:       time.Now(),
		SourceDownloadID:  g.SourceDownloadID,
		ReleaseID:         g.ReleaseID,
		SourceID:          g.SourceID,
		DistributionID:    g.DistributionID,
		ReleaseUploadedAt: g.ReleaseUploadedAt,
		CanonicalGameID:   g.CanonicalGameID,
		Repacker:          g.Repacker,
		ReleaseVersion:    g.ReleaseVersion,
		Source:            SourceManaged,
		InstallType:       g.InstallType,
		Owned:             g.Owned,
		Uninstall:         g.Uninstall,
		UninstallUnknown:  g.UninstallUnknown,
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
func (s *Service) SetOutcomeRecorder(fn func(gameID string, played time.Duration, stoppedByUser bool)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onOutcome = fn
}

//wails:ignore
func (s *Service) SetLaunchFailureRecorder(fn func(gameID, code, reason string)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onLaunchFail = fn
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
	const maxMeasureRetries = 3
	preserveExecutable := false
	for attempt := 0; attempt < maxMeasureRetries; attempt++ {
		before, err := s.Find(u.ID)
		if err != nil {
			return Game{}, err
		}
		dir := u.InstallDir
		if dir == "" {
			dir = before.InstallDir
		}
		if dir == "" {
			return Game{}, errEmptyInstallDir
		}
		size, unknown := measureInstall(u.ID, dir)

		s.mu.Lock()
		pos := -1
		for i := range s.games {
			if s.games[i].ID == u.ID {
				pos = i
				break
			}
		}
		if pos < 0 {
			s.mu.Unlock()
			return Game{}, errors.New("игра не найдена")
		}
		previous := s.games[pos]
		if previous.InstallDir != before.InstallDir || previous.Executable != before.Executable {
			s.mu.Unlock()
			if u.InstallDir != "" && previous.InstallDir != u.InstallDir {
				return Game{}, errInstallationChanged
			}
			if previous.Executable != before.Executable {
				// The update pipeline may still carry the executable selected
				// before the measurement started. If the user changed it while
				// we were measuring, keep that newer choice instead of silently
				// switching it back. A different update target is safe only when
				// it agrees with the current user choice; otherwise report a
				// conflict and leave the library untouched.
				switch {
				case u.Executable == "", u.Executable == before.Executable:
					preserveExecutable = true
				case u.Executable != previous.Executable:
					return Game{}, errInstallationChanged
				}
			}
			if attempt+1 == maxMeasureRetries {
				return Game{}, errInstallationChanged
			}
			continue
		}
		if u.Executable != "" && !preserveExecutable {
			s.games[pos].Executable = u.Executable
		}
		if u.InstallDir != "" {
			s.games[pos].InstallDir = u.InstallDir
		}
		if s.games[pos].InstallDir == "" {
			s.mu.Unlock()
			return Game{}, errEmptyInstallDir
		}
		s.games[pos].Version = u.Version
		s.games[pos].VersionSource = u.VersionSource
		s.games[pos].SizeBytes, s.games[pos].SizeUnknown = size, unknown
		s.games[pos].ReleaseID = u.ReleaseID
		s.games[pos].SourceID = u.SourceID
		s.games[pos].DistributionID = u.DistributionID
		s.games[pos].ReleaseUploadedAt = u.ReleaseUploadedAt
		if u.ReleaseID == "" {
			s.games[pos].Repacker = ""
			s.games[pos].ReleaseVersion = ""
		} else {
			s.games[pos].ReleaseVersion = u.Version
		}
		if err := s.persist(); err != nil {
			s.games[pos] = previous
			s.mu.Unlock()
			return Game{}, fmt.Errorf("save library: %w", err)
		}
		updated := s.games[pos]
		markInstalled(updated)
		slog.Info("game version updated", "id", u.ID, "version", u.Version)
		s.emitUpdated()
		s.mu.Unlock()
		return updated, nil
	}
	return Game{}, errInstallationChanged
}

// BindDistribution upgrades an older installation only when its exact saved
// source/release pair still matches. It never guesses from game metadata.
//
//wails:ignore
func (s *Service) BindDistribution(id, sourceID, releaseID, distributionID string, releaseUploadedAt *time.Time) (Game, error) {
	if id == "" || sourceID == "" || releaseID == "" || distributionID == "" {
		return Game{}, errors.New("неполная привязка раздачи")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.games {
		if s.games[i].ID != id {
			continue
		}
		if s.games[i].SourceID != sourceID || s.games[i].ReleaseID != releaseID {
			return Game{}, errors.New("сохранённая привязка раздачи изменилась")
		}
		if s.games[i].DistributionID != "" && s.games[i].DistributionID != distributionID {
			return Game{}, errors.New("установка уже привязана к другой раздаче")
		}
		if s.games[i].DistributionID == distributionID &&
			(s.games[i].ReleaseUploadedAt != nil || releaseUploadedAt == nil) {
			return s.games[i], nil
		}
		previous := s.games[i]
		s.games[i].DistributionID = distributionID
		if s.games[i].ReleaseUploadedAt == nil && releaseUploadedAt != nil {
			stamp := *releaseUploadedAt
			s.games[i].ReleaseUploadedAt = &stamp
		}
		if err := s.persist(); err != nil {
			s.games[i] = previous
			return Game{}, fmt.Errorf("save library: %w", err)
		}
		markInstalled(s.games[i])
		s.emitUpdated()
		return s.games[i], nil
	}
	return Game{}, errNotFound
}

func (s *Service) RemoveGame(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.removeGameLocked(id)
}

// RemoveSyncedGame archives cloud-only cards. A remote tombstone cannot remove
// a local installation, its shortcut or its discovery eligibility, including
// when the installation's volume is temporarily unavailable.
//
//wails:ignore
func (s *Service) RemoveSyncedGame(canonicalID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, game := range s.games {
		if s.sameCanonicalLocked(game.CanonicalGameID, canonicalID) {
			if game.InstallDir != "" || game.Executable != "" {
				return nil
			}
			return s.removeGameLocked(game.ID)
		}
	}
	return nil // Already removed is an idempotent sync result.
}

func (s *Service) removeGameLocked(id string) error {
	if s.starting[id] != nil {
		return uierr.New("library.already_running", "игра запускается")
	}
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
		previousArchive := append([]Game(nil), s.archived...)
		archived := Game{ID: game.ID, Title: game.Title, Cover: game.Cover, CanonicalGameID: game.CanonicalGameID,
			PlaytimeSeconds: game.PlaytimeSeconds, Status: game.Status, StatusAt: game.StatusAt, LastPlayed: game.LastPlayed, Archived: true}
		for j, old := range s.archived {
			if old.ID == game.ID {
				archived.PlaytimeSeconds += old.PlaytimeSeconds
				s.archived = append(s.archived[:j:j], s.archived[j+1:]...)
				break
			}
		}
		s.archived = append(s.archived, archived)
		s.games = append(s.games[:i:i], s.games[i+1:]...)
		if err := s.persist(); err != nil {
			s.games = previous
			s.archived = previousArchive
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
		clearInstalledProvenance(&s.games[i])
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
	game.FavoriteAt = &now
	if err := s.persist(); err != nil {
		*game = previous
		return Game{}, fmt.Errorf("save library: %w", err)
	}
	s.emitUpdated()
	return *game, nil
}

func (s *Service) SetRequiresSteam(id string, on bool) (Game, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	game := s.findLocked(id)
	if game == nil {
		return Game{}, errNotFound
	}
	if game.RequiresSteam != nil && *game.RequiresSteam == on {
		return *game, nil
	}
	previous := *game
	game.RequiresSteam = &on
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
	now := s.now()
	game.StatusAt = &now
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
	return ok || s.starting[id] != nil
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
var measureInstall = func(id, dir string) (int64, bool) {
	size, err := dirSize(dir)
	if err != nil {
		slog.Warn("measure install dir", "id", id, "error", err)
		return 0, true
	}
	return size, false
}

// GetHistoryGames includes removed games for local profile history only.
//
//wails:ignore
func (s *Service) GetHistoryGames() []Game {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := append([]Game{}, s.games...)
	for _, archived := range s.archived {
		found := false
		for i := range out {
			if out[i].ID == archived.ID {
				out[i].PlaytimeSeconds += archived.PlaytimeSeconds
				found = true
				break
			}
		}
		if !found {
			out = append(out, archived)
		}
	}
	return out
}

func clearInstalledProvenance(g *Game) {
	g.ReleaseID = ""
	g.SourceID = ""
	g.DistributionID = ""
	g.ReleaseUploadedAt = nil
	g.Repacker = ""
	g.ReleaseVersion = ""
}
