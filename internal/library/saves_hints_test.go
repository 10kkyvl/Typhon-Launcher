package library

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	"typhon/internal/platform"
	"typhon/internal/steamlink"
)

var testHintSource = SaveHintSource{Source: "PCGamingWiki", License: "CC BY-NC-SA 3.0", Via: "Ludusavi Manifest", PageURL: "https://www.pcgamingwiki.com/wiki/Prey"}

type hintFixture struct {
	s       *Service
	game    Game
	root    string
	calls   int
	hints   []string
	hintErr error
	steam   string
	steamEr error
}

func newHintFixture(t *testing.T, title string) *hintFixture {
	t.Helper()
	root := t.TempDir()
	mkdirs(t, root, "install", "Roaming", "Docs")
	heur := t.TempDir()
	f := &hintFixture{root: root}
	f.s = savesService(t, []platform.SaveRoot{{Path: heur, Depth: 1}})
	f.game = addSavesGame(t, f.s, title, filepath.Join(root, "install"))
	f.s.mu.Lock()
	f.s.games[0].CanonicalGameID = "canon-1"
	f.s.mu.Unlock()
	f.s.saveEnvs = func() ([]platform.SaveEnv, error) {
		return []platform.SaveEnv{{
			platform.TokenAppData:   filepath.Join(root, "Roaming"),
			platform.TokenDocuments: filepath.Join(root, "Docs"),
		}}, nil
	}
	f.s.steamRoot = func() (string, error) { return f.steam, f.steamEr }
	f.s.SetSaveHints(func(_ context.Context, id string) (SaveHintSet, error) {
		f.calls++
		if id != "canon-1" {
			t.Errorf("hint lookup id = %q", id)
		}
		return SaveHintSet{Templates: f.hints, Source: testHintSource}, f.hintErr
	})
	f.s.saveRoots = func() ([]platform.SaveRoot, error) { return []platform.SaveRoot{{Path: heur, Depth: 1}}, nil }
	mkdirs(t, heur, title)
	return f
}

func TestLocateSavesHintBeatsHeuristics(t *testing.T) {
	f := newHintFixture(t, "Prey")
	mkdirs(t, f.root, "Roaming/Arkane/Prey")
	f.hints = []string{"{appdata}/Arkane/Prey"}

	result, err := f.s.LocateSaves(context.Background(), f.game.ID)
	if err != nil {
		t.Fatalf("locate: %v", err)
	}
	want := filepath.Join(f.root, "Roaming", "Arkane", "Prey")
	if result.Path != want || len(result.Candidates) != 0 {
		t.Fatalf("result = %+v, want the hinted folder %q and no heuristic candidates", result, want)
	}
	if result.Hint == nil || *result.Hint != testHintSource {
		t.Fatalf("hint = %+v, want the server attribution", result.Hint)
	}
}

func TestLocateSavesHintsWithSeveralFoldersAreAmbiguous(t *testing.T) {
	f := newHintFixture(t, "Prey")
	mkdirs(t, f.root, "Docs/My Games/A/Saves", "Docs/My Games/B/Saves")
	f.hints = []string{"{documents}/My Games/*/Saves"}

	result, err := f.s.LocateSaves(context.Background(), f.game.ID)
	if err != nil {
		t.Fatalf("locate: %v", err)
	}
	if result.Path != "" || len(result.Candidates) != 2 {
		t.Fatalf("result = %+v, want two candidates and no path", result)
	}
	if result.Hint == nil || result.Hint.Via != "Ludusavi Manifest" {
		t.Fatalf("hint = %+v, want attribution on hinted candidates", result.Hint)
	}
}

func TestLocateSavesNoExistingHintFallsBackToHeuristics(t *testing.T) {
	f := newHintFixture(t, "Prey")
	f.hints = []string{"{appdata}/Missing", "{steam}/steamapps/x"}

	result, err := f.s.LocateSaves(context.Background(), f.game.ID)
	if err != nil {
		t.Fatalf("locate: %v", err)
	}
	if result.Path == "" || result.HintsError != "" {
		t.Fatalf("result = %+v, want the heuristic folder without hint error", result)
	}
	if result.Hint != nil {
		t.Fatalf("hint = %+v, heuristic result must carry no attribution", result.Hint)
	}
}

func TestLocateSavesProviderErrorIsReportedNotHidden(t *testing.T) {
	f := newHintFixture(t, "Prey")
	f.hintErr = errors.New("server down")

	result, err := f.s.LocateSaves(context.Background(), f.game.ID)
	if err != nil {
		t.Fatalf("locate must go on to heuristics: %v", err)
	}
	if result.Path == "" {
		t.Fatalf("heuristics did not run: %+v", result)
	}
	if result.HintsError != "server down" {
		t.Fatalf("HintsError = %q, want the provider failure", result.HintsError)
	}
}

