package extlib

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"typhon/internal/library"
)

func syncService(t *testing.T, lib *fakeLibrary, cat *fakeCatalog, scan func(context.Context) ([]ProviderScan, error)) *Service {
	t.Helper()
	s, err := NewService(lib, cat, &fakeMetadata{})
	if err != nil {
		t.Fatal(err)
	}
	s.scan = scan
	s.syncEnabled = false
	if err := s.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.ServiceShutdown(); err != nil {
			t.Errorf("shutdown: %v", err)
		}
	})
	return s
}

func steamRef(id string) library.ExternalRef {
	return library.ExternalRef{Provider: library.ProviderSteam, AppID: id}
}

func TestSyncOnceMarksOnlyAfterCleanScan(t *testing.T) {
	tests := []struct {
		name        string
		scan        ProviderScan
		wantPresent bool
		wantIDs     []string
	}{
		{"clean scan", ProviderScan{Provider: library.ProviderSteam, Games: []Candidate{steamCandidate("1", "A")}}, true, []string{"1"}},
		{"skipped game counts as present", ProviderScan{Provider: library.ProviderSteam, Games: []Candidate{steamCandidate("1", "A")}, Skipped: []Skipped{{Title: "B", Reason: "x", AppID: "2"}}}, true, []string{"1", "2"}},
		{"skipped without id makes scan incomplete", ProviderScan{Provider: library.ProviderSteam, Games: []Candidate{steamCandidate("1", "A")}, Skipped: []Skipped{{Title: "lib", Reason: "x"}}}, false, nil},
		{"scan error", ProviderScan{Provider: library.ProviderSteam, Err: errors.New("denied")}, false, nil},
		{"not installed", ProviderScan{Provider: library.ProviderSteam, Err: ErrNotInstalled}, false, nil},
		{"unsupported", ProviderScan{Provider: library.ProviderSteam, Err: ErrUnsupported}, false, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			lib := &fakeLibrary{refs: []library.ExternalRef{steamRef("1")}}
			s := syncService(t, lib, &fakeCatalog{}, func(context.Context) ([]ProviderScan, error) {
				return []ProviderScan{tc.scan}, nil
			})
			err := s.syncOnce(context.Background())
			if tc.scan.Err != nil && !errors.Is(tc.scan.Err, ErrNotInstalled) && !errors.Is(tc.scan.Err, ErrUnsupported) && err == nil {
				t.Error("provider scan error must be returned")
			}
			got := lib.syncPresent[library.ProviderSteam]
			if tc.wantPresent != (got != nil) {
				t.Fatalf("present = %v, want present=%v", lib.syncPresent, tc.wantPresent)
			}
			for _, id := range tc.wantIDs {
				if !got[id] {
					t.Errorf("present misses %s: %v", id, got)
				}
			}
			if len(got) != len(tc.wantIDs) {
				t.Errorf("present = %v, want %v", got, tc.wantIDs)
			}
		})
	}
}

func TestSyncOnceSkipsScanWithoutExternalGames(t *testing.T) {
	lib := &fakeLibrary{}
	scans := 0
	s := syncService(t, lib, &fakeCatalog{}, func(context.Context) ([]ProviderScan, error) {
		scans++
		return nil, nil
	})
	if err := s.syncOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if scans != 0 || lib.syncCalls != 0 {
		t.Fatalf("scans = %d, sync calls = %d, want none", scans, lib.syncCalls)
	}
}

func TestSyncOnceIgnoresProvidersWithoutImportedGames(t *testing.T) {
	lib := &fakeLibrary{refs: []library.ExternalRef{steamRef("1")}}
	s := syncService(t, lib, &fakeCatalog{}, func(context.Context) ([]ProviderScan, error) {
		return []ProviderScan{
			{Provider: library.ProviderSteam, Games: []Candidate{steamCandidate("1", "A")}},
			{Provider: library.ProviderEpic, Games: []Candidate{{Provider: library.ProviderEpic, AppID: "e1", Title: "Epic Game", InstallDir: t.TempDir()}}},
		}, nil
	})
	if err := s.syncOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, it := range lib.syncItems {
		if it.Provider == library.ProviderEpic {
			t.Fatalf("epic game added without prior import: %+v", it)
		}
	}
	if _, ok := lib.syncPresent[library.ProviderEpic]; ok {
		t.Fatal("epic provider took part in marking")
	}
}

