package protocolreg

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

type fakeStore struct {
	data   map[string]string
	writes int
	getErr error
	setErr error
}

func id(key, name string) string { return key + "|" + name }

func (f *fakeStore) get(key, name string) (string, bool, error) {
	if f.getErr != nil {
		return "", false, f.getErr
	}
	data, ok := f.data[id(key, name)]
	return data, ok, nil
}

func (f *fakeStore) set(key, name, data string) error {
	if f.setErr != nil {
		return f.setErr
	}
	f.writes++
	if f.data == nil {
		f.data = map[string]string{}
	}
	f.data[id(key, name)] = data
	return nil
}

func touch(t *testing.T, path string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRunsFrom(t *testing.T) {
	root := t.TempDir()
	install := filepath.Join(root, "Typhon")
	exe := touch(t, filepath.Join(install, "typhon.exe"))
	elsewhere := touch(t, filepath.Join(root, "bin", "typhon.exe"))
	nested := touch(t, filepath.Join(install, "bin", "typhon.exe"))
	cases := []struct {
		name, exe, dir string
		want           bool
	}{
		{"installed copy", exe, install, true},
		{"trailing separator", exe, install + string(filepath.Separator), true},
		{"unclean record", exe, filepath.Join(root, "bin") + string(filepath.Separator) + ".." + string(filepath.Separator) + "Typhon", true},
		{"build output", elsewhere, install, false},
		{"subdirectory of the install", nested, install, false},
		{"empty record", exe, "", false},
		{"relative record", exe, "Typhon", false},
		{"removed install", exe, filepath.Join(root, "gone"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := RunsFrom(tc.exe, tc.dir)
			if err != nil || got != tc.want {
				t.Fatalf("RunsFrom = %v, %v; want %v", got, err, tc.want)
			}
		})
	}
}

func TestRunsFromReportsAMissingExecutable(t *testing.T) {
	dir := t.TempDir()
	if _, err := RunsFrom(filepath.Join(dir, "typhon.exe"), dir); err == nil {
		t.Fatal("want an error")
	}
}

func TestSamePath(t *testing.T) {
	a := filepath.Join("x", "Typhon")
	b := filepath.Join("x", "TYPHON")
	if !samePath(a, b, true) {
		t.Fatal("folded paths must match")
	}
	if samePath(a, b, false) {
		t.Fatal("case-sensitive paths must differ")
	}
}

func TestApplyWritesTheProtocol(t *testing.T) {
	exe := filepath.Join(t.TempDir(), "with space", "typhon.exe")
	s := &fakeStore{}
	if err := apply(s, exe); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		id("", ""):                   "URL:Typhon Protocol",
		id("", "URL Protocol"):       "",
		id("DefaultIcon", ""):        `"` + exe + `",0`,
		id(`shell\open\command`, ""): `"` + exe + `" "%1"`,
	}
	if len(s.data) != len(want) {
		t.Fatalf("values = %v", s.data)
	}
	for key, data := range want {
		got, ok := s.data[key]
		if !ok || got != data {
			t.Errorf("%s = %q, want %q", key, got, data)
		}
	}
}

func TestApplyIsIdempotent(t *testing.T) {
	exe := filepath.Join(t.TempDir(), "typhon.exe")
	s := &fakeStore{}
	if err := apply(s, exe); err != nil {
		t.Fatal(err)
	}
	first := s.writes
	if err := apply(s, exe); err != nil {
		t.Fatal(err)
	}
	if s.writes != first {
		t.Fatalf("second apply wrote %d values", s.writes-first)
	}
}

func TestApplyFollowsAMovedLauncher(t *testing.T) {
	dir := t.TempDir()
	s := &fakeStore{}
	if err := apply(s, filepath.Join(dir, "old", "typhon.exe")); err != nil {
		t.Fatal(err)
	}
	first := s.writes
	moved := filepath.Join(dir, "new", "typhon.exe")
	if err := apply(s, moved); err != nil {
		t.Fatal(err)
	}
	if s.writes-first != 2 {
		t.Fatalf("rewrote %d values, want the command and the icon", s.writes-first)
	}
	if got := s.data[id(`shell\open\command`, "")]; got != `"`+moved+`" "%1"` {
		t.Fatalf("command = %q", got)
	}
}

func TestApplyNeverWritesABadPath(t *testing.T) {
	abs := filepath.Join(t.TempDir(), "typhon.exe")
	cases := []struct {
		name, exe string
		want      error
	}{
		{"empty", "", ErrEmptyPath},
		{"relative", "typhon.exe", ErrRelative},
		{"quote", abs + `" --play "x`, ErrQuotedPath},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := &fakeStore{data: map[string]string{id(`shell\open\command`, ""): "kept"}}
			if err := apply(s, tc.exe); !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
			if s.writes != 0 || s.data[id(`shell\open\command`, "")] != "kept" {
				t.Fatalf("store changed: %v", s.data)
			}
		})
	}
}

func TestApplyReportsStoreFailures(t *testing.T) {
	exe := filepath.Join(t.TempDir(), "typhon.exe")
	denied := errors.New("access denied")
	for name, s := range map[string]*fakeStore{
		"read":  {getErr: denied},
		"write": {setErr: denied},
	} {
		t.Run(name, func(t *testing.T) {
			if err := apply(s, exe); !errors.Is(err, denied) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}
