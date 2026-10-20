package updates

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"typhon/internal/download"
	"typhon/internal/library"
)

func ownershipService(t *testing.T) (*Service, *fakeLibrary, *fakeDownloads, string) {
	t.Helper()
	svc, err := newServiceAt(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "Game")
	lib := &fakeLibrary{games: []library.Game{{ID: "g1", Title: "Game", InstallDir: dir}}}
	dl := newFakeDownloads()
	svc.library, svc.downloads = lib, dl
	return svc, lib, dl, dir
}

func TestDownloadInUse(t *testing.T) {
	cases := []struct {
		name  string
		setup func(s *Service, dl *fakeDownloads)
		want  bool
	}{
		{"nothing running", func(*Service, *fakeDownloads) {}, false},
		{"update job reads this download", func(s *Service, _ *fakeDownloads) {
			s.updates["g1"] = &Update{GameID: "g1", DownloadID: "d1"}
			s.jobs["g1"] = &job{}
		}, true},
		{"update job reads another download", func(s *Service, _ *fakeDownloads) {
			s.updates["g1"] = &Update{GameID: "g1", DownloadID: "other"}
			s.jobs["g1"] = &job{}
		}, false},
		{"finished update still remembers the id", func(s *Service, _ *fakeDownloads) {
			s.updates["g1"] = &Update{GameID: "g1", DownloadID: "d1"}
		}, false},
		{"repair downloads this one", func(s *Service, dl *fakeDownloads) {
			s.verifications["g1"] = &VerifyState{GameID: "g1", Repairing: true}
			dl.tasks["d1"] = &download.Download{ID: "d1", Origin: download.Origin{Purpose: download.PurposeRepair, LibraryID: "g1"}}
		}, true},
		{"repair downloads something else", func(s *Service, dl *fakeDownloads) {
			s.verifications["g1"] = &VerifyState{GameID: "g1", Repairing: true}
			dl.tasks["x"] = &download.Download{ID: "x", Origin: download.Origin{Purpose: download.PurposeRepair, LibraryID: "g1"}}
		}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			svc, _, dl, _ := ownershipService(t)
			svc.updates, svc.jobs, svc.verifications = map[string]*Update{}, map[string]*job{}, map[string]*VerifyState{}
			c.setup(svc, dl)
			if got := svc.DownloadInUse("d1"); got != c.want {
				t.Fatalf("DownloadInUse = %v, want %v", got, c.want)
			}
		})
	}
}

func TestCheckDownloadDeletableRefusesWhatAnUpdateWorksOn(t *testing.T) {
	svc, _, _, dir := ownershipService(t)
	svc.updates, svc.jobs, svc.verifications = map[string]*Update{}, map[string]*job{}, map[string]*VerifyState{}
	flat := download.Download{ID: "d9", Destination: dir, Flat: true}.WithRoot("x")
	inside := download.Download{ID: "d8", Destination: filepath.Dir(dir), Status: download.StatusCompleted}.WithRoot("Game")

	if err := svc.CheckDownloadDeletable(flat); err != nil {
		t.Fatalf("no update running, got %v", err)
	}
	svc.jobs["g1"] = &job{}
	for _, d := range []download.Download{flat, inside} {
		if err := svc.CheckDownloadDeletable(d); !errors.Is(err, errDownloadInUse) {
			t.Fatalf("CheckDownloadDeletable(%s) = %v, want errDownloadInUse", d.ID, err)
		}
	}
}

func TestPathInUse(t *testing.T) {
	svc, _, _, dir := ownershipService(t)
	svc.updates, svc.jobs, svc.verifications = map[string]*Update{}, map[string]*job{}, map[string]*VerifyState{}
	other := filepath.Join(t.TempDir(), "Other")
	if svc.PathInUse(dir) {
		t.Fatal("nothing runs yet")
	}
	svc.journals = map[string]*SwapJournal{"g1": {GameID: "g1"}}
	for path, want := range map[string]bool{
		dir:                       true,
		filepath.Join(dir, "sub"): true,
		dir + previousSuffix:      true,
		filepath.Dir(dir):         true,
		other:                     false,
	} {
		if got := svc.PathInUse(path); got != want {
			t.Fatalf("PathInUse(%s) = %v, want %v", path, got, want)
		}
	}
}

func TestWaitDownloadStopsWhenTheDownloadIsBeingDeleted(t *testing.T) {
	svc, _, dl, _ := ownershipService(t)
	svc.updates = map[string]*Update{"g1": {GameID: "g1"}}
	task := download.Download{ID: "d1", Status: download.StatusCompleted}.WithDeleting(true)
	dl.tasks["d1"] = &task
	previous := pollInterval
	pollInterval = time.Millisecond
	t.Cleanup(func() { pollInterval = previous })

	if err := svc.waitDownload(context.Background(), "g1", "d1"); !errors.Is(err, errDownloadDeleting) {
		t.Fatalf("waitDownload = %v, want errDownloadDeleting", err)
	}
}
