package shortcut

import (
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestFileName(t *testing.T) {
	cases := []struct {
		name    string
		title   string
		want    string
		wantErr bool
	}{
		{
			name:  "normal title",
			title: "Half-Life 2",
			want:  "Half-Life 2.lnk",
		},
		{
			name:  "slashes and colon stripped",
			title: `Half-Life 2: Episode/One\Two`,
			want:  "Half-Life 2 EpisodeOneTwo.lnk",
		},
		{
			name:    "only forbidden characters",
			title:   `<>:"/\|?*`,
			wantErr: true,
		},
		{
			name:    "empty string",
			title:   "",
			wantErr: true,
		},
		{
			name:    "whitespace only",
			title:   "   \t\n  ",
			wantErr: true,
		},
		{
			name:  "reserved device name",
			title: "CON",
			want:  "CON_.lnk",
		},
		{
			name:  "reserved device name with extension, lowercase",
			title: "com1.exe",
			want:  "com1.exe_.lnk",
		},
		{
			name:  "trailing dot trimmed",
			title: "Game.",
			want:  "Game.lnk",
		},
		{
			name:  "control characters stripped",
			title: "Game\x00Name\x1f",
			want:  "GameName.lnk",
		},
		{
			name:  "repeated spaces collapsed",
			title: "Half   Life   2",
			want:  "Half Life 2.lnk",
		},
		{
			name:  "tabs are control characters, stripped not collapsed",
			title: "Half\tLife\t2",
			want:  "HalfLife2.lnk",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := FileName(tc.title)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("FileName(%q) = %q, want error", tc.title, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("FileName(%q) unexpected error: %v", tc.title, err)
			}
			if got != tc.want {
				t.Fatalf("FileName(%q) = %q, want %q", tc.title, got, tc.want)
			}
		})
	}
}

func TestFileNameLongMultibyteTitle(t *testing.T) {
	title := strings.Repeat("日本語ゲーム😀", 30) + ".exe"
	if utf8.RuneCountInString(title) <= maxNameRunes {
		t.Fatalf("test title too short: %d runes", utf8.RuneCountInString(title))
	}

	got, err := FileName(title)
	if err != nil {
		t.Fatalf("FileName unexpected error: %v", err)
	}
	if !utf8.ValidString(got) {
		t.Fatalf("FileName(%q) = %q is not valid UTF-8", title, got)
	}
	base := strings.TrimSuffix(got, ".lnk")
	if n := utf8.RuneCountInString(base); n > maxNameRunes {
		t.Fatalf("base name has %d runes, want <= %d", n, maxNameRunes)
	}
	if !strings.HasSuffix(got, ".lnk") {
		t.Fatalf("FileName(%q) = %q, want suffix .lnk", title, got)
	}
}

func TestRemoveMissingFileIsNotError(t *testing.T) {
	dir := t.TempDir()
	if err := Remove(filepath.Join(dir, "does-not-exist.lnk")); err != nil {
		t.Fatalf("Remove of missing file: %v", err)
	}
}
