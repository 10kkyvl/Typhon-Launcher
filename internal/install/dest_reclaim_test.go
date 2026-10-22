package install

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"sync"
	"testing"

	"typhon/internal/library"
)

func (r *rig) putItem(item *Installation) {
	r.t.Helper()
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	r.s.items = append(r.s.items, item)
}

func (r *rig) hasItem(id string) bool {
	r.t.Helper()
	_, ok := r.s.snapshot(id)
	return ok
}

func TestStartRefusesAFolderInsideAnotherGame(t *testing.T) {
	cases := []struct {
		name  string
		setup func(t *testing.T, r *rig, games string)
	}{
		{"library game", func(t *testing.T, r *rig, games string) {
			r.reg.put(library.Game{ID: "g1", Title: "Doom", InstallDir: filepath.Join(games, "Doom")})
		}},
		{"finished install", func(_ *testing.T, r *rig, games string) {
			r.putItem(&Installation{ID: "done1", Status: StatusCompleted, Destination: filepath.Join(games, "Doom")})
		}},
		{"running install", func(_ *testing.T, r *rig, games string) {
			r.putItem(&Installation{ID: "run1", Status: StatusInstalling, Destination: filepath.Join(games, "Doom")})
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newRig(t)
			r.s.freeSpace = freeOf(1 << 30)
			zipDownload(t, r)
			games := t.TempDir()
			mkFile(t, filepath.Join(games, "Doom", "d.exe"), 8)
			tc.setup(t, r, games)
			dest := filepath.Join(games, "Doom", "Sub")

			_, err := r.s.Start("d1", StartOptions{Destination: dest})
			if errCode(err) != "install.dest_inside_game" {
				t.Fatalf("Start error = %v, want install.dest_inside_game", err)
			}
			if exists(dest) {
				t.Fatal("a refused Start created the folder")
			}
		})
	}
}

func TestStartOnAFolderNamedAfterTheGameSuggestsANeighbour(t *testing.T) {
	r := newRig(t)
	r.s.freeSpace = freeOf(1 << 30)
	zipDownload(t, r)
	games := t.TempDir()
	dest := filepath.Join(games, "Game")
	mkFile(t, filepath.Join(dest, "g.exe"), 8)
	r.reg.put(library.Game{ID: "g1", Title: "Game", InstallDir: dest})

	_, err := r.s.Start("d1", StartOptions{Destination: dest})
	if errCode(err) != "install.dest_not_empty" {
		t.Fatalf("Start error = %v, want install.dest_not_empty", err)
	}
	hint := hintOf(t, err)
	if want := filepath.Join(games, "Game (2)"); hint != want {
		t.Fatalf("suggested folder = %q, want %q: a nested folder would put one game inside another", hint, want)
	}
}

var frontendHint = regexp.MustCompile(`(?s)например\s+(.+)$`)

func hintOf(t *testing.T, err error) string {
	t.Helper()
	m := frontendHint.FindStringSubmatch(err.Error())
	if m == nil {
		t.Fatalf("error %q has no hint the frontend can parse", err)
	}
	return m[1]
}

func TestDestNotEmptyHintKeepsTheFormatTheFrontendParses(t *testing.T) {
	r := newRig(t)
	r.s.freeSpace = freeOf(1 << 30)
	zipDownload(t, r)
	games := t.TempDir()
	mkFile(t, filepath.Join(games, "Other", "o.exe"), 8)

	_, err := r.s.Start("d1", StartOptions{Destination: games})
	if errCode(err) != "install.dest_not_empty" {
		t.Fatalf("Start error = %v, want install.dest_not_empty", err)
	}
	if got, want := hintOf(t, err), filepath.Join(games, "Game"); got != want {
		t.Fatalf("hint = %q, want %q", got, want)
	}
}

