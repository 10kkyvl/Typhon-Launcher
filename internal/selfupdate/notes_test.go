package selfupdate

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func testNote(version string, changes ...Change) ReleaseNote {
	if len(changes) == 0 {
		changes = []Change{{Kind: ChangeFixed, Text: "что-то починили"}}
	}
	return ReleaseNote{
		Version:     version,
		PublishedAt: time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC),
		Changes:     changes,
	}
}

func versionsOf(notes []ReleaseNote) []string {
	out := make([]string, len(notes))
	for i, n := range notes {
		out[i] = n.Version
	}
	return out
}

func manyNotes(count int) []ReleaseNote {
	notes := make([]ReleaseNote, 0, count)
	for i := count; i > 0; i-- {
		notes = append(notes, testNote("1.0."+strconv.Itoa(i)))
	}
	return notes
}

func TestMergeReleaseNotes(t *testing.T) {
	capped := manyNotes(MaxReleaseNotes + 5)

	tests := []struct {
		name     string
		existing []ReleaseNote
		incoming []ReleaseNote
		want     []string
		wantErr  bool
	}{
		{
			name:     "empty state takes the manifest as is",
			incoming: []ReleaseNote{testNote("1.2.0"), testNote("1.1.0")},
			want:     []string{"1.2.0", "1.1.0"},
		},
		{
			name:     "history the server trimmed away is kept",
			existing: []ReleaseNote{testNote("1.1.0"), testNote("1.0.0")},
			incoming: []ReleaseNote{testNote("1.2.0")},
			want:     []string{"1.2.0", "1.1.0", "1.0.0"},
		},
		{
			name:     "reordered input comes back newest first",
			existing: []ReleaseNote{testNote("1.0.0")},
			incoming: []ReleaseNote{testNote("1.1.0"), testNote("2.0.0")},
			want:     []string{"2.0.0", "1.1.0", "1.0.0"},
		},
		{
			name:     "unusable version is an error, not a skipped entry",
			incoming: []ReleaseNote{{Version: "не версия", PublishedAt: time.Now(), Changes: []Change{{Kind: ChangeFixed, Text: "x"}}}},
			wantErr:  true,
		},
		{
			name:     "note without changes is an error",
			incoming: []ReleaseNote{{Version: "1.0.0", PublishedAt: time.Now()}},
			wantErr:  true,
		},
		{
			name:     "history is capped",
			incoming: capped,
			want:     versionsOf(capped[:MaxReleaseNotes]),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := mergeReleaseNotes(tt.existing, tt.incoming)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("mergeReleaseNotes() error = nil, want an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("mergeReleaseNotes() error = %v, want nil", err)
			}
			if strings.Join(versionsOf(got), ",") != strings.Join(tt.want, ",") {
				t.Fatalf("versions = %v, want %v", versionsOf(got), tt.want)
			}
		})
	}
}

func TestMergeReleaseNotesPrefersTheManifest(t *testing.T) {
	existing := []ReleaseNote{testNote("1.1.0", Change{Kind: ChangeFixed, Text: "старый текст"})}
	incoming := []ReleaseNote{testNote("1.1.0", Change{Kind: ChangeFixed, Text: "исправленный текст"})}

	merged, err := mergeReleaseNotes(existing, incoming)
	if err != nil {
		t.Fatalf("mergeReleaseNotes() error = %v", err)
	}
	if len(merged) != 1 {
		t.Fatalf("len(merged) = %d, want 1", len(merged))
	}
	if merged[0].Changes[0].Text != "исправленный текст" {
		t.Fatalf("text = %q, want the text from the manifest", merged[0].Changes[0].Text)
	}
}

func TestUnseenReleaseNotes(t *testing.T) {
	history := []ReleaseNote{testNote("1.3.0"), testNote("1.2.0"), testNote("1.1.0"), testNote("1.0.0")}

	tests := []struct {
		name     string
		history  []ReleaseNote
		lastSeen string
		current  string
		want     []string
		wantErr  bool
	}{
		{
			name:     "fresh install shows nothing",
			history:  history,
			lastSeen: "",
			current:  "1.3.0",
		},
		{
			name:     "every version stepped over is shown",
			history:  history,
			lastSeen: "1.0.0",
			current:  "1.3.0",
			want:     []string{"1.3.0", "1.2.0", "1.1.0"},
		},
		{
			name:     "a version that is not installed yet is not shown",
			history:  history,
			lastSeen: "1.1.0",
			current:  "1.2.0",
			want:     []string{"1.2.0"},
		},
		{
			name:     "nothing new once acknowledged",
			history:  history,
			lastSeen: "1.3.0",
			current:  "1.3.0",
		},
		{
			name:     "unusable version is an error, not an empty list",
			history:  []ReleaseNote{testNote("1.0.0")},
			lastSeen: "не версия",
			current:  "1.0.0",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := unseenReleaseNotes(tt.history, tt.lastSeen, tt.current)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("unseenReleaseNotes() error = nil, want an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unseenReleaseNotes() error = %v, want nil", err)
			}
			if strings.Join(versionsOf(got), ",") != strings.Join(tt.want, ",") {
				t.Fatalf("versions = %v, want %v", versionsOf(got), tt.want)
			}
		})
	}
}

