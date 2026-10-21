package install

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"typhon/internal/download"
	"typhon/internal/platform"
)

func differentVolumes(string, string) bool { return false }
func sameVolumes(string, string) bool      { return true }

func bigFile(t *testing.T, path string, size int64) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	if err := f.Truncate(size); err != nil {
		t.Fatalf("truncate %s: %v", path, err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close %s: %v", path, err)
	}
}

func freeOf(bytes uint64) func(string) (platform.StorageInfo, error) {
	return func(string) (platform.StorageInfo, error) { return platform.StorageInfo{FreeBytes: bytes}, nil }
}

type deleteFails struct {
	*fakeDownloads
	err error
}

func (d *deleteFails) DeleteData(string) error { return d.err }

func TestPortableMoveRenamesOnTheSameVolume(t *testing.T) {
	cases := []struct {
		name        string
		volumes     func(a, b string) bool
		opts        func(dest string) StartOptions
		wantMode    string
		wantRenamed bool
	}{
		{"move on the same volume", sameVolumes, func(d string) StartOptions { return StartOptions{Destination: d, Mode: ModeMove} }, ModeMove, true},
		{"nothing asked defaults to move", sameVolumes, func(d string) StartOptions { return StartOptions{Destination: d} }, ModeMove, true},
		{"move across volumes copies", differentVolumes, func(d string) StartOptions { return StartOptions{Destination: d, Mode: ModeMove} }, ModeMove, false},
		{"copy keeps the source", sameVolumes, func(d string) StartOptions { return StartOptions{Destination: d, Mode: ModeCopy} }, ModeCopy, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newRig(t)
			r.s.sameVolume = tc.volumes
			previous := removeInstalledSource
			removeInstalledSource = func(string) error { return errors.New("source removal must not be what moves the data") }
			t.Cleanup(func() { removeInstalledSource = previous })
			root := t.TempDir()
			source := portableSource(t, root, "Game")
			r.download("d1", "Game", root)
			dest := filepath.Join(t.TempDir(), "Games", "Game")

			item, err := r.s.Start("d1", tc.opts(dest))
			if err != nil {
				t.Fatalf("Start: %v", err)
			}
			got := r.settle(item.ID)
			if got.Status != StatusCompleted || got.Error != "" || got.Warning != "" || got.GameID == "" {
				t.Fatalf("after install: %+v (%q, %q)", viewOf(got), got.Error, got.Warning)
			}
			if got.Mode != tc.wantMode {
				t.Fatalf("mode = %q, want %q", got.Mode, tc.wantMode)
			}
			if !exists(filepath.Join(dest, "Game.exe")) || !exists(filepath.Join(dest, "data", "content.pak")) {
				t.Fatal("the game is not in the destination")
			}
			if exists(dest + partialSuffix) {
				t.Fatal("a .partial folder was left behind")
			}
			if renamed := !exists(source); renamed != tc.wantRenamed {
				t.Fatalf("source gone = %v, want %v", renamed, tc.wantRenamed)
			}
			if games := r.reg.registered(); len(games) != 1 || games[0].InstallDir != dest {
				t.Fatalf("registered games = %+v, want one at %s", games, dest)
			}
			if tc.wantRenamed {
				if ids := r.downloads.deletedIDs(); len(ids) != 1 || ids[0] != "d1" {
					t.Fatalf("download records deleted = %v, want [d1]: the data left the downloads folder", ids)
				}
				if _, err := r.downloads.Get("d1"); !errors.Is(err, download.ErrNotFound) {
					t.Fatalf("download record after the move: %v", err)
				}
			} else if ids := r.downloads.deletedIDs(); len(ids) != 0 {
				t.Fatalf("download records deleted = %v, want none: the data is still in the downloads folder", ids)
			}
			r.assertDurable(item.ID)
		})
	}
}

