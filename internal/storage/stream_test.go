package storage

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteAtomicStreamReplacesWholeAndSetsMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "typhon.exe")
	if err := os.WriteFile(path, []byte("old launcher, longer than the new one"), 0o600); err != nil {
		t.Fatal(err)
	}

	err := WriteAtomicStream(path, 0o700, func(w io.Writer) error {
		for _, chunk := range []string{"new ", "launcher"} {
			if _, err := io.WriteString(w, chunk); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WriteAtomicStream: %v", err)
	}
	got, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, []byte("new launcher")) {
		t.Fatalf("content = %q", got)
	}
	if names := dirNames(t, dir); len(names) != 1 {
		t.Fatalf("directory holds %v, want only the target", names)
	}
}

func TestWriteAtomicStreamKeepsTheTargetWhenTheSourceFails(t *testing.T) {
	errBoom := errors.New("source died half-way")
	cases := []struct {
		name string
		seed string
		have bool
	}{
		{"existing target is kept", "old launcher", true},
		{"missing target stays missing", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "typhon.exe")
			if tc.have {
				if err := os.WriteFile(path, []byte(tc.seed), 0o600); err != nil {
					t.Fatal(err)
				}
			}

			err := WriteAtomicStream(path, 0o700, func(w io.Writer) error {
				if _, err := w.Write([]byte("half of the new lau")); err != nil {
					return err
				}
				return errBoom
			})
			if !errors.Is(err, errBoom) {
				t.Fatalf("error = %v, want the source error", err)
			}
			for _, name := range dirNames(t, dir) {
				if strings.HasPrefix(name, ".tmp-") {
					t.Fatalf("temp file left behind: %s", name)
				}
			}
			got, readErr := os.ReadFile(filepath.Clean(path))
			if tc.have {
				if readErr != nil || string(got) != tc.seed {
					t.Fatalf("target = %q, %v; want the old content untouched", got, readErr)
				}
				return
			}
			if !errors.Is(readErr, os.ErrNotExist) {
				t.Fatalf("target exists after a failed write: %v", readErr)
			}
		})
	}
}

func TestEnvelopeVersion(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		version int
		ok      bool
		wantErr bool
	}{
		{"envelope", `{"version":3,"data":{"a":1}}`, 3, true, false},
		{"envelope with data first", `{"data":[],"version":2}`, 2, true, false},
		{"indented envelope", "{\n  \"version\": 7,\n  \"data\": {}\n}", 7, true, false},
		{"legacy object", `{"theme":"dark"}`, 0, false, false},
		{"legacy array", `[{"id":"a"}]`, 0, false, false},
		{"version without data is a legacy object", `{"version":1,"torrents":{}}`, 0, false, false},
		{"empty document", ``, 0, false, true},
		{"not json", `hello`, 0, false, true},
		{"truncated", `{"version":1,"data":{`, 0, false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			version, ok, err := EnvelopeVersion([]byte(tc.raw))
			if (err != nil) != tc.wantErr {
				t.Fatalf("error = %v, want error: %v", err, tc.wantErr)
			}
			if version != tc.version || ok != tc.ok {
				t.Fatalf("EnvelopeVersion = (%d, %v), want (%d, %v)", version, ok, tc.version, tc.ok)
			}
		})
	}
}

func TestWriteAtomicStreamTempUsesTheCallersPattern(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "typhon.exe")
	seen := ""
	err := WriteAtomicStreamTemp(path, 0o700, ".typhon-swap-*", func(w io.Writer) error {
		names := dirNames(t, dir)
		if len(names) != 1 {
			t.Errorf("directory holds %v while writing, want only the temp file", names)
		} else {
			seen = names[0]
		}
		_, err := io.WriteString(w, "payload")
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(seen, ".typhon-swap-") {
		t.Fatalf("temp file was named %q", seen)
	}
	if names := dirNames(t, dir); len(names) != 1 || names[0] != "typhon.exe" {
		t.Fatalf("directory holds %v", names)
	}
}

func TestRemoveAndRename(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	moved := filepath.Join(dir, "b.txt")
	if err := Rename(file, moved); err != nil {
		t.Fatalf("Rename: %v", err)
	}
	if err := Remove(moved); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if err := Remove(moved); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Remove of a missing file = %v, want ErrNotExist", err)
	}
	if err := Rename(moved, file); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Rename of a missing file = %v, want ErrNotExist", err)
	}
}
