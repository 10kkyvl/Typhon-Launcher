package storage

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

type item struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

func TestSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "items.json")
	want := []item{{ID: "1", Title: "Первый"}, {ID: "2", Title: "Второй"}}
	if err := Save(path, 1, want); err != nil {
		t.Fatalf("save: %v", err)
	}

	var got []item
	if err := Load(path, 1, nil, &got); err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(got) != 2 || got[1].Title != "Второй" {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestLoadMissingFileReportsNotExist(t *testing.T) {
	var got []item
	err := Load(filepath.Join(t.TempDir(), "absent.json"), 1, nil, &got)
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("load: %v, want fs.ErrNotExist", err)
	}
	if got != nil {
		t.Fatalf("got %+v, want nil", got)
	}
}

func TestLoadRejectsCorruptFile(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{"truncated envelope", `{"version":1,"data":[`},
		{"truncated array", `[{"id":"1"`},
		{"scalar root", `42`},
		{"garbage", `not json at all`},
		{"empty", ``},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "items.json")
			if err := os.WriteFile(path, []byte(tc.raw), 0o600); err != nil {
				t.Fatal(err)
			}
			var got []item
			err := Load(path, 1, nil, &got)
			if err == nil {
				t.Fatal("expected error")
			}
			if errors.Is(err, fs.ErrNotExist) {
				t.Fatalf("corrupt file reported as missing: %v", err)
			}
		})
	}
}

func TestWriteAtomicLeavesNoTempOnFailure(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "items.json")
	if err := WriteAtomic(path, []byte("payload")); err == nil {
		t.Fatal("expected error writing into missing directory")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("leftovers: %v", entries)
	}
}

func TestWriteAtomicDoesNotUseSuffixTemp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "items.json")
	if err := os.WriteFile(path+".tmp", []byte("sentinel"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := WriteAtomic(path, []byte("payload")); err != nil {
		t.Fatalf("write: %v", err)
	}
	sentinel, err := os.ReadFile(filepath.Clean(path + ".tmp"))
	if err != nil {
		t.Fatalf("read sentinel: %v", err)
	}
	if string(sentinel) != "sentinel" {
		t.Fatalf("path+\".tmp\" was reused as scratch file")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("unexpected leftovers: %v", entries)
	}
}

func TestWriteAtomicKeepsExistingMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "items.json")
	if err := os.WriteFile(path, []byte("a"), 0o640); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteAtomic(path, []byte("b")); err != nil {
		t.Fatalf("write: %v", err)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if before.Mode().Perm() != after.Mode().Perm() {
		t.Fatalf("mode changed: %v -> %v", before.Mode().Perm(), after.Mode().Perm())
	}
}

func TestLoadLegacyFileRunsMigrations(t *testing.T) {
	path := filepath.Join(t.TempDir(), "items.json")
	legacy := []map[string]string{{"id": "1", "name": "Старый формат"}}
	data, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	migrations := map[int]Migration{
		0: func(raw json.RawMessage) (json.RawMessage, error) {
			var old []map[string]string
			if err := json.Unmarshal(raw, &old); err != nil {
				return nil, err
			}
			next := make([]item, 0, len(old))
			for _, entry := range old {
				next = append(next, item{ID: entry["id"], Title: entry["name"]})
			}
			return json.Marshal(next)
		},
	}

	var got []item
	if err := Load(path, 1, migrations, &got); err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(got) != 1 || got[0].Title != "Старый формат" {
		t.Fatalf("got %+v", got)
	}
}

func TestLoadRejectsNewerVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "items.json")
	if err := Save(path, 5, []item{{ID: "1"}}); err != nil {
		t.Fatalf("save: %v", err)
	}
	var got []item
	if err := Load(path, 1, nil, &got); err == nil {
		t.Fatal("expected error for newer version")
	}
}

func TestLoadMissingMigrationFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "items.json")
	if err := Save(path, 1, []item{{ID: "1"}}); err != nil {
		t.Fatalf("save: %v", err)
	}
	var got []item
	if err := Load(path, 3, nil, &got); err == nil {
		t.Fatal("expected error for missing migration")
	}
}

func TestSaveIsAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "items.json")
	if err := Save(path, 1, []item{{ID: "1"}}); err != nil {
		t.Fatalf("save: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) == ".tmp" {
			t.Fatalf("temporary file left behind: %s", entry.Name())
		}
	}
}
