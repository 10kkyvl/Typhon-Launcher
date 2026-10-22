package download

import (
	"path/filepath"
	"testing"

	"typhon/internal/diskreason"
)

func TestPartFileVerdict(t *testing.T) {
	dest := t.TempDir()
	m := newTestManager(t, 2)
	m.addTestItem("active", StatusDownloading)
	m.addTestItem("done", StatusCompleted)
	m.addTestItem("seed", StatusCompleted)
	m.addTestItem("flat", StatusCompleted)
	m.mu.Lock()
	m.findLocked("active").Destination = dest
	m.findLocked("done").Destination = dest
	s := m.findLocked("seed")
	s.Destination = dest
	s.Seeding = true
	f := m.findLocked("flat")
	f.Destination = dest
	f.Flat = true
	f.Files = []FileState{{Path: "sub/game.bin", Size: 1, Selected: true}, {Path: "../escape.bin", Size: 1, Selected: true}}
	m.mu.Unlock()

	cases := []struct {
		name string
		path string
		want bool
		code string
	}{
		{"part of a finished record", filepath.Join(dest, "done") + PartFileSuffix, true, ""},
		{"part of a flat finished record", filepath.Join(dest, "sub", "game.bin") + PartFileSuffix, true, ""},
		{"part of a running download", filepath.Join(dest, "active") + PartFileSuffix, false, diskreason.OwnedByDownload},
		{"part of a seeding download", filepath.Join(dest, "seed") + PartFileSuffix, false, diskreason.Seeding},
		{"unsafe flat path is not claimed", filepath.Join(filepath.Dir(dest), "escape.bin") + PartFileSuffix, false, diskreason.ForeignPart},
		{"a part file no record knows", filepath.Join(dest, "browser-download.zip") + PartFileSuffix, false, diskreason.ForeignPart},
		{"empty path", "", false, diskreason.ForeignPart},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, code, reason := m.PartFileVerdict(c.path)
			if got != c.want || code != c.code {
				t.Fatalf("PartFileVerdict(%q) = %v %q (%s), want %v %q", c.path, got, code, reason, c.want, c.code)
			}
			if !got && reason == "" {
				t.Fatal("a refusal must say why")
			}
		})
	}
}