func TestPortableMoveSurvivesAFailedRegistrationAfterTheRename(t *testing.T) {
	r := newRig(t)
	r.s.sameVolume = sameVolumes
	root := t.TempDir()
	source := portableSource(t, root, "Game")
	r.download("d1", "Game", root)
	dest := filepath.Join(t.TempDir(), "Games", "Game")
	lib := &lockedLibrary{fakeRegistrar: r.reg}
	lib.locked.Store(true)
	r.s.library = lib

	item, err := r.s.Start("d1", StartOptions{Destination: dest, Mode: ModeMove})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	failed := r.settle(item.ID)
	if failed.Status != StatusFailed {
		t.Fatalf("status = %s (%q), want failed: the library is locked", failed.Status, failed.Error)
	}
	if exists(source) || !exists(filepath.Join(dest, "Game.exe")) {
		t.Fatal("the folder was not moved into the destination")
	}
	onDisk := r.diskItem(item.ID)
	if onDisk.Mode != ModeMove || onDisk.ContentRoot != source || onDisk.Destination != dest {
		t.Fatalf("installations.json holds mode %q, content root %q, destination %q", onDisk.Mode, onDisk.ContentRoot, onDisk.Destination)
	}

	lib.locked.Store(false)
	if err := r.s.Retry(item.ID); err != nil {
		t.Fatalf("Retry: %v", err)
	}
	done := r.settle(item.ID)
	if done.Status != StatusCompleted || done.GameID == "" {
		t.Fatalf("after retry: %+v (%q)", viewOf(done), done.Error)
	}
	if games := r.reg.registered(); len(games) != 1 || games[0].InstallDir != dest {
		t.Fatalf("registered games = %+v, want one at %s", games, dest)
	}
	waitFor(t, "download record to go", func() bool { return len(r.downloads.deletedIDs()) == 1 })
}

func TestPortableMoveCrashRecovery(t *testing.T) {
	cases := []struct {
		name         string
		renamed      bool
		wantStatus   Status
		wantGame     bool
		wantSource   bool
		wantDeletion bool
	}{
		{"crashed after the rename, before the registration", true, StatusCompleted, true, false, true},
		{"crashed before the rename", false, StatusInterrupted, false, true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newRig(t)
			root := t.TempDir()
			source := portableSource(t, root, "Game")
			r.download("d1", "Game", root)
			games := t.TempDir()
			dest := filepath.Join(games, "Game")
			if tc.renamed {
				if err := os.Rename(source, dest); err != nil {
					t.Fatalf("simulate the rename: %v", err)
				}
			}
			r.shutdown()
			crashed := Installation{
				ID: "crash1", DownloadID: "d1", Name: "Game", Type: TypePortable, Status: StatusInstalling,
				Mode: ModeMove, SourcePath: source, ContentRoot: source, Destination: dest, StartedAt: time.Now(),
			}
			if err := newStore(r.dir).save([]Installation{crashed}); err != nil {
				t.Fatalf("write the state of the crashed install: %v", err)
			}
			r.boot()

			got := r.settle("crash1")
			if got.Status != tc.wantStatus {
				t.Fatalf("status = %s (%q), want %s", got.Status, got.Error, tc.wantStatus)
			}
			if registered := r.reg.registered(); (len(registered) == 1) != tc.wantGame {
				t.Fatalf("registered games = %+v, want a game: %v", registered, tc.wantGame)
			}
			if exists(source) != tc.wantSource {
				t.Fatalf("source exists = %v, want %v", exists(source), tc.wantSource)
			}
			if tc.wantDeletion {
				waitFor(t, "download record to go", func() bool { return len(r.downloads.deletedIDs()) == 1 })
			} else if ids := r.downloads.deletedIDs(); len(ids) != 0 {
				t.Fatalf("download records deleted = %v, want none", ids)
			}
		})
	}
}

