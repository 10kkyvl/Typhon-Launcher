package discovery

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"

	"typhon/internal/library"
	"typhon/internal/platform"

	"github.com/wailsapp/wails/v3/pkg/application"
)

type fakeLibrary struct {
	mu      sync.Mutex
	games   []library.Game
	applied []library.Discovered
	failOn  string

	entered chan struct{}
	release chan struct{}
	gated   bool
}

// GetInstalledGames пропускает через шлюз только первый вызов: он нужен, чтобы
// удержать сканирование в известной точке, а не чтобы блокировать сервис.
func (f *fakeLibrary) GetInstalledGames() []library.Game {
	f.mu.Lock()
	hold := f.entered != nil && !f.gated
	f.gated = true
	entered, release := f.entered, f.release
	games := append([]library.Game(nil), f.games...)
	f.mu.Unlock()

	if hold {
		entered <- struct{}{}
		<-release
	}
	return games
}

func (f *fakeLibrary) ApplyDiscovered(d library.Discovered) (library.Game, library.Outcome, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.applied = append(f.applied, d)
	if f.failOn != "" && platform.SamePath(f.failOn, d.InstallDir) {
		return library.Game{}, "", errors.New("сохранить не удалось")
	}
	game := library.Game{ID: d.InstallDir, Title: d.Title, InstallDir: d.InstallDir, CanonicalGameID: d.CanonicalGameID}
	f.games = append(f.games, game)
	return game, library.OutcomeCreated, nil
}

type fakeMetadata struct {
	mu   sync.Mutex
	ids  []string
	fail error
}

func (f *fakeMetadata) EnsureFresh(gameID string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ids = append(f.ids, gameID)
	if f.fail != nil {
		return false, f.fail
	}
	return true, nil
}

func TestScanRejectsSecondRun(t *testing.T) {
	settingsService, root, _ := configuredSettings(t)
	installedGame(t, root, "Portal")
	lib := &fakeLibrary{entered: make(chan struct{}), release: make(chan struct{})}
	svc := start(t, settingsService, lib, newCatalog(t), nil)

	done := make(chan error, 1)
	go func() {
		_, err := svc.Scan()
		done <- err
	}()

	<-lib.entered
	if !svc.Scanning() {
		t.Fatal("сервис должен сообщать о работающем поиске")
	}
	if _, err := svc.Scan(); !errors.Is(err, ErrBusy) {
		t.Fatalf("second scan error = %v, want ErrBusy", err)
	}
	close(lib.release)

	if err := <-done; err != nil {
		t.Fatalf("scan: %v", err)
	}
	if svc.Scanning() {
		t.Fatal("после завершения поиск не должен считаться активным")
	}
	if _, err := svc.Scan(); err != nil {
		t.Fatalf("повторный запуск после завершения: %v", err)
	}
}

func TestCancelScanStopsWalk(t *testing.T) {
	settingsService, root, _ := configuredSettings(t)
	installedGame(t, root, "Portal")
	installedGame(t, root, "Doom")
	lib := &fakeLibrary{entered: make(chan struct{}), release: make(chan struct{})}
	svc := start(t, settingsService, lib, newCatalog(t), nil)

	if err := svc.CancelScan(); !errors.Is(err, errNoScan) {
		t.Fatalf("cancel without a scan = %v, want errNoScan", err)
	}

	type outcome struct {
		result Result
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		result, err := svc.Scan()
		done <- outcome{result, err}
	}()

	<-lib.entered
	if err := svc.CancelScan(); err != nil {
		t.Fatalf("cancel scan: %v", err)
	}
	close(lib.release)

	got := <-done
	if got.err != nil {
		t.Fatalf("scan: %v", got.err)
	}
	if !got.result.Cancelled {
		t.Fatalf("result = %+v, want cancelled", got.result)
	}
	if got.result.Added != 0 {
		t.Fatalf("added = %d, want no work after the cancel", got.result.Added)
	}
}

