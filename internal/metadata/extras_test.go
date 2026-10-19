package metadata

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"typhon/internal/catalog"
)

type saveFake struct {
	*fakeProvider
	mu    sync.Mutex
	calls int
	paths SavePaths
	err   error
	gate  chan struct{}
}

func (p *saveFake) SavePaths(_ context.Context, _ string) (SavePaths, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	gate, err, paths := p.gate, p.err, p.paths
	p.mu.Unlock()
	if gate != nil {
		<-gate
	}
	p.mu.Lock()
	if err != nil {
		return SavePaths{}, err
	}
	return paths, nil
}

func (p *saveFake) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}

func steamMeta() *SteamInfo {
	score := 87
	return &SteamInfo{
		AppID:        480,
		URL:          "https://store.steampowered.com/app/480",
		Requirements: SteamRequirements{Minimum: []Requirement{{Label: "OS", Value: "Windows 10"}}},
		DLC:          []SteamDLC{{AppID: 481, Name: "Expansion"}},
		Controller:   ControllerFull,
		Metacritic:   &score,
	}
}

func resolvedGame(t *testing.T, svc *Service, cat *catalog.Service, meta GameMetadata) catalog.Game {
	t.Helper()
	game := addGame(t, cat, catalog.Game{Title: "Prey", Provisional: true})
	if _, err := svc.ApplyMatch(game.ID, meta.ProviderID); err != nil {
		t.Fatalf("apply match: %v", err)
	}
	return game
}

func TestViewCarriesSteamBlockAndSurvivesRestart(t *testing.T) {
	srv := imageServer(t)
	meta := fullMetadata(srv.URL)
	meta.Steam = steamMeta()
	provider := &fakeProvider{meta: map[string]GameMetadata{"2657": meta}}
	svc, cat, dir := newTestService(t, provider)
	game := resolvedGame(t, svc, cat, meta)

	view, err := svc.GetView(game.ID)
	if err != nil {
		t.Fatalf("get view: %v", err)
	}
	if view.Steam == nil || view.Steam.AppID != 480 || len(view.Steam.DLC) != 1 {
		t.Fatalf("view steam = %+v", view.Steam)
	}

	again, err := NewServiceAt(dir, cat, provider)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if got := again.view(game).Steam; got == nil || got.AppID != 480 {
		t.Fatalf("steam block lost after restart: %+v", got)
	}
}

func TestViewWithoutSteamBlockFromOldBackend(t *testing.T) {
	srv := imageServer(t)
	provider := &fakeProvider{meta: map[string]GameMetadata{"2657": fullMetadata(srv.URL)}}
	svc, cat, _ := newTestService(t, provider)
	game := resolvedGame(t, svc, cat, fullMetadata(srv.URL))

	view, err := svc.GetView(game.ID)
	if err != nil || view.Steam != nil {
		t.Fatalf("view steam = %+v err = %v, want nil block and no error", view.Steam, err)
	}
}

func TestEnsureFreshRefetchesGameStoredBeforeSteamBlock(t *testing.T) {
	srv := imageServer(t)
	meta := fullMetadata(srv.URL)
	meta.Steam = steamMeta()
	provider := &fakeProvider{meta: map[string]GameMetadata{"2657": meta}}
	svc, cat, _ := newTestService(t, provider)
	game := resolvedGame(t, svc, cat, meta)

	started, err := svc.EnsureFresh(game.ID)
	if err != nil || started {
		t.Fatalf("fresh game with a stored entry: started=%v err=%v", started, err)
	}

	svc.steam.mu.Lock()
	delete(svc.steam.entries, game.ID)
	svc.steam.introducedAt = time.Now().Add(time.Hour)
	svc.steam.mu.Unlock()

	_, getsBefore := provider.counts()
	started, err = svc.EnsureFresh(game.ID)
	if err != nil || !started {
		t.Fatalf("game without a steam entry must be refreshed: started=%v err=%v", started, err)
	}
	svc.wg.Wait()
	if _, gets := provider.counts(); gets != getsBefore+1 {
		t.Fatalf("gets = %d, want %d", gets, getsBefore+1)
	}
	if _, ok := svc.steam.get(game.ID); !ok {
		t.Fatal("refresh did not record the steam entry")
	}
}

