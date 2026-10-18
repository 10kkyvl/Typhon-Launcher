package library

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func externalFixture(t *testing.T) (ExternalGame, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "Game One")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	return ExternalGame{
		Provider:        ProviderSteam,
		AppID:           "42",
		Title:           "Game One",
		InstallDir:      dir,
		LaunchURI:       "steam://rungameid/42",
		CanonicalGameID: "cat-1",
	}, dir
}

func TestApplyExternalAddsAndUpdates(t *testing.T) {
	s := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))
	item, dir := externalFixture(t)

	sum, err := s.ApplyExternal(context.Background(), []ExternalGame{item})
	if err != nil {
		t.Fatal(err)
	}
	if sum.Added != 1 || sum.Updated != 0 {
		t.Fatalf("summary = %+v, want one added", sum)
	}
	games := s.GetInstalledGames()
	if len(games) != 1 {
		t.Fatalf("games = %d, want 1", len(games))
	}
	g := games[0]
	if g.Source != SourceExternal || g.External == nil || g.External.AppID != "42" || g.External.Provider != ProviderSteam {
		t.Fatalf("stored game = %+v", g)
	}
	if g.CanonicalGameID != "cat-1" || g.InstallDir != dir || g.Executable != "" || g.InstalledAt.IsZero() {
		t.Fatalf("stored game = %+v", g)
	}

	again, err := s.ApplyExternal(context.Background(), []ExternalGame{item})
	if err != nil {
		t.Fatal(err)
	}
	if again.Added != 0 || again.Updated != 0 {
		t.Fatalf("repeat summary = %+v, want no changes", again)
	}

	item.Title = "Game One Renamed"
	renamed, err := s.ApplyExternal(context.Background(), []ExternalGame{item})
	if err != nil {
		t.Fatal(err)
	}
	if renamed.Updated != 1 || renamed.Added != 0 {
		t.Fatalf("rename summary = %+v", renamed)
	}
	games = mustServiceAt(t, s.path).GetInstalledGames()
	if len(games) != 1 || games[0].Title != "Game One Renamed" || games[0].ID != g.ID {
		t.Fatalf("reloaded = %+v", games)
	}
}

func TestApplyExternalKeepsRecordsTheScannerNoLongerSees(t *testing.T) {
	s := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))
	item, _ := externalFixture(t)
	if _, err := s.ApplyExternal(context.Background(), []ExternalGame{item}); err != nil {
		t.Fatal(err)
	}
	sum, err := s.ApplyExternal(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Added != 0 || len(s.GetInstalledGames()) != 1 {
		t.Fatalf("summary = %+v, games = %d", sum, len(s.GetInstalledGames()))
	}
}

func TestApplyExternalRejectsBadInput(t *testing.T) {
	base, dir := externalFixture(t)
	file := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		edit func(*ExternalGame)
		want error
	}{
		{"empty install dir", func(g *ExternalGame) { g.InstallDir = "" }, errEmptyInstallDir},
		{"relative install dir", func(g *ExternalGame) { g.InstallDir = "games/one" }, ErrExternalDir},
		{"missing install dir", func(g *ExternalGame) { g.InstallDir = filepath.Join(dir, "nope") }, ErrExternalDir},
		{"install dir is a file", func(g *ExternalGame) { g.InstallDir = file }, ErrExternalDir},
		{"unknown provider", func(g *ExternalGame) { g.Provider = "origin" }, ErrExternalProvider},
		{"empty app id", func(g *ExternalGame) { g.AppID = " " }, ErrExternalAppID},
		{"empty title", func(g *ExternalGame) { g.Title = "" }, ErrExternalTitle},
		{"no way to launch", func(g *ExternalGame) { g.LaunchURI = "" }, ErrExternalNoLaunch},
		{"file uri", func(g *ExternalGame) { g.LaunchURI = "file:///C:/Windows/notepad.exe" }, ErrExternalURIScheme},
		{"executable outside", func(g *ExternalGame) { g.Executable = filepath.Join(filepath.Dir(dir), "x.exe") }, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))
			item := base
			tc.edit(&item)
			_, err := s.ApplyExternal(context.Background(), []ExternalGame{item})
			if err == nil {
				t.Fatal("expected an error")
			}
			if tc.want != nil && !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
			if len(s.GetGames()) != 0 {
				t.Fatalf("library changed after rejected input: %+v", s.GetGames())
			}
		})
	}
}

