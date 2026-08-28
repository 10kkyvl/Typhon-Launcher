package library

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"typhon/internal/platform"
)

func mkdirs(t *testing.T, root string, paths ...string) {
	t.Helper()
	for _, p := range paths {
		if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(p)), 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

func savesService(t *testing.T, roots []platform.SaveRoot) *Service {
	t.Helper()
	s := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))
	s.saveRoots = func() ([]platform.SaveRoot, error) { return roots, nil }
	return s
}

func addSavesGame(t *testing.T, s *Service, title, installDir string) Game {
	t.Helper()
	exe := filepath.Join(installDir, "game.exe")
	if err := os.WriteFile(exe, []byte("stub"), 0o600); err != nil {
		t.Fatal(err)
	}
	game, err := s.AddGame(exe, title)
	if err != nil {
		t.Fatalf("add game: %v", err)
	}
	return game
}

func TestLocateSavesPrefersStoredDir(t *testing.T) {
	root := t.TempDir()
	mkdirs(t, root, "install", "picked")
	s := savesService(t, nil)
	game := addSavesGame(t, s, "Portal", filepath.Join(root, "install"))

	picked := filepath.Join(root, "picked")
	if _, err := s.SetSavesDir(game.ID, picked); err != nil {
		t.Fatalf("set saves dir: %v", err)
	}
	result, err := s.LocateSaves(context.Background(), game.ID)
	if err != nil {
		t.Fatalf("locate saves: %v", err)
	}
	if result.Path != picked {
		t.Fatalf("path = %q, want %q", result.Path, picked)
	}
}

func TestLocateSavesStoredDirGone(t *testing.T) {
	root := t.TempDir()
	mkdirs(t, root, "install", "picked", "roots/Portal")
	s := savesService(t, []platform.SaveRoot{{Path: filepath.Join(root, "roots"), Depth: 1}})
	game := addSavesGame(t, s, "Portal", filepath.Join(root, "install"))

	if _, err := s.SetSavesDir(game.ID, filepath.Join(root, "picked")); err != nil {
		t.Fatalf("set saves dir: %v", err)
	}
	if err := os.Remove(filepath.Join(root, "picked")); err != nil {
		t.Fatal(err)
	}

	result, err := s.LocateSaves(context.Background(), game.ID)
	if err != nil {
		t.Fatalf("locate saves: %v", err)
	}
	if want := filepath.Join(root, "roots", "Portal"); result.Path != want {
		t.Fatalf("path = %q, want %q", result.Path, want)
	}
}

func TestLocateSavesStoredPathIsFile(t *testing.T) {
	root := t.TempDir()
	mkdirs(t, root, "install")
	s := savesService(t, nil)
	game := addSavesGame(t, s, "Portal", filepath.Join(root, "install"))

	file := filepath.Join(root, "saves.dat")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	s.games[0].SavesDir = file
	s.mu.Unlock()

	if _, err := s.LocateSaves(context.Background(), game.ID); !errors.Is(err, errSavesNotDir) {
		t.Fatalf("err = %v, want errSavesNotDir", err)
	}
}

func TestLocateSavesUnknownGame(t *testing.T) {
	s := savesService(t, nil)
	if _, err := s.LocateSaves(context.Background(), "missing"); !errors.Is(err, errNotFound) {
		t.Fatalf("err = %v, want errNotFound", err)
	}
}

func TestLocateSavesCancelledContext(t *testing.T) {
	root := t.TempDir()
	mkdirs(t, root, "install", "roots/Portal")
	s := savesService(t, []platform.SaveRoot{{Path: filepath.Join(root, "roots"), Depth: 1}})
	game := addSavesGame(t, s, "Portal", filepath.Join(root, "install"))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.LocateSaves(ctx, game.ID); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestLocateSavesRootsFailure(t *testing.T) {
	root := t.TempDir()
	mkdirs(t, root, "install")
	s := savesService(t, nil)
	boom := errors.New("boom")
	s.saveRoots = func() ([]platform.SaveRoot, error) { return nil, boom }
	game := addSavesGame(t, s, "Portal", filepath.Join(root, "install"))

	if _, err := s.LocateSaves(context.Background(), game.ID); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want boom", err)
	}
}

