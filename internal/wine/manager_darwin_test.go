package wine

import (
	"os"
	"path/filepath"
	"testing"
)

// scriptRuntime подменяет CLI CrossOver shell-скриптами: так тесты проходят
// на CI, где CrossOver нет, и заодно проверяют ровно те аргументы, которые
// мы собираемся отдать настоящему бинарю.
func scriptRuntime(t *testing.T, bottles string) (Runtime, string) {
	t.Helper()
	dir := t.TempDir()
	log := filepath.Join(dir, "calls.log")

	// cxbottle создаёт каталог бутыля так же, как настоящий: dosdevices с
	// диском c: и drive_c с профилем пользователя.
	cxbottle := filepath.Join(dir, "cxbottle")
	body := `#!/bin/sh
echo "cxbottle $@" >> ` + log + `
name=""
while [ $# -gt 0 ]; do
  case "$1" in
    --bottle) name="$2"; shift 2;;
    *) shift;;
  esac
done
b="` + bottles + `/$name"
mkdir -p "$b/dosdevices" "$b/drive_c/users/crossover/AppData/Roaming"
ln -sfn ../drive_c "$b/dosdevices/c:"
exit 0
`
	if err := os.WriteFile(cxbottle, []byte(body), 0o755); err != nil {
		t.Fatalf("WriteFile cxbottle: %v", err)
	}
	for _, name := range []string{"cxstart", "wineserver"} {
		path := filepath.Join(dir, name)
		script := "#!/bin/sh\necho \"" + name + " $@\" >> " + log + "\nexit 0\n"
		if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
			t.Fatalf("WriteFile %s: %v", name, err)
		}
	}
	rt := Runtime{
		Root:       dir,
		CxBottle:   cxbottle,
		CxStart:    filepath.Join(dir, "cxstart"),
		WineServer: filepath.Join(dir, "wineserver"),
		Version:    "26.3",
	}
	return rt, log
}

func newTestManager(t *testing.T) (*Manager, string, string) {
	t.Helper()
	bottles := t.TempDir()
	rt, log := scriptRuntime(t, bottles)
	m := NewManager(rt)
	m.BottlesDir = bottles
	return m, bottles, log
}

func TestEnsureCreatesBottleOnce(t *testing.T) {
	m, bottles, log := newTestManager(t)
	games := t.TempDir()
	dest := filepath.Join(games, "Demo")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	first, err := m.Ensure(dest, games)
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if first.Key != dest || first.Games != games || first.Drive == "" {
		t.Fatalf("bottle = %+v", first)
	}
	if _, err := os.Stat(filepath.Join(bottles, first.Name, markerName)); err != nil {
		t.Fatalf("marker: %v", err)
	}
	target, err := os.Readlink(filepath.Join(first.Path, "dosdevices", first.Drive+":"))
	if err != nil {
		t.Fatalf("Readlink: %v", err)
	}
	if target != games {
		t.Fatalf("drive target = %q, want %q", target, games)
	}

	second, err := m.Ensure(dest, games)
	if err != nil {
		t.Fatalf("Ensure again: %v", err)
	}
	if second.Name != first.Name {
		t.Fatalf("second bottle = %q, want %q", second.Name, first.Name)
	}
	data, err := os.ReadFile(log)
	if err != nil {
		t.Fatalf("ReadFile log: %v", err)
	}
	if got := countLines(string(data), "cxbottle"); got != 1 {
		t.Fatalf("cxbottle calls = %d, want 1", got)
	}
}

func TestEnsurePassesTemplate(t *testing.T) {
	m, _, log := newTestManager(t)
	games := t.TempDir()
	dest := filepath.Join(games, "Demo")

	if _, err := m.Ensure(dest, games); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	data, err := os.ReadFile(log)
	if err != nil {
		t.Fatalf("ReadFile log: %v", err)
	}
	line := string(data)
	for _, want := range []string{"--create", "--template win10_64", "--bottle Typhon-Demo-"} {
		if !contains(line, want) {
			t.Fatalf("cxbottle call %q does not contain %q", line, want)
		}
	}
}

func TestEnsureNeedsGamesPath(t *testing.T) {
	m, _, _ := newTestManager(t)
	if _, err := m.Ensure(filepath.Join(t.TempDir(), "Demo"), "  "); err == nil {
		t.Fatal("Ensure without a games path: want error")
	}
}

func TestListAndLookup(t *testing.T) {
	m, bottles, _ := newTestManager(t)
	games := t.TempDir()
	dest := filepath.Join(games, "Demo")

	created, err := m.Ensure(dest, games)
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	// Чужой бутыль без метки в перечисление попадать не должен.
	if err := os.MkdirAll(filepath.Join(bottles, "Steam", "drive_c"), 0o755); err != nil {
		t.Fatalf("MkdirAll Steam: %v", err)
	}

	list, err := m.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 || list[0].Name != created.Name {
		t.Fatalf("List = %+v, want only %q", list, created.Name)
	}

	found, ok := m.Lookup(filepath.Join(dest, "bin", "game.exe"))
	if !ok {
		t.Fatal("Lookup by a path inside the install dir: not found")
	}
	if found.Name != created.Name {
		t.Fatalf("Lookup = %q, want %q", found.Name, created.Name)
	}
	if _, ok := m.Lookup(filepath.Join(games, "Other", "game.exe")); ok {
		t.Fatal("Lookup for another game: want not found")
	}
}

func TestListWithoutBottlesDir(t *testing.T) {
	m, bottles, _ := newTestManager(t)
	if err := os.RemoveAll(bottles); err != nil {
		t.Fatalf("RemoveAll: %v", err)
	}
	got, err := m.List()
	if err != nil {
		t.Fatalf("List without the bottles dir: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %+v, want none", got)
	}
}

func TestRemoveDeletesBottle(t *testing.T) {
	m, _, _ := newTestManager(t)
	games := t.TempDir()
	dest := filepath.Join(games, "Demo")

	created, err := m.Ensure(dest, games)
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if err := m.Remove(dest); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := os.Stat(created.Path); !os.IsNotExist(err) {
		t.Fatalf("bottle still exists: %v", err)
	}
	if _, ok := m.Lookup(dest); ok {
		t.Fatal("Lookup after Remove: want not found")
	}
}

func TestRemoveUnknownIsNoError(t *testing.T) {
	m, _, _ := newTestManager(t)
	if err := m.Remove(filepath.Join(t.TempDir(), "Nope")); err != nil {
		t.Fatalf("Remove unknown: %v", err)
	}
}
