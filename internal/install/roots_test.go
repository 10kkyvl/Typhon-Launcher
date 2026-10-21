package install

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"typhon/internal/platform"
	"typhon/internal/settings"
)

func stubFree(s *Service, free map[string]uint64) {
	s.freeSpace = func(path string) (platform.StorageInfo, error) {
		for root, avail := range free {
			if platform.Inside(root, path) {
				return platform.StorageInfo{FreeBytes: avail}, nil
			}
		}
		return platform.StorageInfo{}, os.ErrNotExist
	}
}

func TestChooseRoot(t *testing.T) {
	base := t.TempDir()
	games := filepath.Join(base, "Lib", "Games")
	extraA := filepath.Join(base, "A")
	extraB := filepath.Join(base, "B")

	cases := []struct {
		name  string
		cfg   settings.Settings
		free  map[string]uint64
		wantR string
	}{
		{"nothing configured", settings.Settings{}, nil, ""},
		{"single root is not measured", settings.Settings{GamesPath: games}, nil, games},
		{
			"last used root wins over more free room",
			settings.Settings{GamesPath: games, ExtraLibraryPaths: []string{extraA, extraB}, LastInstallRoot: extraA},
			map[string]uint64{games: 900, extraA: 10, extraB: 500}, extraA,
		},
		{
			"last used root on a missing volume is skipped",
			settings.Settings{GamesPath: games, ExtraLibraryPaths: []string{extraA, extraB}, LastInstallRoot: extraA},
			map[string]uint64{games: 100, extraB: 500}, extraB,
		},
		{
			"most free room without a last used root",
			settings.Settings{GamesPath: games, ExtraLibraryPaths: []string{extraA, extraB}},
			map[string]uint64{games: 100, extraA: 300, extraB: 200}, extraA,
		},
		{
			"a tie keeps the main folder",
			settings.Settings{GamesPath: games, ExtraLibraryPaths: []string{extraA}},
			map[string]uint64{games: 100, extraA: 100}, games,
		},
		{
			"unreadable volumes are not candidates",
			settings.Settings{GamesPath: games, ExtraLibraryPaths: []string{extraA, extraB}},
			map[string]uint64{extraB: 5}, extraB,
		},
		{
			"no volume readable falls back to the main folder",
			settings.Settings{GamesPath: games, ExtraLibraryPaths: []string{extraA}},
			map[string]uint64{}, games,
		},
		{
			"last used root that is no longer one is ignored",
			settings.Settings{GamesPath: games, ExtraLibraryPaths: []string{extraA}, LastInstallRoot: extraB},
			map[string]uint64{games: 1, extraA: 2, extraB: 99}, extraA,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := mustServiceAt(t, t.TempDir())
			stubFree(s, tc.free)
			if got := s.chooseRoot(tc.cfg); got != tc.wantR {
				t.Fatalf("chooseRoot = %q, want %q", got, tc.wantR)
			}
		})
	}
}

func TestProposeDestinationForUsesTheChosenRoot(t *testing.T) {
	s := mustServiceAt(t, t.TempDir())
	games, extra := t.TempDir(), t.TempDir()
	stubFree(s, map[string]uint64{games: 1, extra: 2})
	mkFile(t, filepath.Join(extra, "Game", "taken.txt"), 4)
	cfg := settings.Settings{GamesPath: games, ExtraLibraryPaths: []string{extra}}

	if got, want := s.proposeDestinationFor(cfg, "Game"), filepath.Join(extra, "Game (2)"); got != want {
		t.Fatalf("destination = %q, want %q", got, want)
	}
	if got := s.proposeDestinationFor(settings.Settings{}, "Game"); got != "" {
		t.Fatalf("destination without any folder = %q, want empty", got)
	}
}

func TestInstallRootsIncludeExtraFolders(t *testing.T) {
	s, _, _ := newTestService(t)
	extra := filepath.Join(t.TempDir(), "Games2")
	if _, err := s.settings.AddLibraryFolder(context.Background(), extra); err != nil {
		t.Fatal(err)
	}
	roots := s.installRoots()
	for _, want := range s.config().GamesRoots() {
		if !slices.Contains(roots, filepath.Clean(want)) {
			t.Fatalf("installRoots = %q lacks %q", roots, want)
		}
	}
	if !slices.Contains(roots, extra) {
		t.Fatalf("installRoots = %q lacks the extra folder %q", roots, extra)
	}
}

func TestInspectDownloadListsRootsAndProposesInTheRoomiestOne(t *testing.T) {
	r := newRig(t)
	extra := filepath.Join(t.TempDir(), "Games2")
	if _, err := r.cfg.AddLibraryFolder(context.Background(), extra); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	portableSource(t, root, "Game")
	r.download("d1", "Game", root)
	games := r.cfg.GetSettings().GamesPath
	stubFree(r.s, map[string]uint64{games: 10, extra: 1 << 40})

	info, err := r.s.InspectDownload("d1")
	if err != nil {
		t.Fatalf("InspectDownload: %v", err)
	}
	if want := []string{games, extra}; !slices.Equal(info.Roots, want) {
		t.Fatalf("Roots = %q, want %q", info.Roots, want)
	}
	if want := filepath.Join(extra, "Game"); info.Plan.Destination != want {
		t.Fatalf("destination = %q, want %q", info.Plan.Destination, want)
	}
}

