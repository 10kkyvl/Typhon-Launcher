package screenshots

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io/fs"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"typhon/internal/library"
	"typhon/internal/platform"
	"typhon/internal/procs"
	"typhon/internal/settings"
	"typhon/internal/storage"
	"typhon/internal/uierr"
)

const (
	EventSaved  = "screenshot:saved"
	EventFailed = "screenshot:failed"

	URLPrefix = "/screenshots/"

	nameLayout  = "2006-01-02_15-04-05"
	pngExt      = ".png"
	maxSegment  = 128
	maxSameName = 1000
)

var (
	ErrNoLibrary   = uierr.New("screenshots.no_library", "библиотека не настроена, папка снимков недоступна")
	ErrNoShots     = uierr.New("screenshots.no_shots", "снимков для этой игры ещё нет")
	ErrNoGame      = uierr.New("screenshots.no_game", "не удалось определить игру по окну")
	ErrNoWindow    = uierr.New("screenshots.no_window", "окно для снимка недоступно")
	ErrBadName     = uierr.New("screenshots.bad_name", "недопустимое имя игры или снимка")
	ErrUnsupported = uierr.New("screenshots.unsupported", "снимки экрана поддерживаются только в Windows")
	ErrTooLarge    = uierr.New("screenshots.too_large", "окно слишком большое для снимка")
	ErrGDI         = uierr.New("screenshots.capture_failed", "системный захват экрана завершился ошибкой")
	ErrWrite       = uierr.New("screenshots.write_failed", "не удалось сохранить снимок")
)

type runningPort interface {
	RunningGameForPID(pid uint32) (id string, ok bool)
	RunningGames() []string
	InstallDir(gameID string) (string, bool)
}

type Saved struct {
	GameID string    `json:"gameId"`
	Title  string    `json:"title"`
	Path   string    `json:"path"`
	Name   string    `json:"name"`
	At     time.Time `json:"at"`
}

type Failed struct {
	GameID string `json:"gameId"`
	Error  string `json:"error"`
}

type Shot struct {
	Name string    `json:"name"`
	Path string    `json:"path"`
	URL  string    `json:"url"`
	Size int64     `json:"size"`
	At   time.Time `json:"at"`
}

type Status struct {
	Enabled bool   `json:"enabled"`
	Hotkey  string `json:"hotkey"`
	Dir     string `json:"dir"`
}

type grabbed struct {
	img *image.RGBA
	pid uint32
}

type Service struct {
	config  func() settings.Settings
	games   runningPort
	grab    func(hwnd uintptr) (grabbed, error)
	pidOf   func(hwnd uintptr) (uint32, error)
	now     func() time.Time
	emit    func(name string, payload any)
	openDir func(path string) error
	exeOf   func(ctx context.Context, pid uint32) (string, error)
	parents func(ctx context.Context) (map[uint32]uint32, error)

	mu      sync.Mutex
	ctx     context.Context
	closed  bool
	wg      sync.WaitGroup
	titleOf func(gameID string) (string, bool)

	saveMu sync.Mutex

	armMu  sync.Mutex
	active map[string]struct{}
	arm    func(bool)
}

//wails:ignore
func NewService(config func() settings.Settings, games runningPort) (*Service, error) {
	if config == nil {
		return nil, errors.New("screenshots: settings source is required")
	}
	if games == nil {
		return nil, errors.New("screenshots: running games source is required")
	}
	return &Service{
		config:  config,
		games:   games,
		grab:    grabWindow,
		pidOf:   windowOwner,
		now:     time.Now,
		emit:    emitToApp,
		openDir: platform.OpenFolder,
		exeOf:   processImage,
		parents: procs.Parents,
		active:  map[string]struct{}{},
	}, nil
}

func emitToApp(name string, payload any) {
	if app := application.Get(); app != nil {
		app.Event.Emit(name, payload)
	}
}

//wails:ignore
func (s *Service) SetTitleResolver(titleOf func(gameID string) (string, bool)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.titleOf = titleOf
}

//wails:ignore
func (s *Service) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ctx = ctx
	return nil
}

//wails:ignore
func (s *Service) ServiceShutdown() error {
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
	s.wg.Wait()
	return nil
}

// Capture runs from the hotkey thread of the overlay, which must not wait for
// a screen grab, so the work goes to a goroutine the service owns.
//
//wails:ignore
func (s *Service) Capture(hwnd uintptr) {
	s.mu.Lock()
	if s.closed || s.ctx == nil {
		s.mu.Unlock()
		return
	}
	ctx := s.ctx
	s.wg.Add(1)
	s.mu.Unlock()

	go func() {
		defer s.wg.Done()
		s.run(ctx, hwnd)
	}()
}