func TestEmptySteamEntryIsNotRefetchedForever(t *testing.T) {
	srv := imageServer(t)
	provider := &fakeProvider{meta: map[string]GameMetadata{"2657": fullMetadata(srv.URL)}}
	svc, cat, _ := newTestService(t, provider)
	game := resolvedGame(t, svc, cat, fullMetadata(srv.URL))

	started, err := svc.EnsureFresh(game.ID)
	if err != nil || started {
		t.Fatalf("old backend answer must count as fetched: started=%v err=%v", started, err)
	}
}

func TestCorruptSteamInfoFileRefusesToStart(t *testing.T) {
	dir := t.TempDir()
	cat, err := catalog.NewServiceAt(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, steamInfoFileName), []byte("{broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewServiceAt(dir, cat, nil); err == nil {
		t.Fatal("service started over a corrupt steam info file")
	}
}

func saveService(t *testing.T, p *saveFake) (*Service, catalog.Game, *time.Time) {
	t.Helper()
	svc, cat, _ := newTestService(t, p)
	now := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	svc.clock = func() time.Time { return now }
	game := addGame(t, cat, catalog.Game{Title: "Prey", ExternalIDs: catalog.ExternalIDs{IGDB: "2657"}})
	return svc, game, &now
}

func goodSaves() SavePaths {
	return SavePaths{
		PageURL: "https://www.pcgamingwiki.com/wiki/Prey",
		Windows: []string{"{appdata}/Arkane/Prey"},
		Source:  "PCGamingWiki",
		License: "CC BY-NC-SA 3.0",
		Via:     "Ludusavi Manifest",
	}
}

func TestSavePathsCachesOnDisk(t *testing.T) {
	p := &saveFake{fakeProvider: &fakeProvider{}, paths: goodSaves()}
	svc, game, _ := saveService(t, p)

	for range 2 {
		got, err := svc.SavePaths(context.Background(), game.ID)
		if err != nil || got.PageURL != goodSaves().PageURL || got.Via != "Ludusavi Manifest" {
			t.Fatalf("got %+v err %v", got, err)
		}
	}
	if p.count() != 1 {
		t.Fatalf("provider calls = %d, want 1 (second answer from cache)", p.count())
	}

	again, err := NewServiceAt(filepath.Dir(svc.saves.path), svc.catalog, p)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	again.clock = svc.clock
	if _, err := again.SavePaths(context.Background(), game.ID); err != nil || p.count() != 1 {
		t.Fatalf("after restart: err %v calls %d", err, p.count())
	}
}

func TestSavePathsRefetchesAfterTTL(t *testing.T) {
	p := &saveFake{fakeProvider: &fakeProvider{}, paths: goodSaves()}
	svc, game, now := saveService(t, p)

	if _, err := svc.SavePaths(context.Background(), game.ID); err != nil {
		t.Fatal(err)
	}
	*now = now.Add(savesTTL - time.Hour)
	if _, err := svc.SavePaths(context.Background(), game.ID); err != nil || p.count() != 1 {
		t.Fatalf("inside TTL: err %v calls %d", err, p.count())
	}
	*now = now.Add(2 * time.Hour)
	if _, err := svc.SavePaths(context.Background(), game.ID); err != nil || p.count() != 2 {
		t.Fatalf("past TTL: err %v calls %d", err, p.count())
	}
}

