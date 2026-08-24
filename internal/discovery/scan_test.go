package discovery

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"typhon/internal/catalog"
	"typhon/internal/library"
)

// makeLink делает ссылку тем механизмом, который реально встречается на
// платформе: на Windows каталоги связывают junction-ами.
func makeLink(t *testing.T, link, target string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		//nolint:gosec // G204: аргументы — пути из t.TempDir(); внешнего ввода, который требует валидации по инварианту 32, здесь нет
		out, err := exec.Command("cmd", "/c", "mklink", "/J", link, target).CombinedOutput()
		if err != nil {
			t.Skipf("junction недоступен: %v (%s)", err, out)
		}
		return
	}
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("символические ссылки недоступны: %v", err)
	}
}

func TestScanUsesConfiguredRoot(t *testing.T) {
	f := newFixture(t)
	installedGame(t, f.root, "Portal")
	outside := filepath.Join(filepath.Dir(f.settings.GetSettings().LibraryPath), "Elsewhere")
	installedGame(t, outside, "Doom")

	result := f.scan(t)

	if result.Roots != 1 || result.Candidates != 1 || result.Added != 1 {
		t.Fatalf("result = %+v, want one root with one added game", result)
	}
	for _, game := range f.lib.GetInstalledGames() {
		if game.Title == "Doom" {
			t.Fatal("сканирование вышло за пределы настроенного корня")
		}
	}
}

func TestScanWithoutConfiguredLibrary(t *testing.T) {
	f := newFixture(t)
	next := f.settings.GetSettings()
	next.LibraryPath = ""
	if err := f.settings.SaveSettings(next); err != nil {
		t.Fatalf("save settings: %v", err)
	}
	if _, err := f.svc.Scan(); err == nil {
		t.Fatal("без настроенной библиотеки сканирование должно возвращать ошибку")
	}
}

func TestScanSkipsMissingRoot(t *testing.T) {
	f := newFixture(t)
	if err := os.RemoveAll(f.root); err != nil {
		t.Fatal(err)
	}

	result := f.scan(t)

	if result.Roots != 0 || result.RootsSkipped != 1 {
		t.Fatalf("result = %+v, want the missing root skipped", result)
	}
	if result.Errors != 0 {
		t.Fatalf("errors = %d, want none for a missing root", result.Errors)
	}
}

func TestNormalizeRootsDedupesAndRejectsEmpty(t *testing.T) {
	base := t.TempDir()
	first := filepath.Join(base, "Games")
	second := filepath.Join(base, "More")

	roots, err := normalizeRoots([]string{first, "", first + string(filepath.Separator), second})
	if err != nil {
		t.Fatalf("normalize roots: %v", err)
	}
	if len(roots) != 2 || roots[0] != first || roots[1] != second {
		t.Fatalf("roots = %v, want %v and %v once each", roots, first, second)
	}
	if _, err := normalizeRoots([]string{"", "  "}); err == nil {
		t.Fatal("пустой список корней должен быть ошибкой")
	}
}

func TestScanDiscoversAndKeepsKnownGames(t *testing.T) {
	f := newFixture(t)
	known := installedGame(t, f.root, "Portal")
	installedGame(t, f.root, "Doom")
	if _, err := f.lib.RegisterInstalled(library.InstalledGame{
		Title:      "Portal",
		Executable: filepath.Join(known, "Portal.exe"),
		InstallDir: known,
	}); err != nil {
		t.Fatalf("register installed: %v", err)
	}

	result := f.scan(t)

	if result.Added != 1 || result.Known != 1 || result.Updated != 0 {
		t.Fatalf("result = %+v, want one added and one known", result)
	}
	if games := f.lib.GetInstalledGames(); len(games) != 2 {
		t.Fatalf("games = %d, want 2", len(games))
	}
	doom := f.game(t, "Doom")
	if doom.Source != library.SourceDiscovered {
		t.Fatalf("source = %q, want %q", doom.Source, library.SourceDiscovered)
	}
	if doom.SizeBytes <= 0 {
		t.Fatalf("size = %d, want the measured install size", doom.SizeBytes)
	}
}

