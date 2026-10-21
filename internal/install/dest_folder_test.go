package install

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"typhon/internal/library"
)

func zipDownload(t *testing.T, r *rig) string {
	t.Helper()
	root := t.TempDir()
	archive := filepath.Join(root, "Game", "game.zip")
	if err := os.MkdirAll(filepath.Dir(archive), 0o755); err != nil {
		t.Fatal(err)
	}
	writeZip(t, archive, gameZipEntries())
	r.download("d1", "Game", root)
	return archive
}

func seedFailedInstall(t *testing.T, r *rig, archive, dest string, owned bool) {
	t.Helper()
	mkFile(t, filepath.Join(dest, "stale.bin"), 32)
	item := &Installation{
		ID: "failed1", DownloadID: "d1", Type: TypeArchiveZip, Status: StatusFailed,
		ArchivePath: archive, Destination: dest, Owned: true,
	}
	if owned {
		item.OwnedDestination = dest
	}
	r.s.mu.Lock()
	r.s.items = append(r.s.items, item)
	r.s.mu.Unlock()
}

var errNoAccess = errors.New("access is denied")

func TestPlanSpaceJudgesTheDestinationFolder(t *testing.T) {
	cases := []struct {
		name  string
		setup func(t *testing.T, r *rig, archive, games string)
		// dest и wantSuggested — относительно games ("" — сама папка games).
		dest          string
		wantBusy      bool
		wantReusable  bool
		wantSuggested string
		wantErr       error
	}{
		{name: "folder does not exist", dest: "Game", wantSuggested: "Game"},
		{
			name: "folder is empty", dest: "Game", wantSuggested: "Game",
			setup: func(t *testing.T, _ *rig, _, games string) {
				if err := os.MkdirAll(filepath.Join(games, "Game"), 0o755); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "games folder with other games gets a subfolder", wantBusy: true, wantSuggested: "Game",
			setup: func(t *testing.T, _ *rig, _, games string) { mkFile(t, filepath.Join(games, "Other", "o.exe"), 8) },
		},
		{
			name: "taken subfolder is numbered", wantBusy: true, wantSuggested: "Game (3)",
			setup: func(t *testing.T, _ *rig, _, games string) {
				mkFile(t, filepath.Join(games, "Game", "g.exe"), 8)
				mkFile(t, filepath.Join(games, "Game (2)", "g.exe"), 8)
			},
		},
		{
			name: "folder left by a failed install is reusable", dest: "Game", wantReusable: true, wantSuggested: "Game",
			setup: func(t *testing.T, r *rig, archive, games string) {
				seedFailedInstall(t, r, archive, filepath.Join(games, "Game"), true)
			},
		},
		{
			name: "failed install that did not create the folder does not own it", dest: "Game", wantBusy: true, wantSuggested: filepath.Join("Game", "Game"),
			setup: func(t *testing.T, r *rig, archive, games string) {
				seedFailedInstall(t, r, archive, filepath.Join(games, "Game"), false)
			},
		},
		{
			name: "failed install whose archive is gone may hold the only copy", dest: "Game", wantBusy: true, wantSuggested: filepath.Join("Game", "Game"),
			setup: func(t *testing.T, r *rig, _, games string) {
				seedFailedInstall(t, r, filepath.Join(t.TempDir(), "gone.zip"), filepath.Join(games, "Game"), true)
			},
		},
		{
			name: "failed install whose folder is a library game", dest: "Game", wantBusy: true, wantSuggested: filepath.Join("Game", "Game"),
			setup: func(t *testing.T, r *rig, archive, games string) {
				dest := filepath.Join(games, "Game")
				seedFailedInstall(t, r, archive, dest, true)
				r.reg.put(library.Game{ID: "g1", Title: "Game", InstallDir: dest})
			},
		},
		{
			name: "folder cannot be read", dest: "Game", wantErr: errDestUnreadable,
			setup: func(_ *testing.T, r *rig, _, _ string) {
				r.s.checkDest = func(string) (destStatus, error) { return destNotEmpty, errNoAccess }
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newRig(t)
			r.s.freeSpace = freeOf(1 << 30)
			archive := zipDownload(t, r)
			games := t.TempDir()
			if tc.setup != nil {
				tc.setup(t, r, archive, games)
			}
			dest := filepath.Join(games, tc.dest)

			got, err := r.s.PlanSpace("d1", dest, "")
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("PlanSpace error = %v, want %v", err, tc.wantErr)
			}
			if tc.wantErr != nil {
				if got != (SpaceInfo{}) {
					t.Fatalf("PlanSpace = %+v on error, want zero", got)
				}
				return
			}
			want := filepath.Join(games, tc.wantSuggested)
			if got.DestinationBusy != tc.wantBusy || got.Reusable != tc.wantReusable || got.Suggested != want {
				t.Fatalf("busy %v reusable %v suggested %q, want %v %v %q", got.DestinationBusy, got.Reusable, got.Suggested, tc.wantBusy, tc.wantReusable, want)
			}
		})
	}
}