func TestScanStopsWhenServiceContextEnds(t *testing.T) {
	settingsService, root, config := configuredSettings(t)
	installedGame(t, root, "Portal")
	lib, err := library.NewServiceAt(filepath.Join(config, "library.json"))
	if err != nil {
		t.Fatalf("library service: %v", err)
	}
	svc, err := NewService(settingsService, lib, newCatalog(t), nil)
	if err != nil {
		t.Fatalf("discovery service: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	if err := svc.ServiceStartup(ctx, application.ServiceOptions{}); err != nil {
		t.Fatalf("startup: %v", err)
	}
	cancel()

	result, err := svc.Scan()
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if !result.Cancelled || result.Added != 0 {
		t.Fatalf("result = %+v, want a cancelled scan without changes", result)
	}
	if games := lib.GetInstalledGames(); len(games) != 0 {
		t.Fatalf("games = %+v, want none", games)
	}
	if err := svc.ServiceShutdown(); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	if _, err := svc.Scan(); !errors.Is(err, errNotStarted) {
		t.Fatalf("scan after shutdown = %v, want errNotStarted", err)
	}
}

func TestScanContinuesAfterCandidateFailure(t *testing.T) {
	settingsService, root, _ := configuredSettings(t)
	broken := installedGame(t, root, "Portal")
	installedGame(t, root, "Doom")
	lib := &fakeLibrary{failOn: broken}
	svc := start(t, settingsService, lib, newCatalog(t), nil)

	result, err := svc.Scan()
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if result.Errors != 1 {
		t.Fatalf("errors = %d, want the single failing candidate (%+v)", result.Errors, result.Problems)
	}
	if result.Added != 1 {
		t.Fatalf("added = %d, want the other game still registered", result.Added)
	}
	if len(result.Problems) != 1 || result.Problems[0].Path != broken {
		t.Fatalf("problems = %+v, want %q reported", result.Problems, broken)
	}
}

func TestScanRequestsMetadataForNewGames(t *testing.T) {
	settingsService, root, config := configuredSettings(t)
	installedGame(t, root, "Portal")
	lib, err := library.NewServiceAt(filepath.Join(config, "library.json"))
	if err != nil {
		t.Fatalf("library service: %v", err)
	}
	meta := &fakeMetadata{}
	svc := start(t, settingsService, lib, newCatalog(t), meta)

	if _, err := svc.Scan(); err != nil {
		t.Fatalf("scan: %v", err)
	}

	games := lib.GetInstalledGames()
	if len(games) != 1 || games[0].CanonicalGameID == "" {
		t.Fatalf("games = %+v, want one game with a canonical id", games)
	}
	meta.mu.Lock()
	defer meta.mu.Unlock()
	if len(meta.ids) != 1 || meta.ids[0] != games[0].CanonicalGameID {
		t.Fatalf("metadata ids = %v, want %q", meta.ids, games[0].CanonicalGameID)
	}
}

func TestScanKeepsInstallWhenMetadataFails(t *testing.T) {
	settingsService, root, config := configuredSettings(t)
	installedGame(t, root, "Portal")
	lib, err := library.NewServiceAt(filepath.Join(config, "library.json"))
	if err != nil {
		t.Fatalf("library service: %v", err)
	}
	meta := &fakeMetadata{fail: errors.New("провайдер недоступен")}
	svc := start(t, settingsService, lib, newCatalog(t), meta)

	result, err := svc.Scan()
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if result.Added != 1 {
		t.Fatalf("result = %+v, want the install registered anyway", result)
	}
	if games := lib.GetInstalledGames(); len(games) != 1 {
		t.Fatalf("games = %+v, want the discovered install kept", games)
	}
}

func TestNewServiceRequiresDependencies(t *testing.T) {
	settingsService, _, config := configuredSettings(t)
	lib, err := library.NewServiceAt(filepath.Join(config, "library.json"))
	if err != nil {
		t.Fatalf("library service: %v", err)
	}
	cases := []struct {
		name string
		set  bool
		lib  gameLibrary
		cat  gameCatalog
	}{
		{"без настроек", false, lib, newCatalog(t)},
		{"без библиотеки", true, nil, newCatalog(t)},
		{"без каталога", true, lib, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var svc *Service
			var err error
			if tc.set {
				svc, err = NewService(settingsService, tc.lib, tc.cat, nil)
			} else {
				svc, err = NewService(nil, tc.lib, tc.cat, nil)
			}
			if err == nil {
				t.Fatalf("service = %+v, want an error", svc)
			}
		})
	}
}

func TestScanBeforeStartup(t *testing.T) {
	settingsService, _, config := configuredSettings(t)
	lib, err := library.NewServiceAt(filepath.Join(config, "library.json"))
	if err != nil {
		t.Fatalf("library service: %v", err)
	}
	svc, err := NewService(settingsService, lib, newCatalog(t), nil)
	if err != nil {
		t.Fatalf("discovery service: %v", err)
	}
	if _, err := svc.Scan(); !errors.Is(err, errNotStarted) {
		t.Fatalf("scan = %v, want errNotStarted", err)
	}
}

func TestConcurrentScansLeaveOneWinner(t *testing.T) {
	settingsService, root, config := configuredSettings(t)
	installedGame(t, root, "Portal")
	installedGame(t, root, "Doom")
	lib, err := library.NewServiceAt(filepath.Join(config, "library.json"))
	if err != nil {
		t.Fatalf("library service: %v", err)
	}
	svc := start(t, settingsService, lib, newCatalog(t), nil)

	const runs = 4
	var wg sync.WaitGroup
	results := make([]error, runs)
	start := make(chan struct{})
	for i := range runs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			_, err := svc.Scan()
			results[i] = err
		}(i)
	}
	close(start)
	wg.Wait()

	busy := 0
	for _, err := range results {
		if errors.Is(err, ErrBusy) {
			busy++
			continue
		}
		if err != nil {
			t.Fatalf("scan: %v", err)
		}
	}
	if busy == 0 {
		t.Fatal("параллельные запуски должны получать busy")
	}
	if games := lib.GetInstalledGames(); len(games) != 2 {
		t.Fatalf("games = %d, want 2 without duplicates", len(games))
	}
}