func TestSavePathsMissingIsCachedBriefly(t *testing.T) {
	p := &saveFake{fakeProvider: &fakeProvider{}, err: ErrNoSaveData}
	svc, game, now := saveService(t, p)

	for range 2 {
		if _, err := svc.SavePaths(context.Background(), game.ID); !errors.Is(err, ErrNoSaveData) {
			t.Fatalf("err = %v, want ErrNoSaveData", err)
		}
	}
	if p.count() != 1 {
		t.Fatalf("calls = %d, want 1", p.count())
	}
	*now = now.Add(savesMissingTTL + time.Hour)
	if _, err := svc.SavePaths(context.Background(), game.ID); !errors.Is(err, ErrNoSaveData) || p.count() != 2 {
		t.Fatalf("after missing TTL: err %v calls %d", err, p.count())
	}
}

func TestSavePathsTransientFailureIsNotCached(t *testing.T) {
	boom := errors.New("server down")
	p := &saveFake{fakeProvider: &fakeProvider{}, err: boom}
	svc, game, now := saveService(t, p)

	if _, err := svc.SavePaths(context.Background(), game.ID); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want boom", err)
	}
	*now = now.Add(savesFailureTTL + time.Second)
	p.mu.Lock()
	p.err = nil
	p.paths = goodSaves()
	p.mu.Unlock()
	if got, err := svc.SavePaths(context.Background(), game.ID); err != nil || got.PageURL == "" {
		t.Fatalf("recovery: %+v %v", got, err)
	}
}

func TestSavePathsRejectsInvalidProviderAnswer(t *testing.T) {
	bad := goodSaves()
	bad.PageURL = "https://evil.example/wiki/Prey"
	p := &saveFake{fakeProvider: &fakeProvider{}, paths: bad}
	svc, game, _ := saveService(t, p)

	if _, err := svc.SavePaths(context.Background(), game.ID); !errors.Is(err, ErrSaveDataInvalid) {
		t.Fatalf("err = %v, want ErrSaveDataInvalid", err)
	}
	if _, ok := svc.saves.peek("2657"); ok {
		t.Fatal("invalid answer was cached")
	}
}

func TestSavePathsPersistFailureIsReturnedAndRolledBack(t *testing.T) {
	p := &saveFake{fakeProvider: &fakeProvider{}, paths: goodSaves()}
	svc, game, _ := saveService(t, p)
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	svc.saves.path = filepath.Join(blocker, savesFileName)

	if _, err := svc.SavePaths(context.Background(), game.ID); err == nil {
		t.Fatal("persist failure swallowed")
	}
	if _, ok := svc.saves.peek("2657"); ok {
		t.Fatal("memory kept the entry after the write failed")
	}
}

func TestCorruptSavesCacheRefusesToStart(t *testing.T) {
	dir := t.TempDir()
	cat, err := catalog.NewServiceAt(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, savesFileName), []byte("{broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewServiceAt(dir, cat, nil); err == nil {
		t.Fatal("service started over a corrupt save paths cache")
	}
}

func TestSavePathsUnresolvedGameAndProviderWithoutSupport(t *testing.T) {
	svc, cat, _ := newTestService(t, &fakeProvider{})
	plain := addGame(t, cat, catalog.Game{Title: "Local"})
	if _, err := svc.SavePaths(context.Background(), plain.ID); !errors.Is(err, ErrGameNotResolved) {
		t.Fatalf("err = %v, want ErrGameNotResolved", err)
	}
	resolved := addGame(t, cat, catalog.Game{Title: "R", ExternalIDs: catalog.ExternalIDs{Steam: "10"}})
	if _, err := svc.SavePaths(context.Background(), resolved.ID); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("err = %v, want ErrNotConfigured", err)
	}
	if _, err := svc.SavePaths(context.Background(), " "); !errors.Is(err, errNoGameID) {
		t.Fatalf("err = %v, want errNoGameID", err)
	}
}

func TestValidateSavePathsLimits(t *testing.T) {
	long := "{game}/" + strings.Repeat("a", 300)
	in := goodSaves()
	in.Windows = []string{long, "{game}", "{game}/*", "{appdata}/*/*", "{steam}/steamapps/x", "{localappdatalow}/a/*/b"}
	out, err := ValidateSavePaths(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Windows) != 2 {
		t.Fatalf("windows = %v, want overlong and whole-root paths dropped", out.Windows)
	}
}

