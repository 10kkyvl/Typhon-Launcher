package install

import (
	"errors"
	"path/filepath"
	"sync"
	"testing"

	"typhon/internal/download"
	"typhon/internal/library"
)

func TestRemoveGameRefusesSeedingFlatOrInPlaceIntoInstallDir(t *testing.T) {
	for _, layout := range []string{"flat", "in place"} {
		t.Run(layout, func(t *testing.T) {
			s, downloads, registrar := newTestService(t)
			dir := gameDir(t, "Seeded")
			downloads.add("d1", "Seeded", dir)
			downloads.mu.Lock()
			d := downloads.items["d1"]
			d.Flat, d.InPlace = layout == "flat", layout == "in place"
			d.Seeding = true
			downloads.items["d1"] = d
			downloads.mu.Unlock()
			registrar.put(library.Game{ID: "g1", InstallDir: dir, Owned: true, SourceDownloadID: "d1"})

			if err := s.RemoveGame("g1", RemoveOptions{DeleteFiles: true}); !errors.Is(err, errRemoveSeeding) {
				t.Fatalf("remove: %v, want errRemoveSeeding", err)
			}
			if missing(t, dir) {
				t.Fatal("files deleted while seeding from them")
			}
		})
	}
}

func TestCheckDownloadDeletableFlatDownload(t *testing.T) {
	dest := t.TempDir()
	flat := download.Download{ID: "d1", Destination: dest, Flat: true, Status: download.StatusCompleted}.WithRoot("x")
	cases := []struct {
		name  string
		setup func(s *Service, r *fakeRegistrar)
		want  error
	}{
		{"nothing there", func(*Service, *fakeRegistrar) {}, nil},
		{"game inside the destination", func(_ *Service, r *fakeRegistrar) {
			r.put(library.Game{ID: "g", Title: "G", InstallDir: filepath.Join(dest, "Game")})
		}, errDownloadIsInstall},
		{"game is the destination", func(_ *Service, r *fakeRegistrar) {
			r.put(library.Game{ID: "g", Title: "G", InstallDir: dest})
		}, errDownloadIsInstall},
		{"destination inside a game", func(_ *Service, r *fakeRegistrar) {
			r.put(library.Game{ID: "g", Title: "G", InstallDir: filepath.Dir(dest)})
		}, errDownloadIsInstall},
		{"unrelated game", func(_ *Service, r *fakeRegistrar) {
			r.put(library.Game{ID: "g", Title: "G", InstallDir: t.TempDir()})
		}, nil},
		{"install writing into the destination", func(s *Service, _ *fakeRegistrar) {
			s.items = append(s.items, &Installation{ID: "i", DownloadID: "other", Status: StatusInstalling, Destination: filepath.Join(dest, "out")})
		}, errDownloadInUse},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s, _, registrar := newTestService(t)
			s.mu.Lock()
			c.setup(s, registrar)
			s.mu.Unlock()
			if err := s.CheckDownloadDeletable(flat); !errors.Is(err, c.want) {
				t.Fatalf("CheckDownloadDeletable = %v, want %v", err, c.want)
			}
		})
	}
}

func TestInstallRefusesADownloadThatIsBeingDeleted(t *testing.T) {
	s, downloads, _ := newTestService(t)
	downloads.add("d1", "Pack", t.TempDir())
	downloads.mu.Lock()
	downloads.items["d1"] = downloads.items["d1"].WithDeleting(true)
	downloads.mu.Unlock()

	if _, err := s.completedDownload("d1"); !errors.Is(err, errDownloadDeleting) {
		t.Fatalf("completedDownload = %v, want errDownloadDeleting", err)
	}
	if _, err := s.Start("d1", StartOptions{Destination: filepath.Join(t.TempDir(), "G")}); !errors.Is(err, errDownloadDeleting) {
		t.Fatalf("Start = %v, want errDownloadDeleting", err)
	}
}