func (s *Service) run(ctx context.Context, hwnd uintptr) {
	saved, err := s.capture(ctx, hwnd)
	if errors.Is(err, ErrNoGame) {
		slog.Info("screenshot ignored: window does not belong to a running game")
		slog.Debug("screenshot attribution", "error", err)
		return
	}
	if err != nil {
		slog.Warn("capture screenshot", "error", err)
		s.emit(EventFailed, Failed{GameID: saved.GameID, Error: err.Error()})
		return
	}
	s.emit(EventSaved, saved)
}

// capture returns ErrNoGame, and stores nothing, when the window does not
// belong to a running game: a key press in a browser must not leave a picture
// of it in the library.
func (s *Service) capture(ctx context.Context, hwnd uintptr) (Saved, error) {
	root := s.config().ScreenshotsPath
	if root == "" {
		return Saved{}, ErrNoLibrary
	}
	if err := ctx.Err(); err != nil {
		return Saved{}, err
	}
	pid, err := s.pidOf(hwnd)
	if err != nil {
		return Saved{}, err
	}
	gameID, err := s.pickGame(ctx, pid)
	if err != nil {
		return Saved{}, fmt.Errorf("pid %d: %w", pid, err)
	}
	shot, err := s.grab(hwnd)
	if err != nil {
		return Saved{}, err
	}
	if err := ctx.Err(); err != nil {
		return Saved{}, err
	}
	at := s.now()

	s.saveMu.Lock()
	defer s.saveMu.Unlock()
	path, err := saveImage(root, gameID, shot.img, at)
	if err != nil {
		return Saved{GameID: gameID}, err
	}
	return Saved{GameID: gameID, Title: s.title(gameID), Path: path, Name: filepath.Base(path), At: at}, nil
}

// pickGame tries the window's own process first, then its parents (a game
// that unpacks and starts its real executable from a temp folder owns the
// window in a child of the process the session was bound to), then the one
// running game whose install dir holds the window's executable. Every refusal
// carries the reason and the executable, because the log is the only place a
// user can see why a key press left no picture.
func (s *Service) pickGame(ctx context.Context, pid uint32) (string, error) {
	if id, ok := s.games.RunningGameForPID(pid); ok {
		return id, nil
	}
	var (
		exe     string
		exeErr  error
		exeRead bool
	)
	imageOf := func() (string, error) {
		if !exeRead {
			exe, exeErr = s.exeOf(ctx, pid)
			exeRead = true
		}
		return exe, exeErr
	}
	refuse := func(reason string, cause error) error {
		err := fmt.Errorf("%w: %s", ErrNoGame, reason)
		if cause != nil {
			err = fmt.Errorf("%w: %w", err, cause)
		}
		path, pathErr := imageOf()
		if pathErr != nil {
			return fmt.Errorf("%w: image of the window process: %w", err, pathErr)
		}
		return fmt.Errorf("%w (exe %q)", err, path)
	}

	parents, parentsErr := s.parents(ctx)
	if parentsErr == nil {
		for _, ancestor := range procs.Ancestors(parents, pid) {
			if id, ok := s.games.RunningGameForPID(ancestor); ok {
				return id, nil
			}
		}
	}
	running := s.games.RunningGames()
	if len(running) != 1 {
		return "", refuse(fmt.Sprintf("%d games running, none owns the window process or its parents", len(running)), parentsErr)
	}
	dir, ok := s.games.InstallDir(running[0])
	if !ok || dir == "" {
		return "", refuse("install dir of the running game is unknown", parentsErr)
	}
	path, err := imageOf()
	if err != nil {
		return "", refuse("image of the window process is unreadable", parentsErr)
	}
	if !insideDir(dir, path) {
		return "", refuse("window process is outside the install dir of the running game", parentsErr)
	}
	return running[0], nil
}

func insideDir(dir, path string) bool {
	if dir == "" || path == "" || !filepath.IsAbs(dir) || !filepath.IsAbs(path) {
		return false
	}
	rel, err := filepath.Rel(filepath.Clean(dir), filepath.Clean(path))
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

//wails:ignore
func (s *Service) SetArmer(arm func(bool)) {
	s.armMu.Lock()
	defer s.armMu.Unlock()
	s.arm = arm
}

// SessionStarted and SessionStopped make the service a library session
// watcher. The library calls them under its own lock, so they must not call
// back into it.
//
//wails:ignore
func (s *Service) SessionStarted(game library.Game) {
	s.armMu.Lock()
	defer s.armMu.Unlock()
	before := len(s.active)
	s.active[game.ID] = struct{}{}
	if before == 0 && s.arm != nil {
		s.arm(true)
	}
}

//wails:ignore
func (s *Service) SessionStopped(gameID string) {
	s.armMu.Lock()
	defer s.armMu.Unlock()
	if _, ok := s.active[gameID]; !ok {
		return
	}
	delete(s.active, gameID)
	if len(s.active) == 0 && s.arm != nil {
		s.arm(false)
	}
}

func (s *Service) title(gameID string) string {
	s.mu.Lock()
	titleOf := s.titleOf
	s.mu.Unlock()
	if titleOf == nil {
		return ""
	}
	title, ok := titleOf(gameID)
	if !ok {
		return ""
	}
	return title
}

func saveImage(root, gameID string, img *image.RGBA, at time.Time) (string, error) {
	if !validSegment(gameID) {
		return "", fmt.Errorf("%w: %q", ErrBadName, gameID)
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return "", fmt.Errorf("%w: encode png: %w", ErrWrite, err)
	}
	info, err := os.Stat(root)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", fmt.Errorf("%w: %w", ErrNoLibrary, err)
		}
		return "", fmt.Errorf("%w: stat %s: %w", ErrWrite, root, err)
	}
	dir := filepath.Join(root, gameID)
	mode := info.Mode().Perm()
	if err := os.MkdirAll(dir, mode); err != nil {
		return "", fmt.Errorf("%w: create %s: %w", ErrWrite, dir, err)
	}
	if err := os.Chmod(dir, mode); err != nil {
		return "", fmt.Errorf("%w: chmod %s: %w", ErrWrite, dir, err)
	}
	path, err := freeName(dir, at)
	if err != nil {
		return "", err
	}
	if err := storage.WriteAtomic(path, buf.Bytes()); err != nil {
		return "", fmt.Errorf("%w: %w", ErrWrite, err)
	}
	return path, nil
}