func TestStartRemembersTheFolderInstalledInto(t *testing.T) {
	cases := []struct {
		name string
		dest func(games, extra string) string
		want func(games, extra string) string
	}{
		{"extra folder", func(_, extra string) string { return filepath.Join(extra, "Game") }, func(_, extra string) string { return extra }},
		{"main games folder", func(games, _ string) string { return filepath.Join(games, "Game") }, func(games, _ string) string { return games }},
		{"outside every folder", func(string, string) string { return filepath.Join(t.TempDir(), "Game") }, func(string, string) string { return "" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newRig(t)
			extra := filepath.Join(t.TempDir(), "Games2")
			if _, err := r.cfg.AddLibraryFolder(context.Background(), extra); err != nil {
				t.Fatal(err)
			}
			root := t.TempDir()
			portableSource(t, root, "Game")
			r.download("d1", "Game", root)
			r.freeSpace(1<<40, nil)
			games := r.cfg.GetSettings().GamesPath

			item, err := r.s.Start("d1", StartOptions{Destination: tc.dest(games, extra), Mode: ModeCopy})
			if err != nil {
				t.Fatalf("Start: %v", err)
			}
			r.s.waitStatus(t, item.ID, StatusCompleted)
			if got, want := r.cfg.GetSettings().LastInstallRoot, tc.want(games, extra); got != want {
				t.Fatalf("LastInstallRoot = %q, want %q", got, want)
			}
		})
	}
}

func TestStartSurvivesFailingToRememberTheFolder(t *testing.T) {
	r := newRig(t)
	extra := filepath.Join(t.TempDir(), "Games2")
	if _, err := r.cfg.AddLibraryFolder(context.Background(), extra); err != nil {
		t.Fatal(err)
	}
	if err := r.cfg.AddApplier(func(prev, next settings.Settings) error {
		if prev.LastInstallRoot != next.LastInstallRoot {
			return errors.New("applier refuses")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	portableSource(t, root, "Game")
	r.download("d1", "Game", root)
	r.freeSpace(1<<40, nil)

	item, err := r.s.Start("d1", StartOptions{Destination: filepath.Join(extra, "Game"), Mode: ModeCopy})
	if err != nil {
		t.Fatalf("Start must not fail over the remembered folder: %v", err)
	}
	r.s.waitStatus(t, item.ID, StatusCompleted)
	if got := r.cfg.GetSettings().LastInstallRoot; got != "" {
		t.Fatalf("LastInstallRoot = %q, want it unchanged", got)
	}
}

func TestBrokerPinWithSeveralLibraryRoots(t *testing.T) {
	root := t.TempDir()
	first, second, third := filepath.Join(root, "g1"), filepath.Join(root, "g2"), filepath.Join(root, "g3")
	pin := brokerPin{
		DownloadID:   "d1",
		ContentRoot:  filepath.Join(root, "downloads", "game"),
		LibraryRoots: []string{first, second},
		StateDir:     filepath.Join(root, "state"),
	}
	specIn := func(dest string) workerSpec {
		spec := goodSpec(brokerPin{ContentRoot: pin.ContentRoot, LibraryRoot: first, StateDir: pin.StateDir})
		spec.Destination = dest
		return spec
	}

	cases := []struct {
		name    string
		dest    string
		wantErr bool
	}{
		{"first root", filepath.Join(first, "Game"), false},
		{"second root", filepath.Join(second, "Game"), false},
		{"the root itself", second, false},
		{"third root", filepath.Join(third, "Game"), true},
		{"escape from the second root", filepath.Join(second, "..", "g3", "Game"), true},
		{"sibling with the same prefix", second + "-evil", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateBrokerSpec(pin, specIn(tc.dest))
			if tc.wantErr && !errors.Is(err, errBrokerOutsidePin) {
				t.Fatalf("err = %v, want errBrokerOutsidePin", err)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("rejected %s: %v", tc.dest, err)
			}
		})
	}
}

func TestBrokerPinWrittenByAnOlderLauncherStillWorks(t *testing.T) {
	dir := t.TempDir()
	games := filepath.Join(dir, "games")
	path := writePinFile(t, dir, map[string]any{
		"downloadId":  "d1",
		"contentRoot": filepath.Join(dir, "content"),
		"libraryRoot": games,
		"stateDir":    filepath.Join(dir, "state"),
	})
	pin, err := readBrokerPin(path)
	if err != nil {
		t.Fatalf("readBrokerPin: %v", err)
	}
	spec := goodSpec(pin)
	spec.Destination = filepath.Join(games, "Game")
	if err := validateBrokerSpec(pin, spec); err != nil {
		t.Fatalf("old pin rejected a destination in its library: %v", err)
	}
	spec.Destination = filepath.Join(dir, "elsewhere", "Game")
	if err := validateBrokerSpec(pin, spec); !errors.Is(err, errBrokerOutsidePin) {
		t.Fatalf("err = %v, want errBrokerOutsidePin", err)
	}
}

func TestBrokerPinWithoutLibraryRootsIsRefused(t *testing.T) {
	dir := t.TempDir()
	path := writePinFile(t, dir, map[string]any{
		"downloadId":   "d1",
		"contentRoot":  filepath.Join(dir, "c"),
		"libraryRoots": []string{},
		"stateDir":     filepath.Join(dir, "s"),
	})
	if _, err := readBrokerPin(path); !errors.Is(err, errBrokerNoPin) {
		t.Fatalf("err = %v, want errBrokerNoPin", err)
	}
}

func writePinFile(t *testing.T, dir string, fields map[string]any) string {
	t.Helper()
	body, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "pin.json")
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