func TestClaimLockedSeesAClaimMadeAfterTheCheck(t *testing.T) {
	s, downloads, _ := newTestService(t)
	downloads.add("d1", "Pack", t.TempDir())
	item := &Installation{ID: "i", DownloadID: "d1", Status: StatusPending}

	s.mu.Lock()
	err := s.claimLocked(item)
	s.mu.Unlock()
	if err != nil {
		t.Fatalf("claimLocked on a free download = %v", err)
	}
	downloads.mu.Lock()
	downloads.items["d1"] = downloads.items["d1"].WithDeleting(true)
	downloads.mu.Unlock()
	s.mu.Lock()
	err = s.claimLocked(item)
	s.mu.Unlock()
	if !errors.Is(err, errDownloadDeleting) {
		t.Fatalf("claimLocked on a claimed download = %v, want errDownloadDeleting", err)
	}
}

// claimingDownloads follows the manager's order: the claim is made before the
// guard is asked.
type claimingDownloads struct {
	*fakeDownloads
	s *Service
}

func (c *claimingDownloads) deleteData(id string) error {
	c.mu.Lock()
	c.items[id] = c.items[id].WithDeleting(true)
	snap := c.items[id]
	c.mu.Unlock()
	if err := c.s.CheckDownloadDeletable(snap); err != nil {
		c.mu.Lock()
		c.items[id] = c.items[id].WithDeleting(false)
		c.mu.Unlock()
		return err
	}
	c.mu.Lock()
	delete(c.items, id)
	c.mu.Unlock()
	return nil
}

func TestInstallAndDeleteNeverBothWin(t *testing.T) {
	for round := 0; round < 200; round++ {
		s, downloads, _ := newTestService(t)
		downloads.add("d1", "Pack", t.TempDir())
		c := &claimingDownloads{fakeDownloads: downloads, s: s}
		item := &Installation{ID: "i", DownloadID: "d1", Status: StatusPending, Destination: filepath.Join(t.TempDir(), "G")}

		var installed, deleted bool
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			s.mu.Lock()
			defer s.mu.Unlock()
			s.items = append(s.items, item)
			if err := s.claimLocked(item); err != nil {
				s.items = s.items[:len(s.items)-1]
				return
			}
			installed = true
		}()
		go func() {
			defer wg.Done()
			deleted = c.deleteData("d1") == nil
		}()
		wg.Wait()
		if installed && deleted {
			t.Fatalf("round %d: the install started on a download whose data was deleted", round)
		}
		if !installed && !deleted {
			t.Fatalf("round %d: neither side won, the download was refused twice", round)
		}
	}
}

func TestReservePartial(t *testing.T) {
	base := t.TempDir()
	dest := filepath.Join(base, "Game")
	partial := dest + partialSuffix
	failed := func() *Installation {
		return &Installation{ID: "i", Type: TypePortable, Status: StatusFailed, Destination: dest, Mode: ModeCopy}
	}

	t.Run("refused when the verdict is no", func(t *testing.T) {
		s, _, _ := newTestService(t)
		if _, err := s.ReservePartial(partial); !errors.Is(err, errPartialRefused) {
			t.Fatalf("ReservePartial = %v, want errPartialRefused", err)
		}
	})
	t.Run("blocks a new job on the same destination until released", func(t *testing.T) {
		s, downloads, _ := newTestService(t)
		downloads.add("d1", "Pack", t.TempDir())
		s.mu.Lock()
		s.items = append(s.items, failed())
		s.mu.Unlock()
		release, err := s.ReservePartial(partial)
		if err != nil {
			t.Fatal(err)
		}
		next := &Installation{ID: "n", DownloadID: "d1", Status: StatusPending, Destination: dest}
		s.mu.Lock()
		blocked := s.claimLocked(next)
		s.mu.Unlock()
		if !errors.Is(blocked, errPartialReserved) {
			t.Fatalf("claimLocked while reserved = %v, want errPartialReserved", blocked)
		}
		release()
		s.mu.Lock()
		free := s.claimLocked(next)
		s.mu.Unlock()
		if free != nil {
			t.Fatalf("claimLocked after release = %v", free)
		}
	})
	t.Run("a live job also vetoes the reservation", func(t *testing.T) {
		s, _, _ := newTestService(t)
		live := failed()
		live.Status = StatusExtracting
		s.mu.Lock()
		s.items = append(s.items, live)
		s.mu.Unlock()
		if _, err := s.ReservePartial(partial); !errors.Is(err, errPartialRefused) {
			t.Fatalf("ReservePartial = %v, want errPartialRefused", err)
		}
	})
}