func TestStartOnAFileInPlaceOfTheFolder(t *testing.T) {
	r := newRig(t)
	r.s.freeSpace = freeOf(1 << 30)
	zipDownload(t, r)
	dest := filepath.Join(t.TempDir(), "Game")
	mkFile(t, dest, 8)

	_, err := r.s.Start("d1", StartOptions{Destination: dest})
	if errCode(err) != "install.dest_not_folder" {
		t.Fatalf("Start error = %v, want install.dest_not_folder", err)
	}
	if len(r.s.List()) != 0 {
		t.Fatal("a refused Start left a record")
	}
}

func TestProposeDestinationSkipsAFolderItCannotRead(t *testing.T) {
	s := mustServiceAt(t, t.TempDir())
	s.checkDest = func(p string) (destStatus, error) {
		if filepath.Base(p) == "Game" {
			return destNotEmpty, errNoAccess
		}
		return destState(p)
	}
	root := t.TempDir()

	got, err := s.proposeDestination(root, "Game")
	if err != nil {
		t.Fatalf("proposeDestination: %v", err)
	}
	if want := filepath.Join(root, "Game (2)"); got != want {
		t.Fatalf("proposeDestination = %q, want %q", got, want)
	}
}

func TestInspectDownloadSurvivesAnUnreadableSuggestion(t *testing.T) {
	r := newRig(t)
	r.s.freeSpace = freeOf(1 << 30)
	zipDownload(t, r)
	r.s.checkDest = func(p string) (destStatus, error) {
		if filepath.Base(p) == "Game" {
			return destNotEmpty, errNoAccess
		}
		return destState(p)
	}

	plan, err := r.s.InspectDownload("d1")
	if err != nil {
		t.Fatalf("InspectDownload: %v", err)
	}
	if filepath.Base(plan.Plan.Destination) != "Game (2)" {
		t.Fatalf("plan destination = %q, want a Game (2) folder", plan.Plan.Destination)
	}
}

func TestRetryOfARecordWhoseFolderIsInUse(t *testing.T) {
	r := newRig(t)
	archive := zipDownload(t, r)
	dest := filepath.Join(t.TempDir(), "Game")
	seedFailedInstall(t, r, archive, dest, true)
	r.putItem(&Installation{ID: "run1", Status: StatusInstalling, Type: TypeArchiveZip, Destination: dest})

	if err := r.s.Retry("failed1"); !errors.Is(err, errBusy) {
		t.Fatalf("Retry error = %v, want install.busy", err)
	}
	if got := r.get("failed1"); got.Status != StatusFailed {
		t.Fatalf("a refused Retry moved the record to %s", got.Status)
	}
	if !exists(filepath.Join(dest, "stale.bin")) {
		t.Fatal("a refused Retry touched the folder")
	}
}

func TestReclaimedLeftoverCannotBeRetried(t *testing.T) {
	r := newRig(t)
	r.s.freeSpace = freeOf(1 << 30)
	archive := zipDownload(t, r)
	dest := filepath.Join(t.TempDir(), "Game")
	seedFailedInstall(t, r, archive, dest, true)

	item, err := r.s.Start("d1", StartOptions{Destination: dest})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := r.s.Retry("failed1"); !errors.Is(err, errNotFound) {
		t.Fatalf("Retry of the reclaimed record = %v, want install.not_found", err)
	}
	if got := r.settle(item.ID); got.Status == StatusFailed {
		t.Fatalf("install failed: %s", got.Error)
	}
	for _, saved := range r.disk() {
		if saved.ID == "failed1" {
			t.Fatal("the reclaimed record is still in installations.json")
		}
	}
}

func TestReclaimKeepsTheRecordWhenPersistFails(t *testing.T) {
	r := newRig(t)
	r.s.freeSpace = freeOf(1 << 30)
	archive := zipDownload(t, r)
	dest := filepath.Join(t.TempDir(), "Game")
	seedFailedInstall(t, r, archive, dest, true)
	notDir := filepath.Join(t.TempDir(), "file")
	mkFile(t, notDir, 1)
	good := r.s.store
	r.s.store = newStore(filepath.Join(notDir, "state"))
	t.Cleanup(func() { r.s.store = good })

	if _, err := r.s.Start("d1", StartOptions{Destination: dest}); err == nil {
		t.Fatal("Start succeeded although the state could not be saved")
	}
	got, ok := r.s.snapshot("failed1")
	if !ok || got.Status != StatusFailed || len(r.s.List()) != 1 {
		t.Fatalf("memory out of step with the disk after a failed save: %+v", r.s.List())
	}
}

