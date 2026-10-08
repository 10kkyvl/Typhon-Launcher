package storage

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

type itemV3 struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Tags  []string
}

func TestSaveAndLoadRefuseEmptyPath(t *testing.T) {
	t.Chdir(t.TempDir())

	if err := Save("", 1, []item{{ID: "1"}}); err == nil {
		t.Fatal("save with an empty path must fail")
	}
	var got []item
	err := Load("", 1, nil, &got)
	if err == nil {
		t.Fatal("load with an empty path must fail")
	}
	if errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("an empty path must not read as a missing file: %v", err)
	}
	if names := dirNames(t, "."); len(names) != 0 {
		t.Fatalf("a relative file appeared in the working directory: %v", names)
	}
}

func TestSaveCreatesMissingParents(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a", "b", "items.json")
	if err := Save(path, 1, []item{{ID: "1", Title: "Первый"}}); err != nil {
		t.Fatalf("save: %v", err)
	}
	var got []item
	if err := Load(path, 1, nil, &got); err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(got) != 1 || got[0].Title != "Первый" {
		t.Fatalf("got %+v", got)
	}
}

func TestSaveUnderAFileParentFailsAndKeepsIt(t *testing.T) {
	root := t.TempDir()
	blocker := filepath.Join(root, "blocker")
	if err := os.WriteFile(blocker, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Save(filepath.Join(blocker, "items.json"), 1, []item{{ID: "1"}}); err == nil {
		t.Fatal("save under a file must fail")
	}
	got, err := os.ReadFile(filepath.Clean(blocker))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "not a directory" {
		t.Fatalf("blocker rewritten: %q", got)
	}
	if names := dirNames(t, root); len(names) != 1 {
		t.Fatalf("directory holds %v, want only the blocker", names)
	}
}

func TestSaveSecondTimeReplacesWholeDocument(t *testing.T) {
	path := filepath.Join(t.TempDir(), "items.json")
	long := make([]item, 50)
	for i := range long {
		long[i] = item{ID: "id", Title: "a fairly long title to pad the first document"}
	}
	if err := Save(path, 1, long); err != nil {
		t.Fatal(err)
	}
	if err := Save(path, 1, []item{{ID: "only"}}); err != nil {
		t.Fatal(err)
	}
	var got []item
	if err := Load(path, 1, nil, &got); err != nil {
		t.Fatalf("a shorter save left a tail of the old document: %v", err)
	}
	if len(got) != 1 || got[0].ID != "only" {
		t.Fatalf("got %+v", got)
	}
}

func TestSaveUnencodableDataKeepsStoredFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "items.json")
	if err := Save(path, 1, []item{{ID: "1"}}); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatal(err)
	}

	if err := Save(path, 1, make(chan int)); err == nil {
		t.Fatal("a channel cannot be encoded")
	}

	after, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("a failed save changed the stored file")
	}
	if names := dirNames(t, dir); len(names) != 1 {
		t.Fatalf("directory holds %v, want only the target", names)
	}
}

func TestLoadNeverRewritesTheFile(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		wantErr bool
	}{
		{"corrupt", `{"version":1,"data":[{"id":`, true},
		{"legacy without migration", `[{"id":"1","title":"a"}]`, true},
		{"newer than supported", `{"version":9,"data":[]}`, true},
		{"valid", `{"version":1,"data":[{"id":"1","title":"a"}]}`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "items.json")
			if err := os.WriteFile(path, []byte(tc.raw), 0o600); err != nil {
				t.Fatal(err)
			}
			var got []item
			if err := Load(path, 1, nil, &got); (err != nil) != tc.wantErr {
				t.Fatalf("load error = %v, want error: %v", err, tc.wantErr)
			}
			after, err := os.ReadFile(filepath.Clean(path))
			if err != nil {
				t.Fatal(err)
			}
			if string(after) != tc.raw {
				t.Fatalf("file rewritten: %q", after)
			}
		})
	}
}