func TestApplyExternalRollsBackWhenPersistFails(t *testing.T) {
	s := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))
	item, _ := externalFixture(t)
	if _, err := s.ApplyExternal(context.Background(), []ExternalGame{item}); err != nil {
		t.Fatal(err)
	}
	blockPersist(t, s)

	item.Title = "Changed"
	other, _ := externalFixture(t)
	other.AppID = "43"
	if _, err := s.ApplyExternal(context.Background(), []ExternalGame{item, other}); err == nil {
		t.Fatal("expected persist failure")
	}
	games := s.GetGames()
	if len(games) != 1 || games[0].Title != "Game One" {
		t.Fatalf("memory after failed persist = %+v, want the original single record", games)
	}
}

func TestApplyExternalSkipsDirectoryOfExistingGame(t *testing.T) {
	s := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))
	item, dir := externalFixture(t)
	exe := testPlaceExecutable(t, filepath.Join(dir, "game.exe"))
	if _, err := s.AddGame(exe, "Manual"); err != nil {
		t.Fatal(err)
	}
	sum, err := s.ApplyExternal(context.Background(), []ExternalGame{item})
	if err != nil {
		t.Fatal(err)
	}
	if sum.Added != 0 || len(sum.Skipped) != 1 || len(s.GetGames()) != 1 {
		t.Fatalf("summary = %+v, games = %d", sum, len(s.GetGames()))
	}
}

func TestApplyExternalHonoursCancelledContext(t *testing.T) {
	s := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))
	item, _ := externalFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.ApplyExternal(ctx, []ExternalGame{item}); !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if len(s.GetGames()) != 0 {
		t.Fatal("library changed after cancelled apply")
	}
}

func TestPlayGameOpensExternalURI(t *testing.T) {
	cases := []struct {
		name    string
		uri     string
		opened  bool
		wantErr bool
	}{
		{"steam", "steam://rungameid/42", true, false},
		{"epic", "com.epicgames.launcher://apps/ns%3Aitem%3Aapp?action=launch&silent=true", true, false},
		{"upper case scheme", "STEAM://rungameid/42", false, true},
		{"steam install", "steam://install/42", false, true},
		{"steam openurl", "steam://openurl/https://example.com", false, true},
		{"space", "steam://rungameid/42 -applaunch", false, true},
		{"too long", "com.epicgames.launcher://apps/" + strings.Repeat("a", 600) + "%3Ab%3Ac?action=launch&silent=true", false, true},
		{"epic other action", "com.epicgames.launcher://apps/a%3Ab%3Ac?action=install", false, true},
		{"steam huge id", "steam://rungameid/1234567890123", false, true},
		{"file", "file:///C:/Windows/System32/cmd.exe", false, true},
		{"http", "https://example.com/", false, true},
		{"bare scheme", "steam://", false, true},
		{"line break", "steam://rungameid/42\r\nfile:///x", false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))
			item, _ := externalFixture(t)
			if _, err := s.ApplyExternal(context.Background(), []ExternalGame{item}); err != nil {
				t.Fatal(err)
			}
			id := s.GetGames()[0].ID
			s.mu.Lock()
			s.findLocked(id).External.LaunchURI = tc.uri
			s.mu.Unlock()
			var opened []string
			s.openURI = func(uri string) error {
				opened = append(opened, uri)
				return nil
			}
			err := s.PlayGame(id)
			if (err != nil) != tc.wantErr {
				t.Fatalf("PlayGame error = %v, wantErr %v", err, tc.wantErr)
			}
			if tc.opened != (len(opened) == 1) {
				t.Fatalf("opened = %v, want opened=%v", opened, tc.opened)
			}
			if len(s.GetRunningGames()) != 0 {
				t.Fatal("external launch must not create a session")
			}
		})
	}
}