func TestReclaimFailureReleasesTheOwnershipMark(t *testing.T) {
	r := newRig(t)
	r.s.freeSpace = freeOf(1 << 30)
	archive := zipDownload(t, r)
	dest := filepath.Join(t.TempDir(), "Game")
	seedFailedInstall(t, r, archive, dest, true)
	blocker := filepath.Join(r.dir, "removals.json")
	if err := os.Mkdir(blocker, 0o755); err != nil {
		t.Fatal(err)
	}

	_, err := r.s.Start("d1", StartOptions{Destination: dest})
	if err == nil || errCode(err) == "install.dest_not_empty" {
		t.Fatalf("Start error = %v, want the failure of the cleanup", err)
	}
	if got := r.get("failed1").OwnedDestination; got != "" {
		t.Fatalf("a failed cleanup left the folder marked as created by the install: %q", got)
	}
	if err := os.Remove(blocker); err != nil {
		t.Fatal(err)
	}

	_, err = r.s.Start("d1", StartOptions{Destination: dest})
	if errCode(err) != "install.dest_not_empty" {
		t.Fatalf("second Start error = %v, want install.dest_not_empty: the folder is no longer provably a leftover", err)
	}
}

func TestLeftoverRaceWithDismissAndSecondStart(t *testing.T) {
	for i := range 20 {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			r := newRig(t)
			r.s.freeSpace = freeOf(1 << 30)
			archive := zipDownload(t, r)
			root2 := t.TempDir()
			archive2 := filepath.Join(root2, "Game", "game.zip")
			if err := os.MkdirAll(filepath.Dir(archive2), 0o755); err != nil {
				t.Fatal(err)
			}
			writeZip(t, archive2, gameZipEntries())
			r.download("d2", "Game", root2)
			dest := filepath.Join(t.TempDir(), "Game")
			seedFailedInstall(t, r, archive, dest, true)

			var (
				wg      sync.WaitGroup
				gate    = make(chan struct{})
				results [2]struct {
					item Installation
					err  error
				}
				dismissErr error
			)
			for n, download := range []string{"d1", "d2"} {
				wg.Add(1)
				go func() {
					defer wg.Done()
					<-gate
					results[n].item, results[n].err = r.s.Start(download, StartOptions{Destination: dest})
				}()
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-gate
				dismissErr = r.s.Dismiss("failed1")
			}()
			close(gate)
			wg.Wait()

			if dismissErr != nil && !errors.Is(dismissErr, errNotFound) {
				t.Fatalf("Dismiss: %v", dismissErr)
			}
			started := 0
			for _, res := range results {
				if res.err == nil {
					started++
					continue
				}
				switch errCode(res.err) {
				case "install.dest_not_empty", "install.busy", "install.partial_reserved":
				default:
					t.Fatalf("Start failed with an unexpected error: %v", res.err)
				}
			}
			if started > 1 {
				t.Fatalf("%d installs started into one folder", started)
			}
			for _, res := range results {
				if res.err == nil {
					if got := r.settle(res.item.ID); got.Status == StatusFailed {
						t.Fatalf("install failed: %s", got.Error)
					}
				}
			}
			r.s.wg.Wait()
			if r.hasItem("failed1") {
				t.Fatal("the failed record survived both the dismiss and the reclaim")
			}
			if started == 0 && !exists(filepath.Join(dest, "stale.bin")) {
				t.Fatal("nothing started, yet the files of the failed install are gone")
			}
			if started == 1 && exists(filepath.Join(dest, "stale.bin")) {
				t.Fatal("an install started but the leftover is still in its folder")
			}
		})
	}
}