func TestDetectSaves(t *testing.T) {
	oneRoot := func(name string, depth int) func(string) []platform.SaveRoot {
		return func(root string) []platform.SaveRoot {
			return []platform.SaveRoot{{Path: filepath.Join(root, name), Depth: depth}}
		}
	}
	tests := []struct {
		name       string
		title      string
		dirs       []string
		installSub []string
		roots      func(root string) []platform.SaveRoot
		wantPath   string
		wantFound  []string
	}{
		{
			name:     "single match resolves to path",
			title:    "Portal 2",
			dirs:     []string{"roots/Portal 2"},
			roots:    oneRoot("roots", 1),
			wantPath: "roots/Portal 2",
		},
		{
			name:  "several matches stay candidates",
			title: "Portal 2",
			dirs:  []string{"a/Portal 2", "b/Portal 2"},
			roots: func(root string) []platform.SaveRoot {
				return []platform.SaveRoot{
					{Path: filepath.Join(root, "a"), Depth: 1},
					{Path: filepath.Join(root, "b"), Depth: 1},
				}
			},
			wantFound: []string{"a/Portal 2", "b/Portal 2"},
		},
		{
			name:     "publisher level found at depth two",
			title:    "Portal 2",
			dirs:     []string{"roots/Valve/Portal 2"},
			roots:    oneRoot("roots", 2),
			wantPath: "roots/Valve/Portal 2",
		},
		{
			name:  "publisher level ignored at depth one",
			title: "Portal 2",
			dirs:  []string{"roots/Valve/Portal 2"},
			roots: oneRoot("roots", 1),
		},
		{
			name:       "save folder inside the install dir",
			title:      "Portal 2",
			installSub: []string{"SaveGames"},
			roots:      func(root string) []platform.SaveRoot { return nil },
			wantPath:   "install/SaveGames",
		},
		{
			name:     "punctuation and case ignored",
			title:    "The Witcher 3: Wild Hunt",
			dirs:     []string{"roots/the witcher 3 wild hunt"},
			roots:    oneRoot("roots", 1),
			wantPath: "roots/the witcher 3 wild hunt",
		},
		{
			name:     "shorter folder matches longer title",
			title:    "The Witcher 3 Wild Hunt",
			dirs:     []string{"roots/The Witcher 3"},
			roots:    oneRoot("roots", 1),
			wantPath: "roots/The Witcher 3",
		},
		{
			name:  "unrelated folders are not matched",
			title: "Portal 2",
			dirs:  []string{"roots/Microsoft", "roots/Steam"},
			roots: oneRoot("roots", 1),
		},
		{
			name:  "missing root is not an error",
			title: "Portal 2",
			roots: oneRoot("nope", 2),
		},
		{
			name:  "empty title skips the roots",
			title: "",
			dirs:  []string{"roots/Portal 2"},
			roots: oneRoot("roots", 1),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			mkdirs(t, root, "install")
			mkdirs(t, root, tt.dirs...)
			for _, sub := range tt.installSub {
				mkdirs(t, root, "install/"+sub)
			}
			installDir := filepath.Join(root, "install")
			roots := func() ([]platform.SaveRoot, error) { return tt.roots(root), nil }

			result, err := detectSaves(context.Background(), tt.title, installDir, roots)
			if err != nil {
				t.Fatalf("detect saves: %v", err)
			}
			wantPath := ""
			if tt.wantPath != "" {
				wantPath = filepath.Join(root, filepath.FromSlash(tt.wantPath))
			}
			if result.Path != wantPath {
				t.Fatalf("path = %q, want %q", result.Path, wantPath)
			}
			if len(result.Candidates) != len(tt.wantFound) {
				t.Fatalf("candidates = %v, want %v", result.Candidates, tt.wantFound)
			}
			for i, want := range tt.wantFound {
				full := filepath.Join(root, filepath.FromSlash(want))
				if result.Candidates[i] != full {
					t.Fatalf("candidate %d = %q, want %q", i, result.Candidates[i], full)
				}
			}
		})
	}
}

func TestDetectSavesDeduplicates(t *testing.T) {
	root := t.TempDir()
	mkdirs(t, root, "install", "roots/Portal 2")
	roots := func() ([]platform.SaveRoot, error) {
		return []platform.SaveRoot{
			{Path: filepath.Join(root, "roots"), Depth: 1},
			{Path: filepath.Join(root, "roots"), Depth: 2},
		}, nil
	}
	result, err := detectSaves(context.Background(), "Portal 2", filepath.Join(root, "install"), roots)
	if err != nil {
		t.Fatalf("detect saves: %v", err)
	}
	if want := filepath.Join(root, "roots", "Portal 2"); result.Path != want {
		t.Fatalf("path = %q, want %q", result.Path, want)
	}
}

