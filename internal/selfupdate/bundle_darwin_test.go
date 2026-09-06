//go:build darwin && !devmock

package selfupdate

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"typhon/internal/settings"
)

type zipEntry struct {
	name    string
	body    string
	mode    os.FileMode
	symlink string
}

func writeZip(t *testing.T, entries []zipEntry) string {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for _, e := range entries {
		header := &zip.FileHeader{Name: e.name, Method: zip.Deflate}
		mode := e.mode
		if e.symlink != "" {
			mode |= os.ModeSymlink
		}
		if mode == 0 {
			mode = 0o644
		}
		header.SetMode(mode)
		f, err := w.CreateHeader(header)
		if err != nil {
			t.Fatalf("CreateHeader %s: %v", e.name, err)
		}
		payload := e.body
		if e.symlink != "" {
			payload = e.symlink
		}
		if _, err := f.Write([]byte(payload)); err != nil {
			t.Fatalf("Write %s: %v", e.name, err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("zip Close: %v", err)
	}
	path := filepath.Join(t.TempDir(), "update.zip")
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

func appZip(t *testing.T, body string) string {
	t.Helper()
	return writeZip(t, []zipEntry{
		{name: "Typhon.app/Contents/Info.plist", body: "<plist/>"},
		{name: "Typhon.app/Contents/MacOS/typhon", body: body, mode: 0o755},
		{name: "Typhon.app/Contents/Resources/icons.icns", body: "icns"},
	})
}

func TestExtractBundleKeepsExecutableBit(t *testing.T) {
	dest := t.TempDir()

	bundle, err := extractBundle(appZip(t, "new binary"), dest)
	if err != nil {
		t.Fatalf("extractBundle: %v", err)
	}
	if filepath.Base(bundle) != "Typhon.app" {
		t.Fatalf("bundle = %q, want Typhon.app", bundle)
	}
	exe := filepath.Join(bundle, "Contents", "MacOS", "typhon")
	info, err := os.Stat(exe)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if info.Mode()&0o111 == 0 {
		// Без бита исполнения обновлённый лаунчер не запустится, а понять
		// это можно будет только после перезапуска.
		t.Fatalf("режим = %v, исполняемый бит потерян", info.Mode())
	}
	body, err := os.ReadFile(exe)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(body) != "new binary" {
		t.Fatalf("содержимое = %q", string(body))
	}
}

func TestExtractBundleKeepsSymlinks(t *testing.T) {
	path := writeZip(t, []zipEntry{
		{name: "Typhon.app/Contents/MacOS/typhon", body: "bin", mode: 0o755},
		{name: "Typhon.app/Contents/Frameworks/Current", symlink: "A"},
	})

	bundle, err := extractBundle(path, t.TempDir())
	if err != nil {
		t.Fatalf("extractBundle: %v", err)
	}
	target, err := os.Readlink(filepath.Join(bundle, "Contents", "Frameworks", "Current"))
	if err != nil {
		t.Fatalf("Readlink: %v", err)
	}
	if target != "A" {
		t.Fatalf("симлинк указывает на %q, want %q", target, "A")
	}
}

// Архив приходит из сети. Путь с .. или абсолютный обязан отвергаться, иначе
// обновление перезапишет что угодно на диске.
func TestExtractBundleRejectsEscapingPaths(t *testing.T) {
	for _, name := range []string{"../evil.txt", "Typhon.app/../../evil.txt", "/etc/evil.txt"} {
		path := writeZip(t, []zipEntry{{name: name, body: "x"}})
		if _, err := extractBundle(path, t.TempDir()); !errors.Is(err, errBundleEscapingPath) {
			t.Fatalf("extractBundle(%q) = %v, want errBundleEscapingPath", name, err)
		}
	}
}

// Симлинк, указывающий наружу, — тот же побег, только в две ступени.
func TestExtractBundleRejectsEscapingSymlink(t *testing.T) {
	path := writeZip(t, []zipEntry{
		{name: "Typhon.app/Contents/MacOS/typhon", body: "bin", mode: 0o755},
		{name: "Typhon.app/Contents/evil", symlink: "../../../../etc/passwd"},
	})
	if _, err := extractBundle(path, t.TempDir()); !errors.Is(err, errBundleEscapingPath) {
		t.Fatalf("err = %v, want errBundleEscapingPath", err)
	}
}

func TestExtractBundleWithoutApp(t *testing.T) {
	path := writeZip(t, []zipEntry{{name: "readme.txt", body: "no app here"}})
	if _, err := extractBundle(path, t.TempDir()); !errors.Is(err, errBundleNoApp) {
		t.Fatalf("err = %v, want errBundleNoApp", err)
	}
}

func TestExtractBundleRejectsGarbage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "not.zip")
	if err := os.WriteFile(path, []byte("definitely not a zip"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := extractBundle(path, t.TempDir()); err == nil {
		t.Fatal("extractBundle on garbage: want error")
	}
}

func TestSwapBundleReplacesInPlace(t *testing.T) {
	root := t.TempDir()
	current := filepath.Join(root, "Typhon.app")
	if err := os.MkdirAll(filepath.Join(current, "Contents", "MacOS"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	exe := filepath.Join(current, "Contents", "MacOS", "typhon")
	if err := os.WriteFile(exe, []byte("old binary"), 0o755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	staged, err := extractBundle(appZip(t, "new binary"), t.TempDir())
	if err != nil {
		t.Fatalf("extractBundle: %v", err)
	}

	if err := swapBundle(staged, current); err != nil {
		t.Fatalf("swapBundle: %v", err)
	}
	body, err := os.ReadFile(exe)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(body) != "new binary" {
		t.Fatalf("после подмены = %q", string(body))
	}
	// Ничего лишнего рядом остаться не должно: иначе каждое обновление
	// оставляет копию бандла.
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "Typhon.app" {
		t.Fatalf("рядом остался мусор: %v", entries)
	}
}

// Если новый бандл не встал, старый обязан остаться на месте: лаунчер,
// которого нет вовсе, хуже необновлённого.
func TestSwapBundleRestoresOnFailure(t *testing.T) {
	root := t.TempDir()
	current := filepath.Join(root, "Typhon.app")
	if err := os.MkdirAll(current, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(current, "marker"), []byte("old"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if err := swapBundle(filepath.Join(t.TempDir(), "missing.app"), current); err == nil {
		t.Fatal("swapBundle с отсутствующим источником: want error")
	}
	if _, err := os.Stat(filepath.Join(current, "marker")); err != nil {
		t.Fatalf("старый бандл потерян: %v", err)
	}
}

func TestBundleOfClimbsToTheApp(t *testing.T) {
	got, err := bundleOf("/Applications/Typhon.app/Contents/MacOS/typhon")
	if err != nil {
		t.Fatalf("bundleOf: %v", err)
	}
	if got != "/Applications/Typhon.app" {
		t.Fatalf("got %q", got)
	}
}

// Запуск не из бандла — это dev-сборка или распакованная копия: подменять там
// нечего, и молча трогать соседние каталоги нельзя.
func TestBundleOfRejectsLooseBinary(t *testing.T) {
	for _, path := range []string{"", "/usr/local/bin/typhon", "/tmp/bin/typhon"} {
		if _, err := bundleOf(path); err == nil {
			t.Fatalf("bundleOf(%q): want error", path)
		}
	}
}

// Полная проверка Apply: подменяем домашний каталог, кладём архив в кеш и
// состояние в стор — так же, как это делает загрузчик, — и убеждаемся, что
// бандл действительно подменён.
func TestApplyReplacesTheBundle(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	configDir, err := settings.ConfigDir()
	if err != nil {
		t.Fatalf("ConfigDir: %v", err)
	}
	cacheDir, err := CacheDir(configDir)
	if err != nil {
		t.Fatalf("CacheDir: %v", err)
	}
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	archive := filepath.Join(cacheDir, "typhon-darwin-arm64.zip")
	body, err := os.ReadFile(appZip(t, "new binary"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	//nolint:gosec // G703: archive лежит в кеше внутри t.TempDir(), внешнего ввода в пути нет
	if err := os.WriteFile(archive, body, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	sum := sha256.Sum256(body)

	store, err := NewStore(configDir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if err := store.Save(stored{
		AvailableVersion: "9.9.9",
		ReadyPath:        archive,
		Artifact: &Artifact{
			OS: "darwin", Arch: "arm64", Kind: KindBundle,
			Name: "typhon-darwin-arm64.zip", URL: "https://example.invalid/typhon.zip",
			Size: int64(len(body)), SHA256: hex.EncodeToString(sum[:]),
		},
	}); err != nil {
		t.Fatalf("store.Save: %v", err)
	}

	apps := filepath.Join(home, "Applications")
	exe := filepath.Join(apps, "Typhon.app", "Contents", "MacOS", "typhon")
	if err := os.MkdirAll(filepath.Dir(exe), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(exe, []byte("old binary"), 0o755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if err := Apply(t.Context(), archive, filepath.Dir(exe), exe); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	got, err := os.ReadFile(exe)
	if err != nil {
		t.Fatalf("ReadFile after apply: %v", err)
	}
	if string(got) != "new binary" {
		t.Fatalf("после обновления = %q, want %q", string(got), "new binary")
	}
	// Временный каталог распаковки не должен пережить обновление.
	entries, err := os.ReadDir(apps)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("рядом с бандлом остался мусор: %v", entries)
	}
}

// Архив с подделанным содержимым обязан быть отвергнут до подмены: подпись
// манифеста только тогда чего-то стоит, когда хеш проверяется на месте.
func TestApplyRejectsTamperedArchive(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	configDir, err := settings.ConfigDir()
	if err != nil {
		t.Fatalf("ConfigDir: %v", err)
	}
	cacheDir, err := CacheDir(configDir)
	if err != nil {
		t.Fatalf("CacheDir: %v", err)
	}
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	archive := filepath.Join(cacheDir, "typhon-darwin-arm64.zip")
	body, err := os.ReadFile(appZip(t, "tampered binary"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	//nolint:gosec // G703: archive лежит в кеше внутри t.TempDir(), внешнего ввода в пути нет
	if err := os.WriteFile(archive, body, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	store, err := NewStore(configDir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	wrong := sha256.Sum256([]byte("what the manifest promised"))
	if err := store.Save(stored{
		AvailableVersion: "9.9.9",
		ReadyPath:        archive,
		Artifact: &Artifact{
			OS: "darwin", Arch: "arm64", Kind: KindBundle,
			Name: "typhon-darwin-arm64.zip", URL: "https://example.invalid/typhon.zip",
			Size: int64(len(body)), SHA256: hex.EncodeToString(wrong[:]),
		},
	}); err != nil {
		t.Fatalf("store.Save: %v", err)
	}

	exe := filepath.Join(home, "Applications", "Typhon.app", "Contents", "MacOS", "typhon")
	if err := os.MkdirAll(filepath.Dir(exe), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(exe, []byte("old binary"), 0o755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if err := Apply(t.Context(), archive, filepath.Dir(exe), exe); err == nil {
		t.Fatal("Apply на подделанном архиве: want error")
	}
	got, err := os.ReadFile(exe)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != "old binary" {
		t.Fatal("подделанный архив подменил бандл")
	}
}