func freeName(dir string, at time.Time) (string, error) {
	base := at.Format(nameLayout)
	for i := 0; i < maxSameName; i++ {
		name := base + pngExt
		if i > 0 {
			name = fmt.Sprintf("%s_%d%s", base, i+1, pngExt)
		}
		path := filepath.Join(dir, name)
		_, err := os.Stat(path)
		if errors.Is(err, fs.ErrNotExist) {
			return path, nil
		}
		if err != nil {
			return "", fmt.Errorf("%w: stat %s: %w", ErrWrite, path, err)
		}
	}
	return "", fmt.Errorf("%w: no free name in %s for %s", ErrWrite, dir, base)
}

func validSegment(name string) bool {
	if name == "" || len(name) > maxSegment || name == "." {
		return false
	}
	if strings.Contains(name, "..") || strings.ContainsAny(name, "/\\:") {
		return false
	}
	for _, r := range name {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}

func validShotName(name string) bool {
	return validSegment(name) && strings.EqualFold(filepath.Ext(name), pngExt) && !strings.HasPrefix(name, ".")
}

func (s *Service) root() (string, error) {
	root := s.config().ScreenshotsPath
	if root == "" {
		return "", ErrNoLibrary
	}
	return root, nil
}

func (s *Service) List(gameID string) ([]Shot, error) {
	if !validSegment(gameID) {
		return nil, fmt.Errorf("%w: %q", ErrBadName, gameID)
	}
	root, err := s.root()
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(root, gameID)
	info, err := os.Stat(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return []Shot{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("stat screenshots of %s: %w", gameID, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("screenshots of %s: %s is not a folder", gameID, dir)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read screenshots of %s: %w", gameID, err)
	}
	shots := make([]Shot, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !validShotName(entry.Name()) {
			continue
		}
		info, err := entry.Info()
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("stat screenshot %s: %w", entry.Name(), err)
		}
		if !info.Mode().IsRegular() {
			continue
		}
		shots = append(shots, Shot{
			Name: entry.Name(),
			Path: filepath.Join(dir, entry.Name()),
			URL:  URLPrefix + url.PathEscape(gameID) + "/" + url.PathEscape(entry.Name()),
			Size: info.Size(),
			At:   info.ModTime(),
		})
	}
	sort.Slice(shots, func(i, j int) bool {
		if !shots[i].At.Equal(shots[j].At) {
			return shots[i].At.After(shots[j].At)
		}
		return shots[i].Name > shots[j].Name
	})
	return shots, nil
}

func (s *Service) Delete(gameID, name string) error {
	if !validSegment(gameID) || !validShotName(name) {
		return fmt.Errorf("%w: %q/%q", ErrBadName, gameID, name)
	}
	root, err := s.root()
	if err != nil {
		return err
	}
	path := filepath.Join(root, gameID, name)
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("delete screenshot %s: %w", name, err)
	}
	return nil
}

func (s *Service) OpenFolder(gameID string) error {
	if !validSegment(gameID) {
		return fmt.Errorf("%w: %q", ErrBadName, gameID)
	}
	root, err := s.root()
	if err != nil {
		return err
	}
	dir := filepath.Join(root, gameID)
	info, err := os.Stat(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return ErrNoShots
	}
	if err != nil {
		return fmt.Errorf("stat %s: %w", dir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%w: %s is not a folder", ErrNoShots, dir)
	}
	return s.openDir(dir)
}

func (s *Service) Status() Status {
	cfg := s.config()
	return Status{Enabled: cfg.ScreenshotsEnabled, Hotkey: cfg.ScreenshotHotkey, Dir: cfg.ScreenshotsPath}
}
