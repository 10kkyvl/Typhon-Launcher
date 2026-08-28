package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"typhon/internal/selfupdate"
)

const sampleChangelog = `# Changelog

Вводный текст, который не относится ни к одной версии.

## 0.2.4 — 2026-08-28
Ярлыки игр на рабочем столе.

### Добавлено
- Ярлык игры на рабочем столе
- Длинный пункт, который
  продолжается на следующей строке

### Исправлено
- Обновление больше не возвращает удалённые ярлыки

## 0.2.3 - 2026-08-27

### Fixed
- Что-то ещё
`

func TestParseChangelog(t *testing.T) {
	notes, err := parseChangelog([]byte(sampleChangelog))
	if err != nil {
		t.Fatalf("parseChangelog() error = %v", err)
	}
	if len(notes) != 2 {
		t.Fatalf("len(notes) = %d, want 2", len(notes))
	}

	top := notes[0]
	if top.Version != "0.2.4" {
		t.Fatalf("Version = %q, want 0.2.4", top.Version)
	}
	if got := top.PublishedAt.Format("2006-01-02"); got != "2026-08-28" {
		t.Fatalf("PublishedAt = %q, want 2026-08-28", got)
	}
	if top.Summary != "Ярлыки игр на рабочем столе." {
		t.Fatalf("Summary = %q", top.Summary)
	}
	if len(top.Changes) != 3 {
		t.Fatalf("len(Changes) = %d, want 3", len(top.Changes))
	}
	if top.Changes[0].Kind != selfupdate.ChangeAdded {
		t.Fatalf("Changes[0].Kind = %q, want added", top.Changes[0].Kind)
	}
	if top.Changes[1].Text != "Длинный пункт, который продолжается на следующей строке" {
		t.Fatalf("Changes[1].Text = %q, want the continuation joined in", top.Changes[1].Text)
	}
	if top.Changes[2].Kind != selfupdate.ChangeFixed {
		t.Fatalf("Changes[2].Kind = %q, want fixed", top.Changes[2].Kind)
	}
	if notes[1].Version != "0.2.3" || notes[1].Summary != "" {
		t.Fatalf("second entry = %+v, want 0.2.3 with no summary", notes[1])
	}
	if notes[1].Changes[0].Kind != selfupdate.ChangeFixed {
		t.Fatalf("english section did not map to a kind: %+v", notes[1].Changes[0])
	}
}

func TestParseChangelogRejects(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr error
	}{
		{
			name:    "no entries",
			body:    "# Changelog\n\nПросто текст.\n",
			wantErr: errChangelogEmpty,
		},
		{
			name:    "heading without a date",
			body:    "## 0.2.4\n\n### Добавлено\n- пункт\n",
			wantErr: errChangelogHeading,
		},
		{
			name:    "unparsable date",
			body:    "## 0.2.4 — вчера\n\n### Добавлено\n- пункт\n",
			wantErr: errChangelogHeading,
		},
		{
			name:    "unknown section",
			body:    "## 0.2.4 — 2026-08-28\n\n### Придумано\n- пункт\n",
			wantErr: errChangelogSection,
		},
		{
			name:    "bullet outside a section",
			body:    "## 0.2.4 — 2026-08-28\n- пункт\n",
			wantErr: errChangelogStray,
		},
		{
			name:    "continuation without a bullet",
			body:    "## 0.2.4 — 2026-08-28\n\n### Добавлено\nпродолжение ниоткуда\n",
			wantErr: errChangelogOrphan,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseChangelog([]byte(tt.body))
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("parseChangelog() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestParseChangelogEntriesAreValidReleaseNotes(t *testing.T) {
	notes, err := parseChangelog([]byte(sampleChangelog))
	if err != nil {
		t.Fatalf("parseChangelog() error = %v", err)
	}
	for _, n := range notes {
		if err := n.Validate(); err != nil {
			t.Fatalf("%s: Validate() error = %v", n.Version, err)
		}
	}
}

func TestReleaseNotesFor(t *testing.T) {
	t.Run("trims to the limit", func(t *testing.T) {
		notes, err := releaseNotesFor([]byte(sampleChangelog), "0.2.4", 1)
		if err != nil {
			t.Fatalf("releaseNotesFor() error = %v", err)
		}
		if len(notes) != 1 || notes[0].Version != "0.2.4" {
			t.Fatalf("notes = %+v, want only 0.2.4", notes)
		}
	})

	t.Run("refuses to release a version the changelog does not describe", func(t *testing.T) {
		_, err := releaseNotesFor([]byte(sampleChangelog), "0.2.5", 10)
		if !errors.Is(err, errChangelogVersionHead) {
			t.Fatalf("releaseNotesFor() error = %v, want errChangelogVersionHead", err)
		}
	})
}

func TestBuildManifestTakesNotesFromTheChangelog(t *testing.T) {
	notes, err := parseChangelog([]byte(sampleChangelog))
	if err != nil {
		t.Fatalf("parseChangelog() error = %v", err)
	}
	artifacts := []selfupdate.Artifact{{
		OS:     "windows",
		Arch:   "amd64",
		Kind:   selfupdate.KindInstaller,
		Name:   "typhon-amd64-installer.exe",
		URL:    "https://api.example.com/launcher/download/0.2.4/typhon-amd64-installer.exe",
		Size:   1024,
		SHA256: strings.Repeat("0123456789abcdef", 4),
	}}

	m, err := buildManifest("0.2.4", "", notes[0].PublishedAt, notes, artifacts)
	if err != nil {
		t.Fatalf("buildManifest() error = %v", err)
	}
	if m.Notes != notes[0].Summary {
		t.Fatalf("Notes = %q, want the summary of the top entry", m.Notes)
	}
	if len(m.Releases) != len(notes) {
		t.Fatalf("len(Releases) = %d, want %d", len(m.Releases), len(notes))
	}

	explicit, err := buildManifest("0.2.4", "своя строка", notes[0].PublishedAt, notes, artifacts)
	if err != nil {
		t.Fatalf("buildManifest() error = %v", err)
	}
	if explicit.Notes != "своя строка" {
		t.Fatalf("Notes = %q, want the explicit -notes value to win", explicit.Notes)
	}
}

// The release workflow signs whatever this file says; a changelog that no
// longer parses, or whose top entry drifted from VERSION, breaks the release
// and is worth catching here instead.
func TestRepositoryChangelogMatchesVersion(t *testing.T) {
	root := filepath.Join("..", "..")
	data, err := os.ReadFile(filepath.Join(root, "CHANGELOG.md"))
	if err != nil {
		t.Fatalf("read CHANGELOG.md: %v", err)
	}
	rawVersion, err := os.ReadFile(filepath.Join(root, "VERSION"))
	if err != nil {
		t.Fatalf("read VERSION: %v", err)
	}
	version := strings.TrimSpace(string(rawVersion))

	notes, err := releaseNotesFor(data, version, 10)
	if err != nil {
		t.Fatalf("releaseNotesFor(CHANGELOG.md, %q) error = %v", version, err)
	}
	for _, n := range notes {
		if err := n.Validate(); err != nil {
			t.Fatalf("%s: Validate() error = %v", n.Version, err)
		}
	}
}