func TestRepeatScanIsIdempotent(t *testing.T) {
	f := newFixture(t)
	installedGame(t, f.root, "Portal")
	installedGame(t, f.root, "Doom")

	first := f.scan(t)
	second := f.scan(t)

	if first.Added != 2 {
		t.Fatalf("first = %+v, want two added", first)
	}
	if second.Added != 0 || second.Updated != 0 || second.Known != 2 {
		t.Fatalf("second = %+v, want both games already known", second)
	}
	if games := f.lib.GetInstalledGames(); len(games) != 2 {
		t.Fatalf("games = %d, want 2 after a repeat scan", len(games))
	}
}

func TestScanSkipsNonGameDirectories(t *testing.T) {
	f := newFixture(t)
	unrelatedFolder(t, f.root, "Documents")
	incompleteInstall(t, f.root, "Broken Install")
	writeFile(t, filepath.Join(f.root, "notes.txt"), 16)
	if err := os.MkdirAll(filepath.Join(f.root, "Portal.partial"), 0o755); err != nil {
		t.Fatal(err)
	}

	result := f.scan(t)

	if result.Added != 0 {
		t.Fatalf("result = %+v, want nothing added", result)
	}
	if result.Skipped != 2 {
		t.Fatalf("skipped = %d, want the two candidate folders (%+v)", result.Skipped, result.Problems)
	}
	if games := f.lib.GetInstalledGames(); len(games) != 0 {
		t.Fatalf("games = %+v, want none", games)
	}
}

func TestScanPicksExecutableByExistingPolicy(t *testing.T) {
	f := newFixture(t)
	dir := installedGame(t, f.root, "Portal")
	writeFile(t, filepath.Join(dir, "unins000.exe"), 8192)
	writeFile(t, filepath.Join(dir, "setup.exe"), 8192)

	f.scan(t)

	game := f.game(t, "Portal")
	if !strings.EqualFold(game.Executable, filepath.Join(dir, "Portal.exe")) {
		t.Fatalf("executable = %q, want the game binary", game.Executable)
	}
}

func TestScanRegistersAmbiguousInstallWithoutExecutable(t *testing.T) {
	f := newFixture(t)
	ambiguousInstall(t, f.root, "Mystery")

	result := f.scan(t)

	if result.Added != 1 {
		t.Fatalf("result = %+v, want the install registered", result)
	}
	game := f.game(t, "Mystery")
	if game.Executable != "" {
		t.Fatalf("executable = %q, want no guess for an ambiguous set", game.Executable)
	}
}

func TestScanPrefersMarkerOverDirectoryName(t *testing.T) {
	f := newFixture(t)
	dir := installedGame(t, f.root, "Portal")
	renamed := filepath.Join(f.root, "totally different name")
	if err := library.WriteMarker(dir, library.Marker{
		GameID:          "stale-id",
		Title:           "Portal",
		Executable:      "Portal.exe",
		CanonicalGameID: "canon-1",
		Version:         "1.4",
	}); err != nil {
		t.Fatalf("write marker: %v", err)
	}
	if err := os.Rename(dir, renamed); err != nil {
		t.Fatal(err)
	}

	f.scan(t)

	game := f.game(t, "Portal")
	if game.InstallDir != renamed {
		t.Fatalf("install dir = %q, want %q", game.InstallDir, renamed)
	}
	if game.CanonicalGameID != "canon-1" || game.Version != "1.4" {
		t.Fatalf("game = %+v, want the identity from the marker", game)
	}
	if !strings.EqualFold(game.Executable, filepath.Join(renamed, "Portal.exe")) {
		t.Fatalf("executable = %q, want the marker path resolved against the new folder", game.Executable)
	}
}

func TestScanMovesKnownInstallInsteadOfDuplicating(t *testing.T) {
	f := newFixture(t)
	dir := installedGame(t, f.root, "Portal")
	registered, err := f.lib.RegisterInstalled(library.InstalledGame{
		Title:      "Portal",
		Executable: filepath.Join(dir, "Portal.exe"),
		InstallDir: dir,
	})
	if err != nil {
		t.Fatalf("register installed: %v", err)
	}
	moved := filepath.Join(f.root, "Portal (backup)")
	if err := os.Rename(dir, moved); err != nil {
		t.Fatal(err)
	}

	result := f.scan(t)

	if result.Updated != 1 || result.Added != 0 {
		t.Fatalf("result = %+v, want the existing record updated", result)
	}
	games := f.lib.GetInstalledGames()
	if len(games) != 1 {
		t.Fatalf("games = %d, want 1", len(games))
	}
	if games[0].ID != registered.ID || games[0].InstallDir != moved {
		t.Fatalf("game = %+v, want %q moved to %q", games[0], registered.ID, moved)
	}
}

