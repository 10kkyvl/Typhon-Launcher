package settings

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func resetPortable() {
	portableOnce = sync.Once{}
	portableCached = portableResult{}
}

func fakeLauncher(t *testing.T, marker *string) string {
	t.Helper()
	// PortableDir resolves the executable path, which expands 8.3 short names such as
	// RUNNER~1 in the CI temp dir; the expected directory must be resolved the same way.
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(dir, "typhon.exe")
	if err := os.WriteFile(exe, []byte("exe"), 0o600); err != nil {
		t.Fatal(err)
	}
	if marker != nil {
		if err := os.WriteFile(filepath.Join(dir, PortableMarkerName), []byte(*marker), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	prev := executablePath
	executablePath = func() (string, error) { return exe, nil }
	resetPortable()
	t.Cleanup(func() {
		executablePath = prev
		resetPortable()
	})
	return dir
}

func TestPortableDirWithoutMarkerIsOrdinaryMode(t *testing.T) {
	fakeLauncher(t, nil)
	dir, ok, err := PortableDir()
	if err != nil || ok || dir != "" {
		t.Fatalf("PortableDir() = %q, %v, %v; want empty, false, nil", dir, ok, err)
	}
	if IsPortable() {
		t.Fatal("IsPortable() = true without a marker")
	}
	if err := PortableError(); err != nil {
		t.Fatalf("PortableError() = %v, want nil", err)
	}
}

func TestPortableDirResolvesMarkerContent(t *testing.T) {
	cases := []struct {
		name    string
		content string
		rel     string
	}{
		{"empty marker", "", "data"},
		{"blank marker", "  \r\n\t\n", "data"},
		{"subdirectory", "profile", "profile"},
		{"nested with slash", "profile/state", filepath.Join("profile", "state")},
		{"nested with backslash", `profile\state`, filepath.Join("profile", "state")},
		{"crlf and spaces", "  profile  \r\n", "profile"},
		{"byte order mark", "\ufeffprofile\r\n", "profile"},
		{"padded to the limit", "profile" + strings.Repeat(" ", portableMarkerMax-len("profile")), "profile"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			exeDir := fakeLauncher(t, &tc.content)
			dir, ok, err := PortableDir()
			if err != nil || !ok {
				t.Fatalf("PortableDir() = %q, %v, %v; want ok", dir, ok, err)
			}
			if want := filepath.Join(exeDir, tc.rel); dir != want {
				t.Fatalf("PortableDir() = %q, want %q", dir, want)
			}
			if !IsPortable() {
				t.Fatal("IsPortable() = false with a valid marker")
			}
		})
	}
}

func TestPortableDirRejectsUnsafeMarkers(t *testing.T) {
	abs := t.TempDir()
	cases := map[string]string{
		"absolute path":        abs,
		"parent":               "..",
		"parent in the middle": "a/../b",
		"leading parent":       `..\outside`,
		"drive":                "C:",
		"drive with path":      `C:\data`,
		"rooted slash":         "/data",
		"rooted backslash":     `\data`,
		"empty segment":        "a//b",
		"trailing separator":   "data/",
		"dot":                  ".",
		"dots":                 "...",
		"stream":               "data:stream",
		"two lines":            "a\nb",
		"control character":    "da\x00ta",
		"larger than limit":    strings.Repeat("a", portableMarkerMax+1),
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			content := content
			fakeLauncher(t, &content)
			dir, ok, err := PortableDir()
			if !errors.Is(err, ErrPortableMarkerInvalid) {
				t.Fatalf("PortableDir() error = %v, want ErrPortableMarkerInvalid", err)
			}
			if ok || dir != "" {
				t.Fatalf("PortableDir() = %q, %v with a rejected marker", dir, ok)
			}
			if IsPortable() {
				t.Fatal("IsPortable() = true with a rejected marker")
			}
			if !errors.Is(PortableError(), ErrPortableMarkerInvalid) {
				t.Fatalf("PortableError() = %v, want ErrPortableMarkerInvalid", PortableError())
			}
		})
	}
}