func TestPortableMoveRefusesASourceThatIsNotTheDownloadsOwn(t *testing.T) {
	cases := []struct {
		name  string
		setup func(t *testing.T, r *rig) string
		opts  StartOptions
		want  error
	}{
		{
			name: "content sits directly in the downloads folder",
			setup: func(t *testing.T, r *rig) string {
				root := t.TempDir()
				mkFile(t, filepath.Join(root, "Game.exe"), 256<<10)
				mkFile(t, filepath.Join(root, "data", "content.pak"), 4096)
				r.download("d1", "Missing", root)
				return root
			},
			want: errMoveSource,
		},
		{
			name: "download written in place",
			setup: func(t *testing.T, r *rig) string {
				root := t.TempDir()
				source := portableSource(t, root, "Game")
				r.download("d1", "Game", root)
				r.downloads.mu.Lock()
				d := r.downloads.items["d1"]
				d.InPlace = true
				r.downloads.items["d1"] = d
				r.downloads.mu.Unlock()
				return source
			},
			want: errMoveSource,
		},
		{
			name: "unknown mode",
			setup: func(t *testing.T, r *rig) string {
				root := t.TempDir()
				source := portableSource(t, root, "Game")
				r.download("d1", "Game", root)
				return source
			},
			opts: StartOptions{Mode: "teleport"},
			want: errUnknownMode,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newRig(t)
			r.s.sameVolume = sameVolumes
			source := tc.setup(t, r)
			opts := tc.opts
			opts.Destination = filepath.Join(t.TempDir(), "Games", "Game")
			if opts.Mode == "" {
				opts.Mode = ModeMove
			}

			_, err := r.s.Start("d1", opts)
			if !errors.Is(err, tc.want) {
				t.Fatalf("Start = %v, want %v", err, tc.want)
			}
			if !exists(source) {
				t.Fatal("the source disappeared although the move was refused")
			}
			if n := len(r.s.List()); n != 0 {
				t.Fatalf("a refused Start left %d records", n)
			}
		})
	}
}

func TestPortableMoveNothingAskedCopiesWhenTheSourceIsNotTheDownloadsOwn(t *testing.T) {
	r := newRig(t)
	r.s.sameVolume = sameVolumes
	root := t.TempDir()
	mkFile(t, filepath.Join(root, "Game.exe"), 256<<10)
	mkFile(t, filepath.Join(root, "data", "content.pak"), 4096)
	r.download("d1", "Game", root)

	item, err := r.s.Start("d1", StartOptions{Destination: filepath.Join(t.TempDir(), "Games", "Game")})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if item.Mode != ModeCopy {
		t.Fatalf("mode = %q, want copy: moving would take the whole downloads folder", item.Mode)
	}
	got := r.settle(item.ID)
	if got.Status != StatusCompleted || !exists(filepath.Join(root, "Game.exe")) {
		t.Fatalf("status %s, source kept: %v", got.Status, exists(filepath.Join(root, "Game.exe")))
	}
}

func TestPortableMoveWhileSeedingCopies(t *testing.T) {
	r := newRig(t)
	r.s.sameVolume = sameVolumes
	root := t.TempDir()
	source := portableSource(t, root, "Game")
	r.download("d1", "Game", root)
	r.downloads.setSeeding("d1", true)

	item, err := r.s.Start("d1", StartOptions{Destination: filepath.Join(t.TempDir(), "Games", "Game"), Mode: ModeMove})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if item.Mode != ModeCopy {
		t.Fatalf("mode = %q, want copy: the torrent client still reads the files", item.Mode)
	}
	got := r.settle(item.ID)
	if got.Status != StatusCompleted || !exists(source) {
		t.Fatalf("status %s, source kept: %v", got.Status, exists(source))
	}
	if ids := r.downloads.deletedIDs(); len(ids) != 0 {
		t.Fatalf("download records deleted = %v, want none", ids)
	}
}

func TestPortableMoveDownloadRecordCleanup(t *testing.T) {
	cases := []struct {
		name        string
		deleteErr   error
		wantWarning bool
	}{
		{"record already gone", download.ErrNotFound, false},
		{"record cannot be removed", errors.New("the download is claimed by someone else"), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newRig(t)
			r.s.sameVolume = sameVolumes
			r.s.downloads = &deleteFails{fakeDownloads: r.downloads, err: tc.deleteErr}
			root := t.TempDir()
			portableSource(t, root, "Game")
			r.download("d1", "Game", root)

			item, err := r.s.Start("d1", StartOptions{Destination: filepath.Join(t.TempDir(), "Games", "Game"), Mode: ModeMove})
			if err != nil {
				t.Fatalf("Start: %v", err)
			}
			got := r.settle(item.ID)
			if got.Status != StatusCompleted || got.Error != "" || got.GameID == "" {
				t.Fatalf("a record that cannot go must not fail the install: %+v (%q)", viewOf(got), got.Error)
			}
			if tc.wantWarning {
				waitFor(t, "warning on the install", func() bool { return r.get(item.ID).Warning != "" })
				if w := r.diskItem(item.ID).Warning; !strings.Contains(w, tc.deleteErr.Error()) {
					t.Fatalf("persisted warning = %q, want the cause in it", w)
				}
			} else if w := r.get(item.ID).Warning; w != "" {
				t.Fatalf("warning = %q, want none", w)
			}
		})
	}
}

