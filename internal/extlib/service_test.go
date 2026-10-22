package extlib

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"

	"typhon/internal/catalog"
	"typhon/internal/library"
	"typhon/internal/uierr"
)

type fakeLibrary struct {
	mu      sync.Mutex
	applied []library.ExternalGame
	err     error

	refs        []library.ExternalRef
	syncCalls   int
	syncItems   []library.ExternalGame
	syncPresent map[string]map[string]bool
	syncErr     error
	removed     []library.ExternalRef
	removeErr   error
}

func (f *fakeLibrary) RemoveExternal(refs []library.ExternalRef) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.removed = append(f.removed, refs...)
	return len(refs), f.removeErr
}

func (f *fakeLibrary) ExternalRefs() []library.ExternalRef {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]library.ExternalRef(nil), f.refs...)
}

func (f *fakeLibrary) SyncExternal(_ context.Context, items []library.ExternalGame, present map[string]map[string]bool) (library.ExternalSummary, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.syncCalls++
	f.syncItems = items
	f.syncPresent = present
	if f.syncErr != nil {
		return library.ExternalSummary{}, f.syncErr
	}
	sum := library.ExternalSummary{}
	for _, it := range items {
		sum.Added++
		sum.Outcomes = append(sum.Outcomes, library.ExternalOutcome{Provider: it.Provider, AppID: it.AppID, Added: true})
	}
	return sum, nil
}

func (f *fakeLibrary) ApplyExternal(_ context.Context, items []library.ExternalGame) (library.ExternalSummary, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return library.ExternalSummary{}, f.err
	}
	f.applied = append(f.applied, items...)
	sum := library.ExternalSummary{}
	for _, it := range items {
		sum.Added++
		sum.Outcomes = append(sum.Outcomes, library.ExternalOutcome{Provider: it.Provider, AppID: it.AppID, Added: true})
	}
	return sum, nil
}

type fakeCatalog struct {
	resolved  []catalog.Query
	provision []catalog.Query
	matchSet  map[string]string
	none      bool
	err       error
}

func (f *fakeCatalog) Resolve(q catalog.Query) catalog.Match {
	f.resolved = append(f.resolved, q)
	if id, ok := f.matchSet[q.ExternalIDs.Steam]; ok && q.ExternalIDs.Steam != "" {
		return catalog.Match{Status: catalog.StatusMatched, GameID: id}
	}
	return catalog.Match{Status: catalog.StatusUnmatched}
}

func (f *fakeCatalog) Provision(qs []catalog.Query) (map[string]catalog.Game, error) {
	f.provision = append(f.provision, qs...)
	if f.err != nil {
		return nil, f.err
	}
	out := map[string]catalog.Game{}
	if f.none {
		return out, nil
	}
	for _, q := range qs {
		out[q.Normalized] = catalog.Game{ID: "new-" + q.Normalized}
	}
	return out, nil
}

type fakeMetadata struct {
	mu  sync.Mutex
	ids []string
	err error
}

func (f *fakeMetadata) EnsureFresh(id string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ids = append(f.ids, id)
	return true, f.err
}