func TestNotesStoreMissingFileIsEmptyState(t *testing.T) {
	store := mustNotesStore(t, t.TempDir())
	v, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v, want nil for a missing file", err)
	}
	if len(v.Releases) != 0 || v.LastSeenVersion != "" {
		t.Fatalf("Load() = %+v, want the zero state", v)
	}
}

func TestNotesStoreRoundTrip(t *testing.T) {
	store := mustNotesStore(t, t.TempDir())
	want := notesState{Releases: []ReleaseNote{testNote("1.2.0"), testNote("1.1.0")}, LastSeenVersion: "1.1.0"}
	if err := store.Save(want); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	got, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.LastSeenVersion != want.LastSeenVersion || len(got.Releases) != len(want.Releases) {
		t.Fatalf("Load() = %+v, want %+v", got, want)
	}
	if got.Releases[0].Changes[0].Text != want.Releases[0].Changes[0].Text {
		t.Fatalf("change text = %q, want %q", got.Releases[0].Changes[0].Text, want.Releases[0].Changes[0].Text)
	}
}

func TestNotesStoreRefusesToSaveAfterFailedLoad(t *testing.T) {
	tests := map[string]string{
		"malformed json":       `{"version":1,"data":{`,
		"unordered history":    `{"version":1,"data":{"releases":[{"version":"1.0.0","publishedAt":"2026-08-28T12:00:00Z","changes":[{"kind":"fixed","text":"x"}]},{"version":"2.0.0","publishedAt":"2026-08-28T12:00:00Z","changes":[{"kind":"fixed","text":"x"}]}]}}`,
		"unknown change kind":  `{"version":1,"data":{"releases":[{"version":"1.0.0","publishedAt":"2026-08-28T12:00:00Z","changes":[{"kind":"exploded","text":"x"}]}]}}`,
		"note without changes": `{"version":1,"data":{"releases":[{"version":"1.0.0","publishedAt":"2026-08-28T12:00:00Z"}]}}`,
	}

	for name, content := range tests {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			cache, err := CacheDir(dir)
			if err != nil {
				t.Fatalf("CacheDir: %v", err)
			}
			path := filepath.Join(cache, notesName)
			writeTestFile(t, path, []byte(content))

			store := mustNotesStore(t, dir)
			if _, err := store.Load(); err == nil {
				t.Fatal("Load() error = nil, want an error for a broken file")
			}
			if err := store.Save(notesState{LastSeenVersion: "9.9.9"}); !errors.Is(err, ErrReadOnly) {
				t.Fatalf("Save() error = %v, want ErrReadOnly", err)
			}

			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read back: %v", err)
			}
			if string(data) != content {
				t.Fatalf("file changed after a refused save:\n got %s\nwant %s", data, content)
			}
		})
	}
}

func TestManifestValidateReleaseNotes(t *testing.T) {
	longText := strings.Repeat("а", MaxChangeLen+1)

	tests := []struct {
		name     string
		releases []ReleaseNote
		wantErr  error
	}{
		{name: "no notes at all is fine"},
		{name: "newest first", releases: []ReleaseNote{testNote("2.0.0"), testNote("1.0.0")}},
		{name: "oldest first", releases: []ReleaseNote{testNote("1.0.0"), testNote("2.0.0")}, wantErr: ErrUnorderedReleaseNotes},
		{name: "duplicate version", releases: []ReleaseNote{testNote("1.0.0"), testNote("1.0.0")}, wantErr: ErrUnorderedReleaseNotes},
		{name: "unknown kind", releases: []ReleaseNote{testNote("1.0.0", Change{Kind: "exploded", Text: "x"})}, wantErr: ErrInvalidChangeKind},
		{name: "empty text", releases: []ReleaseNote{testNote("1.0.0", Change{Kind: ChangeFixed, Text: ""})}, wantErr: ErrEmptyNoteText},
		{name: "control character", releases: []ReleaseNote{testNote("1.0.0", Change{Kind: ChangeFixed, Text: "строка\nи ещё"})}, wantErr: ErrInvalidNoteText},
		{name: "text too long", releases: []ReleaseNote{testNote("1.0.0", Change{Kind: ChangeFixed, Text: longText})}, wantErr: ErrInvalidNoteText},
		{name: "too many entries", releases: manyNotes(MaxReleaseNotes + 1), wantErr: ErrTooManyReleaseNotes},
		{
			name:     "missing publish date",
			releases: []ReleaseNote{{Version: "1.0.0", Changes: []Change{{Kind: ChangeFixed, Text: "x"}}}},
			wantErr:  ErrInvalidReleaseNote,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := validManifest()
			m.Releases = tt.releases
			err := m.Validate()
			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("Validate() error = %v, want nil", err)
				}
				return
			}
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Validate() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestManifestReleasesSurviveEncoding(t *testing.T) {
	m := validManifest()
	m.Releases = []ReleaseNote{testNote("1.2.3", Change{Kind: ChangeAdded, Text: "новая кнопка"})}

	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back Manifest
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(back.Releases) != 1 || back.Releases[0].Changes[0].Kind != ChangeAdded {
		t.Fatalf("releases did not survive the round trip: %+v", back.Releases)
	}
}