func TestScanFallsBackWhenMarkerIsUnreadable(t *testing.T) {
	f := newFixture(t)
	dir := installedGame(t, f.root, "Portal")
	writeFile(t, filepath.Join(dir, library.MarkerName), 0)
	if err := os.WriteFile(filepath.Join(dir, library.MarkerName), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}

	result := f.scan(t)

	if result.Added != 1 {
		t.Fatalf("result = %+v, want the install discovered without a usable marker", result)
	}
	game := f.game(t, "Portal")
	if game.InstallDir != dir {
		t.Fatalf("install dir = %q, want %q", game.InstallDir, dir)
	}
}

func TestScanReusesCanonicalMatch(t *testing.T) {
	f := newFixture(t)
	installedGame(t, f.root, "Portal")
	canonical, err := f.cat.AddGame(catalog.Game{Title: "Portal"})
	if err != nil {
		t.Fatalf("add catalog game: %v", err)
	}

	f.scan(t)

	if got := f.game(t, "Portal").CanonicalGameID; got != canonical.ID {
		t.Fatalf("canonical id = %q, want the existing game %q", got, canonical.ID)
	}
}

func TestScanDoesNotBindAmbiguousMatch(t *testing.T) {
	f := newFixture(t)
	installedGame(t, f.root, "Fallout")
	third, err := f.cat.AddGame(catalog.Game{Title: "Fallout 3"})
	if err != nil {
		t.Fatalf("add catalog game: %v", err)
	}
	fourth, err := f.cat.AddGame(catalog.Game{Title: "Fallout 4"})
	if err != nil {
		t.Fatalf("add catalog game: %v", err)
	}

	f.scan(t)

	game := f.game(t, "Fallout")
	if game.CanonicalGameID == third.ID || game.CanonicalGameID == fourth.ID {
		t.Fatalf("canonical id = %q, want no binding to a similar title", game.CanonicalGameID)
	}
	if game.CanonicalGameID == "" {
		t.Fatal("для новой игры должна создаваться собственная запись каталога")
	}
	created, err := f.cat.GetGame(game.CanonicalGameID)
	if err != nil {
		t.Fatalf("get canonical game: %v", err)
	}
	if created.Title != "Fallout" {
		t.Fatalf("canonical title = %q, want Fallout", created.Title)
	}
}

func TestScanSkipsLinkedDirectories(t *testing.T) {
	f := newFixture(t)
	outside := installedGame(t, filepath.Join(t.TempDir(), "elsewhere"), "Doom")
	makeLink(t, filepath.Join(f.root, "Doom"), outside)

	result := f.scan(t)

	if result.Added != 0 {
		t.Fatalf("result = %+v, want the link skipped", result)
	}
	if result.Skipped != 1 {
		t.Fatalf("skipped = %d, want the link counted (%+v)", result.Skipped, result.Problems)
	}
	if games := f.lib.GetInstalledGames(); len(games) != 0 {
		t.Fatalf("games = %+v, want none: сканер не должен уходить по ссылке", games)
	}
}

func TestScanSurvivesUnreadableCandidate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("права на каталог на Windows не снимаются через chmod")
	}
	f := newFixture(t)
	installedGame(t, f.root, "Portal")
	locked := filepath.Join(f.root, "Locked")
	if err := os.MkdirAll(locked, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(locked, "Locked.exe"), 2048)
	if err := os.Chmod(locked, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		//nolint:gosec // G302: каталогу возвращается исходный режим (инвариант 8), иначе t.TempDir() не сможет его удалить
		if err := os.Chmod(locked, 0o755); err != nil {
			t.Errorf("restore permissions: %v", err)
		}
	})

	result := f.scan(t)

	if result.Errors != 1 {
		t.Fatalf("errors = %d, want the locked folder reported (%+v)", result.Errors, result.Problems)
	}
	if result.Added != 1 {
		t.Fatalf("added = %d, want the readable game still discovered", result.Added)
	}
}