func TestLoadRejectsDamagedDocuments(t *testing.T) {
	cases := []struct {
		name string
		raw  []byte
	}{
		{"only nul bytes after a torn write", bytes.Repeat([]byte{0}, 64)},
		{"utf-8 byte order mark", append([]byte{0xEF, 0xBB, 0xBF}, []byte(`{"version":1,"data":[]}`)...)},
		{"trailing garbage", []byte(`{"version":1,"data":[]} garbage`)},
		{"version as a string", []byte(`{"version":"1","data":[]}`)},
		{"envelope without data", []byte(`{"version":1}`)},
		{"data of the wrong shape", []byte(`{"version":1,"data":{"id":"1"}}`)},
		{"whitespace only", []byte("  \n\t ")},
		{"two documents in a row", []byte(`{"version":1,"data":[]}{"version":1,"data":[]}`)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "items.json")
			if err := os.WriteFile(path, tc.raw, 0o600); err != nil {
				t.Fatal(err)
			}
			got := []item{{ID: "untouched"}}
			err := Load(path, 1, nil, &got)
			if err == nil {
				t.Fatal("expected an error")
			}
			if errors.Is(err, fs.ErrNotExist) {
				t.Fatalf("a damaged file was reported as missing: %v", err)
			}
			if len(got) != 1 || got[0].ID != "untouched" {
				t.Fatalf("destination changed on failure: %+v", got)
			}
		})
	}
}

func TestLoadLegacyDocumentWithoutMigrationIsRefused(t *testing.T) {
	path := filepath.Join(t.TempDir(), "items.json")
	if err := os.WriteFile(path, []byte(`[{"id":"1","title":"a"}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	var got []item
	if err := Load(path, 1, nil, &got); err == nil {
		t.Fatal("a pre-envelope file must not be guessed at without a migration")
	}
	if got != nil {
		t.Fatalf("destination filled from an unmigrated document: %+v", got)
	}
}

func TestLoadNullDataLeavesDestinationUntouched(t *testing.T) {
	path := filepath.Join(t.TempDir(), "items.json")
	if err := os.WriteFile(path, []byte(`{"version":1,"data":null}`), 0o600); err != nil {
		t.Fatal(err)
	}
	got := []item{{ID: "preset"}}
	if err := Load(path, 1, nil, &got); err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(got) != 1 || got[0].ID != "preset" {
		t.Fatalf("null data overwrote the destination: %+v", got)
	}
}

func TestLoadRunsMigrationChainInOrder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "items.json")
	if err := Save(path, 1, []item{{ID: "1", Title: "a"}}); err != nil {
		t.Fatal(err)
	}
	var order []int
	migrations := map[int]Migration{
		1: func(raw json.RawMessage) (json.RawMessage, error) {
			order = append(order, 1)
			var in []map[string]any
			if err := json.Unmarshal(raw, &in); err != nil {
				return nil, err
			}
			for _, entry := range in {
				entry["Tags"] = []string{"from-v1"}
			}
			return json.Marshal(in)
		},
		2: func(raw json.RawMessage) (json.RawMessage, error) {
			order = append(order, 2)
			var in []map[string]any
			if err := json.Unmarshal(raw, &in); err != nil {
				return nil, err
			}
			for _, entry := range in {
				title, ok := entry["title"].(string)
				if !ok {
					return nil, errors.New("title is not a string")
				}
				entry["title"] = title + "!"
			}
			return json.Marshal(in)
		},
	}

	var got []itemV3
	if err := Load(path, 3, migrations, &got); err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(order) != 2 || order[0] != 1 || order[1] != 2 {
		t.Fatalf("migrations ran as %v, want [1 2]", order)
	}
	if len(got) != 1 || got[0].Title != "a!" || len(got[0].Tags) != 1 || got[0].Tags[0] != "from-v1" {
		t.Fatalf("got %+v", got)
	}
}

func TestLoadStopsAtFailedMigration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "items.json")
	if err := Save(path, 1, []item{{ID: "1"}}); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatal(err)
	}
	boom := errors.New("migration exploded")
	laterRan := false
	migrations := map[int]Migration{
		1: func(json.RawMessage) (json.RawMessage, error) { return nil, boom },
		2: func(raw json.RawMessage) (json.RawMessage, error) {
			laterRan = true
			return raw, nil
		},
	}

	got := []item{{ID: "untouched"}}
	err = Load(path, 3, migrations, &got)
	if !errors.Is(err, boom) {
		t.Fatalf("error = %v, want it to wrap the migration failure", err)
	}
	if laterRan {
		t.Fatal("a later migration ran after an earlier one failed")
	}
	if len(got) != 1 || got[0].ID != "untouched" {
		t.Fatalf("destination changed on failure: %+v", got)
	}
	after, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("a failed migration rewrote the stored file")
	}
}