func TestProposeDestinationFailsLoudly(t *testing.T) {
	cases := []struct {
		name    string
		check   func(string) (destStatus, error)
		wantErr error
	}{
		{"folder cannot be read", func(string) (destStatus, error) { return destNotEmpty, errNoAccess }, errDestUnreadable},
		{"every name is taken", func(string) (destStatus, error) { return destNotEmpty, nil }, errNoFreeFolder},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := mustServiceAt(t, t.TempDir())
			s.checkDest = tc.check
			got, err := s.proposeDestination(t.TempDir(), "Game")
			if !errors.Is(err, tc.wantErr) || got != "" {
				t.Fatalf("proposeDestination = %q, %v, want %v", got, err, tc.wantErr)
			}
		})
	}
}

func TestDestStateTellsEveryCaseApart(t *testing.T) {
	dir := t.TempDir()
	empty := filepath.Join(dir, "empty")
	if err := os.MkdirAll(empty, 0o755); err != nil {
		t.Fatal(err)
	}
	full := filepath.Join(dir, "full")
	mkFile(t, filepath.Join(full, "a.txt"), 1)
	file := filepath.Join(dir, "file.txt")
	mkFile(t, file, 1)

	cases := []struct {
		path string
		want destStatus
	}{
		{filepath.Join(dir, "missing"), destMissing},
		{empty, destEmpty},
		{full, destNotEmpty},
		{file, destNotEmpty},
	}
	for _, tc := range cases {
		got, err := destState(tc.path)
		if err != nil || got != tc.want {
			t.Fatalf("destState(%s) = %v, %v, want %v", tc.path, got, err, tc.want)
		}
	}
}

func TestStartOnAFolderWithFilesPointsAtASubfolder(t *testing.T) {
	r := newRig(t)
	r.s.freeSpace = freeOf(1 << 30)
	zipDownload(t, r)
	games := t.TempDir()
	mkFile(t, filepath.Join(games, "Other", "o.exe"), 8)

	_, err := r.s.Start("d1", StartOptions{Destination: games})
	if errCode(err) != "install.dest_not_empty" {
		t.Fatalf("Start error = %v, want install.dest_not_empty", err)
	}
	if want := filepath.Join(games, "Game"); !strings.Contains(err.Error(), want) {
		t.Fatalf("Start error %q does not name the suggested folder %s", err, want)
	}
	if !exists(filepath.Join(games, "Other", "o.exe")) {
		t.Fatal("a refused Start touched the folder")
	}
	if got := r.s.List(); len(got) != 0 {
		t.Fatalf("a refused Start left %+v", got)
	}
}

func TestStartOnAnUnreadableFolderIsNotTreatedAsBusy(t *testing.T) {
	r := newRig(t)
	r.s.freeSpace = freeOf(1 << 30)
	zipDownload(t, r)
	r.s.checkDest = func(string) (destStatus, error) { return destNotEmpty, errNoAccess }
	dest := filepath.Join(t.TempDir(), "Game")

	_, err := r.s.Start("d1", StartOptions{Destination: dest})
	if errCode(err) != "install.dest_unreadable" || !errors.Is(err, errNoAccess) {
		t.Fatalf("Start error = %v, want install.dest_unreadable wrapping the cause", err)
	}
	if exists(dest) || len(r.s.List()) != 0 {
		t.Fatal("a refused Start left traces")
	}
}