func TestPlanSpace(t *testing.T) {
	const copyNeed = (256<<10 + 4096) + (256<<10+4096)/20
	enough := uint64(1 << 30)
	cases := []struct {
		name    string
		dest    func(t *testing.T) string
		mode    string
		volumes func(a, b string) bool
		free    func(string) (platform.StorageInfo, error)
		want    SpaceInfo
		wantErr error
	}{
		{
			name: "copy", mode: ModeCopy, volumes: sameVolumes, free: freeOf(enough),
			want: SpaceInfo{RequiredBytes: copyNeed, FreeBytes: int64(enough), SameVolume: true, Mode: ModeCopy, Enough: true},
		},
		{
			name: "copy, not enough room", mode: ModeCopy, volumes: differentVolumes, free: freeOf(copyNeed - 1),
			want: SpaceInfo{RequiredBytes: copyNeed, FreeBytes: copyNeed - 1, Mode: ModeCopy, Enough: false},
		},
		{
			name: "move on the same volume needs only the reserve", mode: ModeMove, volumes: sameVolumes, free: freeOf(moveReserveBytes),
			want: SpaceInfo{RequiredBytes: moveReserveBytes, FreeBytes: moveReserveBytes, SameVolume: true, Mode: ModeMove, Enough: true},
		},
		{
			name: "move on the same volume below the reserve", mode: ModeMove, volumes: sameVolumes, free: freeOf(moveReserveBytes - 1),
			want: SpaceInfo{RequiredBytes: moveReserveBytes, FreeBytes: moveReserveBytes - 1, SameVolume: true, Mode: ModeMove, Enough: false},
		},
		{
			name: "move across volumes costs a copy", mode: ModeMove, volumes: differentVolumes, free: freeOf(enough),
			want: SpaceInfo{RequiredBytes: copyNeed, FreeBytes: int64(enough), Mode: ModeMove, Enough: true},
		},
		{
			name: "nothing asked means move", mode: "", volumes: sameVolumes, free: freeOf(enough),
			want: SpaceInfo{RequiredBytes: moveReserveBytes, FreeBytes: int64(enough), SameVolume: true, Mode: ModeMove, Enough: true},
		},
		{
			name: "empty destination", dest: func(*testing.T) string { return "" }, mode: ModeCopy, volumes: sameVolumes, free: freeOf(enough),
			wantErr: errNoDestination,
		},
		{
			name: "blank destination", dest: func(*testing.T) string { return "   " }, mode: ModeCopy, volumes: sameVolumes, free: freeOf(enough),
			wantErr: errNoDestination,
		},
		{
			name: "relative destination", dest: func(*testing.T) string { return filepath.Join("Games", "Game") }, mode: ModeCopy, volumes: sameVolumes, free: freeOf(enough),
			wantErr: errRelativeDestination,
		},
		{
			name: "free space cannot be read", mode: ModeCopy, volumes: sameVolumes,
			free: func(string) (platform.StorageInfo, error) {
				return platform.StorageInfo{}, errors.New("volume is gone")
			},
			wantErr: errFreeSpaceUnknown,
		},
		{
			name: "unknown mode", mode: "teleport", volumes: sameVolumes, free: freeOf(enough),
			wantErr: errUnknownMode,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newRig(t)
			r.s.sameVolume = tc.volumes
			var asked []string
			r.s.freeSpace = func(p string) (platform.StorageInfo, error) {
				asked = append(asked, p)
				return tc.free(p)
			}
			root := t.TempDir()
			portableSource(t, root, "Game")
			r.download("d1", "Game", root)
			dest := filepath.Join(t.TempDir(), "Games", "Game")
			if tc.dest != nil {
				dest = tc.dest(t)
			}

			got, err := r.s.PlanSpace("d1", dest, tc.mode)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("PlanSpace error = %v, want %v", err, tc.wantErr)
			}
			if got != tc.want {
				t.Fatalf("PlanSpace = %+v, want %+v", got, tc.want)
			}
			if tc.wantErr == nil && (len(asked) != 1 || asked[0] != dest) {
				t.Fatalf("free space asked for %v, want the entered folder %s", asked, dest)
			}
		})
	}
}