func TestPortableDirUnreadableMarkerIsNotOrdinaryMode(t *testing.T) {
	exeDir := fakeLauncher(t, nil)
	if err := os.Mkdir(filepath.Join(exeDir, PortableMarkerName), 0o700); err != nil {
		t.Fatal(err)
	}
	resetPortable()
	dir, ok, err := PortableDir()
	if err == nil {
		t.Fatalf("PortableDir() = %q, %v, nil; want a read error", dir, ok)
	}
	if errors.Is(err, os.ErrNotExist) || errors.Is(err, ErrPortableMarkerInvalid) {
		t.Fatalf("PortableDir() error = %v, want a plain read error", err)
	}
	if ok || dir != "" || IsPortable() {
		t.Fatalf("PortableDir() = %q, %v, IsPortable = %v on an unreadable marker", dir, ok, IsPortable())
	}
	if PortableError() == nil {
		t.Fatal("PortableError() = nil on an unreadable marker")
	}
}

func TestPortableDirExecutableFailure(t *testing.T) {
	fakeLauncher(t, nil)
	boom := errors.New("no executable")
	executablePath = func() (string, error) { return "", boom }
	resetPortable()
	if _, ok, err := PortableDir(); !errors.Is(err, boom) || ok {
		t.Fatalf("PortableDir() = _, %v, %v; want the executable error", ok, err)
	}

	executablePath = func() (string, error) { return filepath.Join(t.TempDir(), "missing.exe"), nil }
	resetPortable()
	if _, ok, err := PortableDir(); err == nil || ok {
		t.Fatalf("PortableDir() = _, %v, %v for an executable that does not exist; want an error", ok, err)
	}
}

func TestPortableDirIsDecidedOnce(t *testing.T) {
	marker := "first"
	exeDir := fakeLauncher(t, &marker)
	first, _, err := PortableDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(exeDir, PortableMarkerName)); err != nil {
		t.Fatal(err)
	}
	second, ok, err := PortableDir()
	if err != nil || !ok || second != first {
		t.Fatalf("second PortableDir() = %q, %v, %v; want the cached %q", second, ok, err, first)
	}
}

func TestPortableDirCachesTheMarkerError(t *testing.T) {
	marker := ".."
	exeDir := fakeLauncher(t, &marker)
	if _, _, err := PortableDir(); !errors.Is(err, ErrPortableMarkerInvalid) {
		t.Fatalf("first PortableDir() error = %v", err)
	}
	if err := os.Remove(filepath.Join(exeDir, PortableMarkerName)); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := PortableDir(); !errors.Is(err, ErrPortableMarkerInvalid) || ok {
		t.Fatalf("second PortableDir() = _, %v, %v; the error must stay cached", ok, err)
	}
}

func TestPortableDirTakesTheWorkerDirectoryFromTheEnvironment(t *testing.T) {
	fakeLauncher(t, nil)
	want := t.TempDir()
	t.Setenv(PortableEnv, want)
	resetPortable()

	dir, ok, err := PortableDir()
	if err != nil || !ok || dir != want {
		t.Fatalf("PortableDir() = %q, %v, %v; want %q", dir, ok, err, want)
	}
}

func TestPortableDirRejectsAnUnsafeEnvironmentDirectory(t *testing.T) {
	for name, value := range map[string]string{
		"relative": "data",
		"parent":   filepath.Join("..", "data"),
		"unclean":  t.TempDir() + string(filepath.Separator) + ".." + string(filepath.Separator) + "x",
	} {
		t.Run(name, func(t *testing.T) {
			fakeLauncher(t, nil)
			t.Setenv(PortableEnv, value)
			resetPortable()
			dir, ok, err := PortableDir()
			if !errors.Is(err, ErrPortableMarkerInvalid) || ok || dir != "" {
				t.Fatalf("PortableDir() = %q, %v, %v; want ErrPortableMarkerInvalid", dir, ok, err)
			}
		})
	}
}

func TestPortableDirMarkerWinsOverTheEnvironment(t *testing.T) {
	marker := "next-to-exe"
	exeDir := fakeLauncher(t, &marker)
	t.Setenv(PortableEnv, t.TempDir())
	resetPortable()

	dir, ok, err := PortableDir()
	if err != nil || !ok || dir != filepath.Join(exeDir, "next-to-exe") {
		t.Fatalf("PortableDir() = %q, %v, %v; want the marker directory", dir, ok, err)
	}
}