func linkService(t *testing.T) (*Service, catalog.Game, *[]string) {
	t.Helper()
	srv := imageServer(t)
	meta := fullMetadata(srv.URL)
	meta.Steam = steamMeta()
	p := &saveFake{fakeProvider: &fakeProvider{meta: map[string]GameMetadata{"2657": meta}}, paths: goodSaves()}
	svc, cat, _ := newTestService(t, p)
	game := resolvedGame(t, svc, cat, meta)
	var opened []string
	svc.openURL = func(u string) error { opened = append(opened, u); return nil }
	return svc, game, &opened
}

func TestOpenReferenceLinkKinds(t *testing.T) {
	svc, game, opened := linkService(t)
	if _, err := svc.SavePaths(context.Background(), game.ID); err != nil {
		t.Fatal(err)
	}

	if err := svc.OpenReferenceLink(game.ID, LinkPCGamingWiki, 0); err != nil {
		t.Fatalf("pcgamingwiki: %v", err)
	}
	if err := svc.OpenReferenceLink(game.ID, LinkHowLongToBeat, 0); err != nil {
		t.Fatalf("hltb: %v", err)
	}
	if err := svc.OpenReferenceLink(game.ID, LinkSteamDLC, 481); err != nil {
		t.Fatalf("dlc: %v", err)
	}
	want := []string{
		"https://www.pcgamingwiki.com/wiki/Prey",
		"https://howlongtobeat.com/?q=Prey",
		"https://store.steampowered.com/app/481",
	}
	if len(*opened) != 3 || (*opened)[0] != want[0] || (*opened)[1] != want[1] || (*opened)[2] != want[2] {
		t.Fatalf("opened %v, want %v", *opened, want)
	}
}

func TestOpenReferenceLinkEscapesTitle(t *testing.T) {
	svc, _, opened := linkService(t)
	odd := addGame(t, svc.catalog, catalog.Game{Title: "Half-Life & Co/#1 ?x=y"})
	if err := svc.OpenReferenceLink(odd.ID, LinkHowLongToBeat, 0); err != nil {
		t.Fatal(err)
	}
	if got := (*opened)[0]; got != "https://howlongtobeat.com/?q=Half-Life+%26+Co%2F%231+%3Fx%3Dy" {
		t.Fatalf("opened %q", got)
	}
}

func TestOpenReferenceLinkRefusals(t *testing.T) {
	svc, game, opened := linkService(t)

	cases := []struct {
		name  string
		id    string
		kind  string
		appID int64
		want  error
	}{
		{"empty game", " ", LinkHowLongToBeat, 0, errNoGameID},
		{"unknown kind", game.ID, "youtube", 0, errLinkKind},
		{"empty kind", game.ID, "", 0, errLinkKind},
		{"pcgw without cached page", game.ID, LinkPCGamingWiki, 0, errLinkNoData},
		{"dlc not of this game", game.ID, LinkSteamDLC, 999, errLinkNoData},
		{"dlc zero", game.ID, LinkSteamDLC, 0, errLinkNoData},
		{"dlc negative", game.ID, LinkSteamDLC, -481, errLinkNoData},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := svc.OpenReferenceLink(tc.id, tc.kind, tc.appID); !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}
	if len(*opened) != 0 {
		t.Fatalf("a refused request opened %v", *opened)
	}
	if err := svc.OpenReferenceLink("no-such-game", LinkHowLongToBeat, 0); !errors.Is(err, catalog.ErrNoGame) {
		t.Fatalf("unknown game err = %v", err)
	}
}

func TestOpenReferenceLinkSteamDLCWithoutBlock(t *testing.T) {
	srv := imageServer(t)
	provider := &fakeProvider{meta: map[string]GameMetadata{"2657": fullMetadata(srv.URL)}}
	svc, cat, _ := newTestService(t, provider)
	game := resolvedGame(t, svc, cat, fullMetadata(srv.URL))
	svc.openURL = func(string) error { t.Fatal("opened"); return nil }
	if err := svc.OpenReferenceLink(game.ID, LinkSteamDLC, 481); !errors.Is(err, errLinkNoData) {
		t.Fatalf("err = %v, want errLinkNoData", err)
	}
}

