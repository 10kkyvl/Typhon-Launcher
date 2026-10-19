package platform

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestSplitSaveHint(t *testing.T) {
	tests := []struct {
		name     string
		hint     string
		token    string
		segments []string
		wantErr  bool
	}{
		{"bare token", "{game}", "", nil, true},
		{"bare appdata", "{appdata}", "", nil, true},
		{"token and slash", "{documents}/", "", nil, true},
		{"token and wildcard", "{game}/*", "", nil, true},
		{"token and wildcards", "{appdata}/*/*", "", nil, true},
		{"wildcard then name", "{game}/*/save", "game", []string{"*", "save"}, false},
		{"partial wildcard", "{game}/sa*", "game", []string{"sa*"}, false},
		{"nested", "{appdata}/Studio/Game/Saves", "appdata", []string{"Studio", "Game", "Saves"}, false},
		{"wildcard", "{documents}/My Games/*/Saves", "documents", []string{"My Games", "*", "Saves"}, false},
		{"double slash", "{game}//save", "game", []string{"save"}, false},
		{"empty", "", "", nil, true},
		{"no token", "C:/Users/x", "", nil, true},
		{"unknown token", "{windir}/x", "", nil, true},
		{"no separator", "{game}save", "", nil, true},
		{"dotdot", "{game}/../x", "", nil, true},
		{"colon", "{game}/C:/x", "", nil, true},
		{"backslash", `{game}\save`, "", nil, true},
		{"unclosed", "{game/x", "", nil, true},
		{"control char", "{game}/sa\x00ve", "", nil, true},
		{"second token", "{game}/{appdata}", "", nil, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			token, segments, err := SplitSaveHint(tc.hint)
			if tc.wantErr {
				if !errors.Is(err, ErrBadSaveHint) {
					t.Fatalf("err = %v, want ErrBadSaveHint", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if token != tc.token || !slices.Equal(segments, tc.segments) {
				t.Fatalf("got %q %v, want %q %v", token, segments, tc.token, tc.segments)
			}
		})
	}
}

func mkdirs(t *testing.T, root string, rels ...string) {
	t.Helper()
	for _, rel := range rels {
		if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(rel)), 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

func TestResolveSaveHint(t *testing.T) {
	root := t.TempDir()
	mkdirs(t, root, "Docs/My Games/Alpha/Saves", "Docs/My Games/Beta/Saves", "Docs/My Games/Gamma", "Roam/Studio/Game/Saves")
	if err := os.WriteFile(filepath.Join(root, "Roam", "Studio", "file"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	env := SaveEnv{
		TokenDocuments: filepath.Join(root, "Docs"),
		TokenAppData:   filepath.Join(root, "Roam"),
		TokenGame:      filepath.Join(root, "Missing"),
	}
	tests := []struct {
		name string
		hint string
		want []string
	}{
		{"plain", "{appdata}/Studio/Game/Saves", []string{filepath.Join(root, "Roam", "Studio", "Game", "Saves")}},
		{"wildcard in the middle", "{documents}/My Games/*/Saves", []string{
			filepath.Join(root, "Docs", "My Games", "Alpha", "Saves"),
			filepath.Join(root, "Docs", "My Games", "Beta", "Saves"),
		}},
		{"wildcard prefix is case-insensitive", "{documents}/My Games/al*/Saves", []string{filepath.Join(root, "Docs", "My Games", "Alpha", "Saves")}},
		{"missing directory", "{appdata}/Nope", nil},
		{"file is not a directory", "{appdata}/Studio/file", nil},
		{"missing game dir", "{game}/save", nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, unreadable, err := ResolveSaveHint(context.Background(), tc.hint, env)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if unreadable != 0 {
				t.Fatalf("unreadable = %d, want 0", unreadable)
			}
			if !slices.Equal(got, tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestResolveSaveHintErrors(t *testing.T) {
	root := t.TempDir()
	env := SaveEnv{TokenAppData: root, TokenDocuments: "relative/path", TokenPublic: " "}

	if _, _, err := ResolveSaveHint(context.Background(), "{steam}/x", env); !errors.Is(err, ErrSaveTokenUnavail) {
		t.Fatalf("missing token err = %v, want ErrSaveTokenUnavail", err)
	}
	if _, _, err := ResolveSaveHint(context.Background(), "{documents}/x", env); !errors.Is(err, errSaveHintNotAbsolute) {
		t.Fatalf("relative token err = %v", err)
	}
	if _, _, err := ResolveSaveHint(context.Background(), "{public}/x", env); !errors.Is(err, errSaveHintNoTokenVal) {
		t.Fatalf("blank token err = %v", err)
	}
	if _, _, err := ResolveSaveHint(context.Background(), "{appdata}/../x", env); !errors.Is(err, ErrBadSaveHint) {
		t.Fatalf("traversal err = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	mkdirs(t, root, "a")
	if _, _, err := ResolveSaveHint(ctx, "{appdata}/*/x", env); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled ctx err = %v, want context.Canceled", err)
	}
}

func TestResolveSaveHintTooBroad(t *testing.T) {
	root := t.TempDir()
	for i := range maxHintMatches + 1 {
		mkdirs(t, root, "d"+string(rune('a'+i%26))+string(rune('a'+i/26%26))+string(rune('a'+i/676)))
	}
	env := SaveEnv{TokenAppData: root}
	if _, _, err := ResolveSaveHint(context.Background(), "{appdata}/*/x", env); !errors.Is(err, ErrSaveHintTooBroad) {
		t.Fatalf("err = %v, want ErrSaveHintTooBroad", err)
	}
}

func TestWildMatch(t *testing.T) {
	tests := []struct {
		pattern, name string
		want          bool
	}{
		{"*", "anything", true},
		{"a*", "abc", true},
		{"a*", "a", true},
		{"a*a", "a", false},
		{"*b", "ab", true},
		{"a*c*e", "abcde", true},
		{"a*c*e", "abde", false},
		{"[x]", "[x]", true},
		{"exact", "exact", true},
		{"exact", "exacts", false},
	}
	for _, tc := range tests {
		if got := wildMatch(tc.pattern, tc.name); got != tc.want {
			t.Errorf("wildMatch(%q, %q) = %v, want %v", tc.pattern, tc.name, got, tc.want)
		}
	}
}

func TestIsLinkMode(t *testing.T) {
	cases := map[string]struct {
		mode fs.FileMode
		want bool
	}{
		"directory": {fs.ModeDir, false},
		"regular":   {0, false},
		"symlink":   {fs.ModeSymlink, true},
		"junction":  {fs.ModeDir | fs.ModeIrregular, true},
		"irregular": {fs.ModeIrregular, true},
	}
	for name, tc := range cases {
		if got := IsLinkMode(tc.mode); got != tc.want {
			t.Errorf("%s: IsLinkMode = %v, want %v", name, got, tc.want)
		}
	}
}

// Создание симлинка на Windows требует права; без него проверяется только
// предикат выше, а обход всё равно прогоняется на обычных каталогах.
func TestResolveSaveHintSkipsLinks(t *testing.T) {
	root := t.TempDir()
	mkdirs(t, root, "outside/secret", "real/Saves")
	linked := os.Symlink(filepath.Join(root, "outside"), filepath.Join(root, "real", "Link")) == nil
	gameLinked := os.Symlink(filepath.Join(root, "real"), filepath.Join(root, "gamelink")) == nil
	env := SaveEnv{TokenAppData: filepath.Join(root, "real"), TokenGame: filepath.Join(root, "real")}

	paths, unreadable, err := ResolveSaveHint(context.Background(), "{appdata}/Saves", env)
	if err != nil || len(paths) != 1 || unreadable != 0 {
		t.Fatalf("plain: %v %d %v", paths, unreadable, err)
	}
	if linked {
		paths, unreadable, err = ResolveSaveHint(context.Background(), "{appdata}/Link/secret", env)
		if err != nil || len(paths) != 0 || unreadable != 1 {
			t.Fatalf("literal link: %v %d %v, want skipped and counted", paths, unreadable, err)
		}
		paths, unreadable, err = ResolveSaveHint(context.Background(), "{appdata}/Li*/secret", env)
		if err != nil || len(paths) != 0 || unreadable != 1 {
			t.Fatalf("wildcard link: %v %d %v, want skipped and counted", paths, unreadable, err)
		}
	}
	if gameLinked {
		env[TokenGame] = filepath.Join(root, "gamelink")
		paths, unreadable, err = ResolveSaveHint(context.Background(), "{game}/Saves", env)
		if err != nil || len(paths) != 0 || unreadable != 1 {
			t.Fatalf("game root link: %v %d %v", paths, unreadable, err)
		}
	}
}