func TestLocateSavesGameTokenUsesInstallDir(t *testing.T) {
	f := newHintFixture(t, "Prey")
	mkdirs(t, f.root, "install/profile")
	f.hints = []string{"{game}/profile"}

	result, err := f.s.LocateSaves(context.Background(), f.game.ID)
	if err != nil {
		t.Fatal(err)
	}
	if result.Path != filepath.Join(f.root, "install", "profile") {
		t.Fatalf("result = %+v", result)
	}
}

func TestLocateSavesSteamTokenNeedsSteam(t *testing.T) {
	f := newHintFixture(t, "Prey")
	steam := filepath.Join(f.root, "Steam")
	mkdirs(t, f.root, "Steam/userdata/1/480")
	f.hints = []string{"{steam}/userdata/1/480"}

	f.steamEr = steamlink.ErrNotInstalled
	result, err := f.s.LocateSaves(context.Background(), f.game.ID)
	if err != nil || result.HintsError != "" {
		t.Fatalf("not installed steam must only skip the hint: %+v %v", result, err)
	}
	if result.Path == filepath.Join(steam, "userdata", "1", "480") {
		t.Fatal("hint resolved without a steam root")
	}

	f.steamEr = nil
	f.steam = steam
	result, err = f.s.LocateSaves(context.Background(), f.game.ID)
	if err != nil || result.Path != filepath.Join(steam, "userdata", "1", "480") {
		t.Fatalf("with steam root: %+v %v", result, err)
	}

	f.steamEr = errors.New("registry unreadable")
	if _, err := f.s.LocateSaves(context.Background(), f.game.ID); err == nil {
		t.Fatal("a real steam root failure was swallowed")
	}
}

func TestLocateSavesHintFailuresPropagate(t *testing.T) {
	f := newHintFixture(t, "Prey")
	f.hints = []string{"{appdata}/x"}
	f.s.saveEnvs = func() ([]platform.SaveEnv, error) { return nil, errors.New("no known folders") }
	if _, err := f.s.LocateSaves(context.Background(), f.game.ID); err == nil {
		t.Fatal("known folder failure swallowed")
	}

	f.s.saveEnvs = func() ([]platform.SaveEnv, error) { return []platform.SaveEnv{{platform.TokenAppData: f.root}}, nil }
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := f.s.LocateSaves(ctx, f.game.ID); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled ctx err = %v", err)
	}
}

func TestLocateSavesSkipsHintsForGameWithoutCatalogID(t *testing.T) {
	f := newHintFixture(t, "Prey")
	f.s.mu.Lock()
	f.s.games[0].CanonicalGameID = ""
	f.s.mu.Unlock()
	f.hints = []string{"{appdata}"}

	result, err := f.s.LocateSaves(context.Background(), f.game.ID)
	if err != nil || f.calls != 0 {
		t.Fatalf("calls = %d err = %v", f.calls, err)
	}
	if result.Path == filepath.Join(f.root, "Roaming") {
		t.Fatal("hint used without a catalog id")
	}
}

func TestLocateSavesStoredDirCarriesNoHint(t *testing.T) {
	f := newHintFixture(t, "Prey")
	f.hints = []string{"{appdata}/Arkane/Prey"}
	mkdirs(t, f.root, "Roaming/Arkane/Prey", "picked")
	picked := filepath.Join(f.root, "picked")
	if _, err := f.s.SetSavesDir(f.game.ID, picked); err != nil {
		t.Fatal(err)
	}
	result, err := f.s.LocateSaves(context.Background(), f.game.ID)
	if err != nil || result.Path != picked || result.Hint != nil || f.calls != 0 {
		t.Fatalf("result %+v err %v calls %d: SavesDir must win without hints", result, err, f.calls)
	}
}

func TestLocateSavesKeepsHintFailureOnPartialSuccess(t *testing.T) {
	f := newHintFixture(t, "Prey")
	mkdirs(t, f.root, "Roaming/Arkane/Prey")
	for i := range 300 {
		mkdirs(t, f.root, fmt.Sprintf("Docs/d%03d", i))
	}
	f.hints = []string{"{documents}/*/x", "{appdata}/Arkane/Prey"}

	result, err := f.s.LocateSaves(context.Background(), f.game.ID)
	if err != nil {
		t.Fatalf("locate: %v", err)
	}
	if result.Path != filepath.Join(f.root, "Roaming", "Arkane", "Prey") {
		t.Fatalf("result = %+v", result)
	}
	if result.HintsError == "" {
		t.Fatal("the too-broad template failure was lost although another hint succeeded")
	}
}