func TestPlayGameReturnsOpenFailure(t *testing.T) {
	s := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))
	item, _ := externalFixture(t)
	if _, err := s.ApplyExternal(context.Background(), []ExternalGame{item}); err != nil {
		t.Fatal(err)
	}
	boom := errors.New("shell refused")
	s.openURI = func(string) error { return boom }
	var noted string
	s.onLaunchFail = func(_, code, _ string) { noted = code }
	if err := s.PlayGame(s.GetGames()[0].ID); !errors.Is(err, boom) {
		t.Fatalf("PlayGame error = %v, want %v", err, boom)
	}
	s.wg.Wait()
	if noted != "library.launch_failed" {
		t.Fatalf("launch failure code = %q", noted)
	}
}

func TestPlayGameExternalWithoutURIUsesExecutable(t *testing.T) {
	s := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))
	item, dir := externalFixture(t)
	item.LaunchURI = ""
	item.Executable = testPlaceExecutable(t, filepath.Join(dir, "game.exe"))
	if _, err := s.ApplyExternal(context.Background(), []ExternalGame{item}); err != nil {
		t.Fatal(err)
	}
	called := false
	s.openURI = func(string) error { called = true; return nil }
	s.mu.Lock()
	s.findLocked(s.games[0].ID).LaunchArgs = nil
	s.mu.Unlock()
	if err := os.Remove(item.Executable); err != nil {
		t.Fatal(err)
	}
	err := s.PlayGame(s.GetGames()[0].ID)
	if err == nil || called {
		t.Fatalf("err = %v, uri opened = %v: executable path must be used when no URI is set", err, called)
	}
}

func TestPlayGameExternalRejectsDoubleLaunch(t *testing.T) {
	s := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))
	item, _ := externalFixture(t)
	if _, err := s.ApplyExternal(context.Background(), []ExternalGame{item}); err != nil {
		t.Fatal(err)
	}
	id := s.GetGames()[0].ID
	entered := make(chan struct{})
	release := make(chan struct{})
	var mu sync.Mutex
	opens := 0
	s.openURI = func(string) error {
		mu.Lock()
		opens++
		mu.Unlock()
		close(entered)
		<-release
		return nil
	}
	done := make(chan error, 1)
	go func() { done <- s.PlayGame(id) }()
	<-entered
	if err := s.PlayGame(id); err == nil {
		t.Fatal("second PlayGame during launch must be refused")
	}
	if err := s.RemoveGame(id); err == nil {
		t.Fatal("RemoveGame during launch must be refused")
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if opens != 1 {
		t.Fatalf("opens = %d, want 1", opens)
	}
	if ids := s.GetRunningGames(); len(ids) != 0 {
		t.Fatalf("starting marker leaked: %v", ids)
	}
}

func TestRemovedExternalGameCanBeImportedAgain(t *testing.T) {
	s := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))
	item, _ := externalFixture(t)
	if _, err := s.ApplyExternal(context.Background(), []ExternalGame{item}); err != nil {
		t.Fatal(err)
	}
	if err := s.RemoveGame(s.GetGames()[0].ID); err != nil {
		t.Fatal(err)
	}
	if len(s.excluded) != 0 {
		t.Fatalf("excluded = %v: a foreign launcher directory must not be excluded", s.excluded)
	}
	sum, err := s.ApplyExternal(context.Background(), []ExternalGame{item})
	if err != nil {
		t.Fatal(err)
	}
	if sum.Added != 1 || len(sum.Skipped) != 0 {
		t.Fatalf("summary = %+v, want the game imported again", sum)
	}
}