func TestPlanSpaceRefusesAnUnsafeMoveAndAMissingDownload(t *testing.T) {
	r := newRig(t)
	root := t.TempDir()
	mkFile(t, filepath.Join(root, "Game.exe"), 256<<10)
	mkFile(t, filepath.Join(root, "data", "content.pak"), 4096)
	r.download("d1", "Missing", root)
	dest := filepath.Join(t.TempDir(), "Games", "Game")

	if _, err := r.s.PlanSpace("d1", dest, ModeMove); !errors.Is(err, errMoveSource) {
		t.Fatalf("move of the shared downloads folder: %v, want errMoveSource", err)
	}
	got, err := r.s.PlanSpace("d1", dest, "")
	if err != nil || got.Mode != ModeCopy {
		t.Fatalf("nothing asked: %+v, %v, want a copy", got, err)
	}
	if _, err := r.s.PlanSpace("nope", dest, ModeCopy); !errors.Is(err, download.ErrNotFound) {
		t.Fatalf("unknown download: %v, want download.ErrNotFound", err)
	}
}

func TestInspectDownloadReportsTheModeAndTheVolume(t *testing.T) {
	cases := []struct {
		name     string
		volumes  func(a, b string) bool
		seeding  bool
		wantMode string
		wantSame bool
		wantNeed int64
	}{
		{"same volume", sameVolumes, false, ModeMove, true, moveReserveBytes},
		{"other volume", differentVolumes, false, ModeMove, false, 0},
		{"seeding", sameVolumes, true, ModeCopy, true, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newRig(t)
			r.s.sameVolume = tc.volumes
			r.s.freeSpace = freeOf(1 << 30)
			root := t.TempDir()
			portableSource(t, root, "Game")
			r.download("d1", "Game", root)
			r.downloads.setSeeding("d1", tc.seeding)

			info, err := r.s.InspectDownload("d1")
			if err != nil {
				t.Fatalf("InspectDownload: %v", err)
			}
			if info.Mode != tc.wantMode || info.SameVolume != tc.wantSame || info.Seeding != tc.seeding {
				t.Fatalf("mode %q same volume %v seeding %v, want %q %v %v", info.Mode, info.SameVolume, info.Seeding, tc.wantMode, tc.wantSame, tc.seeding)
			}
			if tc.wantNeed != 0 && info.RequiredBytes != tc.wantNeed {
				t.Fatalf("required = %d, want %d", info.RequiredBytes, tc.wantNeed)
			}
			if tc.wantNeed == 0 && info.RequiredBytes != requiredBytes(info.Plan) {
				t.Fatalf("required = %d, want the size of a copy %d", info.RequiredBytes, requiredBytes(info.Plan))
			}
		})
	}
}

func TestStartChecksSpaceByTheSameRuleAsPlanSpace(t *testing.T) {
	cases := []struct {
		name    string
		mode    string
		volumes func(a, b string) bool
		wantErr error
	}{
		{"move on the same volume needs no room for a copy", ModeMove, sameVolumes, nil},
		{"move across volumes needs room for a copy", ModeMove, differentVolumes, errNotEnoughSpace},
		{"copy needs room for a copy", ModeCopy, sameVolumes, errNotEnoughSpace},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newRig(t)
			r.s.sameVolume = tc.volumes
			r.s.freeSpace = freeOf(100 << 20)
			root := t.TempDir()
			source := portableSource(t, root, "Game")
			bigFile(t, filepath.Join(source, "data", "content.pak"), 150<<20)
			r.download("d1", "Game", root)
			dest := filepath.Join(t.TempDir(), "Games", "Game")

			item, err := r.s.Start("d1", StartOptions{Destination: dest, Mode: tc.mode})
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("Start = %v, want %v", err, tc.wantErr)
			}
			if tc.wantErr != nil {
				if exists(dest) || exists(dest+partialSuffix) {
					t.Fatal("a refused Start touched the destination")
				}
				return
			}
			if got := r.settle(item.ID); got.Status != StatusCompleted {
				t.Fatalf("status = %s (%q)", got.Status, got.Error)
			}
		})
	}
}
