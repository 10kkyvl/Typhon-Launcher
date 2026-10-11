package download

import (
	"encoding/json"
	"strings"
	"testing"

	"typhon/internal/settings"
)

func TestSnapshotFilesAreAnArrayInJSON(t *testing.T) {
	cases := []struct {
		name  string
		files []FileState
		want  string
	}{
		{"nil files", nil, `[]`},
		{"empty files", []FileState{}, `[]`},
		{"one file", []FileState{{Path: "a", Size: 1, Selected: true}}, `[{"path":"a","size":1,"selected":true,"bytesDone":0}]`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := &Download{ID: "a", Files: c.files}
			body, err := json.Marshal(snapshot(d))
			if err != nil {
				t.Fatal(err)
			}
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(body, &fields); err != nil {
				t.Fatal(err)
			}
			if got := string(fields["files"]); got != c.want {
				t.Fatalf("files = %s, want %s", got, c.want)
			}
		})
	}
}

func TestSnapshotFilesDoNotAliasTheDownload(t *testing.T) {
	d := &Download{ID: "a", Files: []FileState{{Path: "a", Selected: true}}}
	snap := snapshot(d)
	snap.Files[0].Selected = false
	if !d.Files[0].Selected {
		t.Fatal("changing the snapshot changed the download")
	}
}

func TestListAndGetReportAnEmptyFileListAsAnArray(t *testing.T) {
	m, _ := newManagerWithSettings(t, settings.Defaults())
	d := m.addTestItem("empty", StatusQueued)
	m.mu.Lock()
	d.Files = nil
	m.mu.Unlock()

	got, err := m.Get("empty")
	if err != nil {
		t.Fatal(err)
	}
	for name, v := range map[string]any{"Get": got, "List": m.List()} {
		body, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(body), `"files":null`) || !strings.Contains(string(body), `"files":[]`) {
			t.Fatalf("%s: files is not an empty array in %s", name, body)
		}
	}
}