func TestSyncOnceBindsOnlyNewCandidatesAndOnlyOnce(t *testing.T) {
	lib := &fakeLibrary{refs: []library.ExternalRef{steamRef("1")}}
	cat := &fakeCatalog{}
	s := syncService(t, lib, cat, func(context.Context) ([]ProviderScan, error) {
		return []ProviderScan{{Provider: library.ProviderSteam, Games: []Candidate{steamCandidate("1", "Known"), steamCandidate("2", "Fresh")}}}, nil
	})
	for range 2 {
		if err := s.syncOnce(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if len(cat.provision) != 1 || cat.provision[0].Normalized != "fresh" {
		t.Fatalf("provisioned = %+v, want only the fresh game, once", cat.provision)
	}
	if len(cat.resolved) != 1 {
		t.Fatalf("resolved %d times, want 1", len(cat.resolved))
	}
	if len(lib.syncItems) != 2 {
		t.Fatalf("items = %+v, want known and fresh", lib.syncItems)
	}
}

func TestSyncOnceStillAppliesWhenProvisionFails(t *testing.T) {
	lib := &fakeLibrary{refs: []library.ExternalRef{steamRef("1")}}
	cat := &fakeCatalog{err: errors.New("offline")}
	s := syncService(t, lib, cat, func(context.Context) ([]ProviderScan, error) {
		return []ProviderScan{{Provider: library.ProviderSteam, Games: []Candidate{steamCandidate("2", "Fresh")}}}, nil
	})
	if err := s.syncOnce(context.Background()); err == nil {
		t.Fatal("provision failure must be returned")
	}
	if lib.syncCalls != 1 || len(lib.syncItems) != 0 || lib.syncPresent[library.ProviderSteam] == nil {
		t.Fatalf("calls = %d, items = %+v, present = %v", lib.syncCalls, lib.syncItems, lib.syncPresent)
	}
}

func TestSyncOnceReturnsLibraryError(t *testing.T) {
	lib := &fakeLibrary{refs: []library.ExternalRef{steamRef("1")}, syncErr: errors.New("disk full")}
	s := syncService(t, lib, &fakeCatalog{}, func(context.Context) ([]ProviderScan, error) {
		return []ProviderScan{{Provider: library.ProviderSteam}}, nil
	})
	if err := s.syncOnce(context.Background()); !errors.Is(err, lib.syncErr) {
		t.Fatalf("err = %v, want the library error", err)
	}
}

func TestSyncTickSkipsWhileImportRuns(t *testing.T) {
	lib := &fakeLibrary{refs: []library.ExternalRef{steamRef("1")}}
	s := syncService(t, lib, &fakeCatalog{}, func(context.Context) ([]ProviderScan, error) {
		return []ProviderScan{{Provider: library.ProviderSteam}}, nil
	})
	if _, err := s.begin(); err != nil {
		t.Fatal(err)
	}
	if err := s.syncTick(context.Background()); err != nil || lib.syncCalls != 0 {
		t.Fatalf("err = %v, calls = %d: busy tick must be skipped", err, lib.syncCalls)
	}
	s.finish()
	if err := s.syncTick(context.Background()); err != nil || lib.syncCalls != 1 {
		t.Fatalf("err = %v, calls = %d", err, lib.syncCalls)
	}
}

func TestImportDuringSyncIsBusy(t *testing.T) {
	lib := &fakeLibrary{refs: []library.ExternalRef{steamRef("1")}}
	entered, release := make(chan struct{}), make(chan struct{})
	s := syncService(t, lib, &fakeCatalog{}, func(context.Context) ([]ProviderScan, error) {
		close(entered)
		<-release
		return []ProviderScan{{Provider: library.ProviderSteam}}, nil
	})
	done := make(chan error, 1)
	go func() { done <- s.syncTick(context.Background()) }()
	<-entered
	if _, err := s.Import(); !errors.Is(err, ErrBusy) {
		t.Fatalf("Import during sync = %v, want ErrBusy", err)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestSyncErrorIsLoggedOnlyOnChange(t *testing.T) {
	s := &Service{}
	s.noteSyncError(errors.New("a"))
	s.noteSyncError(errors.New("a"))
	if s.lastSyncErr != "a" {
		t.Fatalf("lastSyncErr = %q", s.lastSyncErr)
	}
	s.noteSyncError(nil)
	if s.lastSyncErr != "" {
		t.Fatalf("lastSyncErr = %q, want reset", s.lastSyncErr)
	}
}

func TestSyncLoopRunsOnTimerAndShutdownWaitsForIt(t *testing.T) {
	lib := &fakeLibrary{refs: []library.ExternalRef{steamRef("1")}}
	scanned := make(chan struct{}, 4)
	s, err := NewService(lib, &fakeCatalog{}, &fakeMetadata{})
	if err != nil {
		t.Fatal(err)
	}
	s.scan = func(context.Context) ([]ProviderScan, error) {
		scanned <- struct{}{}
		return []ProviderScan{{Provider: library.ProviderSteam}}, nil
	}
	delays := make(chan time.Duration, 4)
	ticks := make(chan time.Time)
	s.timer = func(d time.Duration) (<-chan time.Time, func() bool) {
		delays <- d
		return ticks, func() bool { return true }
	}
	s.syncEnabled = true
	if err := s.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatal(err)
	}
	if d := <-delays; d != firstSyncDelay {
		t.Fatalf("first delay = %v", d)
	}
	ticks <- time.Time{}
	<-scanned
	if d := <-delays; d != syncInterval {
		t.Fatalf("next delay = %v", d)
	}
	if err := s.ServiceShutdown(); err != nil {
		t.Fatal(err)
	}
	if len(scanned) != 0 {
		t.Fatal("scan ran after shutdown")
	}
}

func TestSyncLoopStopsOnUnsupported(t *testing.T) {
	lib := &fakeLibrary{refs: []library.ExternalRef{steamRef("1")}}
	s, err := NewService(lib, &fakeCatalog{}, &fakeMetadata{})
	if err != nil {
		t.Fatal(err)
	}
	s.scan = func(context.Context) ([]ProviderScan, error) { return nil, ErrUnsupported }
	ticks := make(chan time.Time, 1)
	armed := make(chan struct{}, 4)
	s.timer = func(time.Duration) (<-chan time.Time, func() bool) {
		armed <- struct{}{}
		return ticks, func() bool { return true }
	}
	s.syncEnabled = true
	ticks <- time.Time{}
	if err := s.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatal(err)
	}
	s.wg.Wait()
	if err := s.ServiceShutdown(); err != nil {
		t.Fatal(err)
	}
	if len(armed) != 1 {
		t.Fatalf("timers armed = %d, want 1: loop must stop after unsupported", len(armed))
	}
}

func TestSyncDisabledStartsNoGoroutine(t *testing.T) {
	s, err := NewService(&fakeLibrary{}, &fakeCatalog{}, &fakeMetadata{})
	if err != nil {
		t.Fatal(err)
	}
	s.timer = func(time.Duration) (<-chan time.Time, func() bool) {
		t.Error("timer armed with sync disabled")
		return nil, func() bool { return true }
	}
	s.syncEnabled = false
	if err := s.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := s.ServiceShutdown(); err != nil {
		t.Fatal(err)
	}
}