func TestClassifyDirErr(t *testing.T) {
	tests := []struct {
		name           string
		err            error
		wantUnreadable int
		wantFatal      bool
	}{
		{name: "no error", err: nil},
		{name: "missing dir", err: fs.ErrNotExist},
		{name: "wrapped missing dir", err: &fs.PathError{Op: "open", Path: "x", Err: fs.ErrNotExist}},
		{name: "no permission", err: fs.ErrPermission, wantUnreadable: 1},
		{name: "wrapped no permission", err: &fs.PathError{Op: "open", Path: "x", Err: fs.ErrPermission}, wantUnreadable: 1},
		{name: "other error", err: errors.New("boom"), wantFatal: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			unreadable, fatal := classifyDirErr(tt.err)
			if unreadable != tt.wantUnreadable || fatal != tt.wantFatal {
				t.Fatalf("classifyDirErr(%v) = (%d, %v), want (%d, %v)", tt.err, unreadable, fatal, tt.wantUnreadable, tt.wantFatal)
			}
		})
	}
}

func TestMatchesTitle(t *testing.T) {
	tests := []struct {
		name  string
		title string
		dir   string
		want  bool
	}{
		{name: "exact", title: "Portal 2", dir: "Portal 2", want: true},
		{name: "case and punctuation", title: "The Witcher 3: Wild Hunt", dir: "the-witcher-3-wild-hunt", want: true},
		{name: "folder is a prefix of the title", title: "The Witcher 3 Wild Hunt", dir: "The Witcher 3", want: true},
		{name: "title is a prefix of the folder", title: "The Witcher", dir: "The Witcher 3", want: true},
		{name: "short prefix is not enough", title: "Doom Eternal", dir: "Doom", want: false},
		{name: "unrelated", title: "Portal 2", dir: "Microsoft", want: false},
		{name: "empty folder name", title: "Portal 2", dir: "...", want: false},
		{name: "empty title", title: "", dir: "Portal 2", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := matchesTitle(tt.dir, titleKey(tt.title)); got != tt.want {
				t.Fatalf("matchesTitle(%q, %q) = %v, want %v", tt.dir, tt.title, got, tt.want)
			}
		})
	}
}

func TestSetSavesDirRejectsBadInput(t *testing.T) {
	root := t.TempDir()
	mkdirs(t, root, "install")
	s := savesService(t, nil)
	game := addSavesGame(t, s, "Portal", filepath.Join(root, "install"))

	file := filepath.Join(root, "saves.dat")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		id   string
		dir  string
	}{
		{name: "empty path", id: game.ID, dir: "   "},
		{name: "missing dir", id: game.ID, dir: filepath.Join(root, "nope")},
		{name: "file instead of dir", id: game.ID, dir: file},
		{name: "unknown game", id: "missing", dir: root},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := s.SetSavesDir(tt.id, tt.dir); err == nil {
				t.Fatal("некорректный ввод должен отклоняться")
			}
		})
	}
	if got := s.GetGames()[0].SavesDir; got != "" {
		t.Fatalf("saves dir = %q, want empty", got)
	}
}

func TestSetSavesDirSurvivesReload(t *testing.T) {
	root := t.TempDir()
	mkdirs(t, root, "install", "picked")
	path := filepath.Join(t.TempDir(), "library.json")
	s := mustServiceAt(t, path)
	game := addSavesGame(t, s, "Portal", filepath.Join(root, "install"))

	picked := filepath.Join(root, "picked")
	if _, err := s.SetSavesDir(game.ID, picked); err != nil {
		t.Fatalf("set saves dir: %v", err)
	}
	reloaded := mustServiceAt(t, path).GetGames()
	if len(reloaded) != 1 || reloaded[0].SavesDir != picked {
		t.Fatalf("reloaded = %+v, want saves dir %q", reloaded, picked)
	}
}

func TestSetSavesDirRollsBackOnPersistFailure(t *testing.T) {
	root := t.TempDir()
	mkdirs(t, root, "install", "picked")
	s := savesService(t, nil)
	game := addSavesGame(t, s, "Portal", filepath.Join(root, "install"))

	s.mu.Lock()
	s.path = filepath.Join(root, "install", "game.exe", "library.json")
	s.mu.Unlock()
	if _, err := s.SetSavesDir(game.ID, filepath.Join(root, "picked")); err == nil {
		t.Fatal("ошибка записи должна дойти до вызывающего")
	}
	if got := s.GetGames()[0].SavesDir; got != "" {
		t.Fatalf("saves dir = %q, want empty after failed save", got)
	}
}