func TestStartRemovesTheLeftoverOfAFailedInstall(t *testing.T) {
	r := newRig(t)
	r.s.freeSpace = freeOf(1 << 30)
	archive := zipDownload(t, r)
	dest := filepath.Join(t.TempDir(), "Game")
	seedFailedInstall(t, r, archive, dest, true)

	item, err := r.s.Start("d1", StartOptions{Destination: dest})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	got := r.settle(item.ID)
	if got.Status == StatusFailed {
		t.Fatalf("install failed: %s", got.Error)
	}
	if exists(filepath.Join(dest, "stale.bin")) {
		t.Fatal("the leftover of the failed install survived")
	}
	if !exists(filepath.Join(dest, "Game", "Game.exe")) && !exists(filepath.Join(dest, "Game.exe")) {
		t.Fatalf("the archive was not extracted into %s", dest)
	}
	old := r.get("failed1")
	if old.OwnedDestination != "" {
		t.Fatalf("the failed record still owns %q", old.OwnedDestination)
	}
}

func TestStartKeepsAFolderItCannotProveIsALeftover(t *testing.T) {
	cases := []struct {
		name  string
		setup func(t *testing.T, r *rig, archive, dest string)
	}{
		{"the failed install did not create the folder", func(t *testing.T, r *rig, archive, dest string) {
			seedFailedInstall(t, r, archive, dest, false)
		}},
		{"the archive of the failed install is gone", func(t *testing.T, r *rig, _, dest string) {
			seedFailedInstall(t, r, filepath.Join(t.TempDir(), "gone.zip"), dest, true)
		}},
		{"a library game lives in the folder", func(t *testing.T, r *rig, archive, dest string) {
			seedFailedInstall(t, r, archive, dest, true)
			r.reg.put(library.Game{ID: "g1", Title: "Game", InstallDir: dest})
		}},
		{"no failed install at all", func(t *testing.T, _ *rig, _, dest string) {
			mkFile(t, filepath.Join(dest, "stale.bin"), 32)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newRig(t)
			r.s.freeSpace = freeOf(1 << 30)
			archive := zipDownload(t, r)
			dest := filepath.Join(t.TempDir(), "Game")
			tc.setup(t, r, archive, dest)

			_, err := r.s.Start("d1", StartOptions{Destination: dest})
			if errCode(err) != "install.dest_not_empty" {
				t.Fatalf("Start error = %v, want install.dest_not_empty", err)
			}
			if !exists(filepath.Join(dest, "stale.bin")) {
				t.Fatal("a folder that could not be proven a leftover lost its content")
			}
		})
	}
}

func TestDismissOfAFailedInstallRemovesWhatItCreated(t *testing.T) {
	cases := []struct {
		name        string
		owned       bool
		dropArchive bool
		wantKept    bool
	}{
		{name: "folder was created by the install", owned: true},
		{name: "folder existed before the install", owned: false, wantKept: true},
		{name: "archive is gone, folder may be the only copy", owned: true, dropArchive: true, wantKept: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newRig(t)
			archive := zipDownload(t, r)
			if tc.dropArchive {
				archive = filepath.Join(t.TempDir(), "gone.zip")
			}
			dest := filepath.Join(t.TempDir(), "Game")
			seedFailedInstall(t, r, archive, dest, tc.owned)

			if err := r.s.Dismiss("failed1"); err != nil {
				t.Fatalf("Dismiss: %v", err)
			}
			r.s.wg.Wait()

			if got := exists(filepath.Join(dest, "stale.bin")); got != tc.wantKept {
				t.Fatalf("folder content kept = %v, want %v", got, tc.wantKept)
			}
			if _, ok := r.s.snapshot("failed1"); ok {
				t.Fatal("the record survived the dismiss")
			}
		})
	}
}