func TestAllowedReferenceURL(t *testing.T) {
	ok := []string{
		"https://www.pcgamingwiki.com/wiki/Prey",
		"https://howlongtobeat.com/?q=Prey+2",
		"https://store.steampowered.com/app/480",
	}
	bad := []string{
		"http://www.pcgamingwiki.com/wiki/Prey",
		"https://pcgamingwiki.com/wiki/Prey",
		"https://www.pcgamingwiki.com/",
		"https://howlongtobeat.com/",
		"https://howlongtobeat.com/?q=",
		"https://howlongtobeat.com/other?q=x",
		"https://store.steampowered.com/app/abc",
		"https://store.steampowered.com/app/480?curator=1",
		"https://steamcommunity.com/app/480",
		"javascript:alert(1)",
		"file:///C:/Windows",
		"https://user@howlongtobeat.com/?q=x",
	}
	for _, u := range ok {
		if !allowedReferenceURL(u) {
			t.Errorf("%s rejected", u)
		}
	}
	for _, u := range bad {
		if allowedReferenceURL(u) {
			t.Errorf("%s accepted", u)
		}
	}
}

func TestSavePathsTransientFailureIsRememberedForFiveMinutes(t *testing.T) {
	boom := errors.New("server down")
	p := &saveFake{fakeProvider: &fakeProvider{}, err: boom}
	svc, game, now := saveService(t, p)

	for range 3 {
		if _, err := svc.SavePaths(context.Background(), game.ID); !errors.Is(err, boom) {
			t.Fatalf("err = %v, want the remembered failure", err)
		}
	}
	if p.count() != 1 {
		t.Fatalf("provider calls = %d, want 1 inside the failure window", p.count())
	}
	*now = now.Add(savesFailureTTL + time.Second)
	if _, err := svc.SavePaths(context.Background(), game.ID); !errors.Is(err, boom) || p.count() != 2 {
		t.Fatalf("after the window: err %v calls %d", err, p.count())
	}
}

func TestSavePathsConcurrentCallsShareOneRequest(t *testing.T) {
	const callers = 8
	gate := make(chan struct{})
	p := &saveFake{fakeProvider: &fakeProvider{}, paths: goodSaves(), gate: gate}
	svc, game, _ := saveService(t, p)

	var wg sync.WaitGroup
	errs := make(chan error, callers)
	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := svc.SavePaths(context.Background(), game.ID)
			errs <- err
		}()
	}
	deadline := time.Now().Add(10 * time.Second)
	for p.count() != 1 || svc.saves.waiting("2657") != callers-1 {
		if time.Now().After(deadline) {
			t.Fatalf("callers did not join one flight: provider calls %d, waiting %d", p.count(), svc.saves.waiting("2657"))
		}
		runtime.Gosched()
	}
	close(gate)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("shared call failed: %v", err)
		}
	}
	if p.count() != 1 {
		t.Fatalf("provider calls = %d, want 1", p.count())
	}
}

func TestBrokenSteamBlockDoesNotReplaceValidOne(t *testing.T) {
	srv := imageServer(t)
	meta := fullMetadata(srv.URL)
	meta.Steam = steamMeta()
	provider := &fakeProvider{meta: map[string]GameMetadata{"2657": meta}}
	svc, cat, _ := newTestService(t, provider)
	game := resolvedGame(t, svc, cat, meta)

	broken := fullMetadata(srv.URL)
	broken.Steam = &SteamInfo{AppID: 0, URL: "https://store.steampowered.com/app/0"}
	provider.mu.Lock()
	provider.meta["2657"] = broken
	provider.mu.Unlock()
	view, err := svc.Refresh(game.ID)
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if view.Steam == nil || view.Steam.AppID != 480 {
		t.Fatalf("valid stored block replaced by a broken one: %+v", view.Steam)
	}
}
