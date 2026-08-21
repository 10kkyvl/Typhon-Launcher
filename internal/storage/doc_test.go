package storage

import (
	"encoding/json"
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

func TestLoadMissingFileIsNoop(t *testing.T) {
	var got []item
	if err := Load(filepath.Join(t.TempDir(), "absent.json"), 1, nil, &got); err != nil {
		t.Fatalf("load: %v", err)
	}
	if got != nil {
		t.Fatalf("got %+v, want nil", got)
	}
}

func TestLoadLegacyFileRunsMigrations(t *testing.T) {
	path := filepath.Join(t.TempDir(), "items.json")
	legacy := []map[string]string{{"id": "1", "name": "Старый формат"}}
	data, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
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