func started(t *testing.T, lib *fakeLibrary, cat *fakeCatalog, meta *fakeMetadata, scan func(context.Context) ([]ProviderScan, error)) *Service {
	t.Helper()
	s, err := NewService(lib, cat, meta)
	if err != nil {
		t.Fatal(err)
	}
	s.scan = scan
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

func steamCandidate(id, title string) Candidate {
	return Candidate{Provider: library.ProviderSteam, AppID: id, Title: title, InstallDir: os.TempDir(), LaunchURI: "steam://rungameid/" + id}
}

func TestNewServiceRequiresDependencies(t *testing.T) {
	if _, err := NewService(nil, &fakeCatalog{}, &fakeMetadata{}); err == nil {
		t.Error("nil library accepted")
	}
	if _, err := NewService(&fakeLibrary{}, nil, &fakeMetadata{}); err == nil {
		t.Error("nil catalog accepted")
	}
	if _, err := NewService(&fakeLibrary{}, &fakeCatalog{}, nil); err == nil {
		t.Error("nil metadata accepted")
	}
}

func TestImportBindsToCatalogAndRefreshesNewGames(t *testing.T) {
	lib, cat, meta := &fakeLibrary{}, &fakeCatalog{matchSet: map[string]string{"730": "cat-730"}}, &fakeMetadata{}
	s := started(t, lib, cat, meta, func(context.Context) ([]ProviderScan, error) {
		return []ProviderScan{
			{Provider: library.ProviderSteam, Games: []Candidate{steamCandidate("730", "Counter-Strike 2"), steamCandidate("9", "Unknown Game")},
				Skipped: []Skipped{{Title: "Broken", Reason: "bad"}}},
			{Provider: library.ProviderEpic, Err: ErrNotInstalled},
			{Provider: library.ProviderGOG, Err: errors.New("registry denied")},
		}, nil
	})

	sum, err := s.Import()
	if err != nil {
		t.Fatal(err)
	}
	if sum.Added != 2 || len(lib.applied) != 2 {
		t.Fatalf("summary = %+v, applied = %+v", sum, lib.applied)
	}
	if lib.applied[0].CanonicalGameID != "cat-730" || lib.applied[1].CanonicalGameID != "new-unknown game" {
		t.Fatalf("canonical ids = %q, %q", lib.applied[0].CanonicalGameID, lib.applied[1].CanonicalGameID)
	}
	if len(cat.resolved) != 2 || cat.resolved[0].ExternalIDs.Steam != "730" {
		t.Fatalf("resolve queries = %+v: the steam id must be passed to the catalog", cat.resolved)
	}
	if len(meta.ids) != 2 {
		t.Fatalf("EnsureFresh calls = %v, want both new games", meta.ids)
	}
	byName := map[string]ProviderResult{}
	for _, p := range sum.Providers {
		byName[p.Provider] = p
	}
	if st := byName[library.ProviderSteam]; st.Found != 2 || st.Added != 2 || len(st.Skipped) != 1 {
		t.Fatalf("steam result = %+v", st)
	}
	if ep := byName[library.ProviderEpic]; !ep.NotInstalled || ep.Error != "" {
		t.Fatalf("epic result = %+v", ep)
	}
	if gog := byName[library.ProviderGOG]; gog.NotInstalled || gog.Error == "" {
		t.Fatalf("gog result = %+v", gog)
	}
}

func TestImportMetadataFailureIsReportedNotFatal(t *testing.T) {
	meta := &fakeMetadata{err: errors.New("offline")}
	s := started(t, &fakeLibrary{}, &fakeCatalog{}, meta, func(context.Context) ([]ProviderScan, error) {
		return []ProviderScan{{Provider: library.ProviderSteam, Games: []Candidate{steamCandidate("1", "Game")}}}, nil
	})
	sum, err := s.Import()
	if err != nil {
		t.Fatal(err)
	}
	if len(sum.MetadataErrors) != 1 {
		t.Fatalf("metadata errors = %v", sum.MetadataErrors)
	}
}

func TestImportErrors(t *testing.T) {
	boom := errors.New("boom")
	cases := []struct {
		name    string
		scan    func(context.Context) ([]ProviderScan, error)
		lib     *fakeLibrary
		cat     *fakeCatalog
		wantErr error
	}{
		{"unsupported", func(context.Context) ([]ProviderScan, error) { return nil, ErrUnsupported }, &fakeLibrary{}, &fakeCatalog{}, errors.ErrUnsupported},
		{"every provider failed", func(context.Context) ([]ProviderScan, error) {
			return []ProviderScan{{Provider: "steam", Err: boom}, {Provider: "epic", Err: boom}}, nil
		}, &fakeLibrary{}, &fakeCatalog{}, boom},
		{"apply failed", func(context.Context) ([]ProviderScan, error) {
			return []ProviderScan{{Provider: "steam", Games: []Candidate{steamCandidate("1", "G")}}}, nil
		}, &fakeLibrary{err: boom}, &fakeCatalog{}, boom},
		{"catalog failed", func(context.Context) ([]ProviderScan, error) {
			return []ProviderScan{{Provider: "steam", Games: []Candidate{steamCandidate("1", "G")}}}, nil
		}, &fakeLibrary{}, &fakeCatalog{err: boom}, boom},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := started(t, tc.lib, tc.cat, &fakeMetadata{}, tc.scan)
			_, err := s.Import()
			if tc.name == "unsupported" && uierr.Code(err) != "extlib.unsupported" {
				t.Fatalf("code = %q, want extlib.unsupported", uierr.Code(err))
			}
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("error = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestImportNothingInstalledIsNotAnError(t *testing.T) {
	s := started(t, &fakeLibrary{}, &fakeCatalog{}, &fakeMetadata{}, func(context.Context) ([]ProviderScan, error) {
		return []ProviderScan{{Provider: "steam", Err: ErrNotInstalled}, {Provider: "epic", Err: ErrNotInstalled}}, nil
	})
	sum, err := s.Import()
	if err != nil {
		t.Fatal(err)
	}
	if sum.Added != 0 || !sum.Providers[0].NotInstalled || !sum.Providers[1].NotInstalled {
		t.Fatalf("summary = %+v", sum)
	}
}

func TestImportRefusedBeforeStartup(t *testing.T) {
	s, err := NewService(&fakeLibrary{}, &fakeCatalog{}, &fakeMetadata{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Import(); !errors.Is(err, errNotStarted) {
		t.Fatalf("error = %v, want errNotStarted", err)
	}
}

func TestImportRejectsConcurrentRun(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	s := started(t, &fakeLibrary{}, &fakeCatalog{}, &fakeMetadata{}, func(context.Context) ([]ProviderScan, error) {
		close(entered)
		<-release
		return nil, nil
	})
	var wg sync.WaitGroup
	wg.Add(1)
	var firstErr error
	go func() {
		defer wg.Done()
		_, firstErr = s.Import()
	}()
	<-entered
	if _, err := s.Import(); !errors.Is(err, ErrBusy) {
		t.Fatalf("second Import error = %v, want ErrBusy", err)
	}
	close(release)
	wg.Wait()
	if firstErr != nil {
		t.Fatalf("first Import: %v", firstErr)
	}
}
