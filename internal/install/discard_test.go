package install

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"typhon/internal/library"
)

func manifestArchiveSource(t *testing.T, downloads *fakeDownloads) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "Game")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(dir, "game.zip")
	writeZip(t, archive, []zipEntry{
		{name: "depotcache/1.manifest", data: []byte("m")},
		{name: "steamapps/app.acf", data: []byte("a")},
	})
	downloads.add("d1", "Game", root)
	return archive
}

func startWaiting(t *testing.T, s *Service, dest string) Installation {
	t.Helper()
	item, err := s.Start("d1", StartOptions{Destination: dest})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	waiting := s.waitStatus(t, item.ID, StatusWaitingForUser)
	if !exists(filepath.Join(dest, "depotcache", "1.manifest")) {
		t.Fatalf("archive was not extracted into %s", dest)
	}
	return waiting
}

func TestCancelWhileWaitingForUserRemovesDestinationItCreated(t *testing.T) {
	s, downloads, _ := newTestService(t)
	manifestArchiveSource(t, downloads)
	dest := filepath.Join(t.TempDir(), "Games", "PIGFACE")
	item := startWaiting(t, s, dest)
	s.waitJobDone(t, item.ID)

	if err := s.Cancel(item.ID); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	s.waitStatus(t, item.ID, StatusCancelled)
	s.wg.Wait()

	if exists(dest) {
		t.Fatalf("destination %s survived the cancel", dest)
	}
	got, _ := s.snapshot(item.ID)
	if got.OwnedDestination != "" {
		t.Fatalf("ownedDestination = %q after the cleanup, want it cleared", got.OwnedDestination)
	}
}

func TestCancelKeepsDestinationThatExistedBeforeTheInstall(t *testing.T) {
	s, downloads, _ := newTestService(t)
	manifestArchiveSource(t, downloads)
	dest := filepath.Join(t.TempDir(), "Games", "PIGFACE")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	item := startWaiting(t, s, dest)
	s.waitJobDone(t, item.ID)

	if err := s.Cancel(item.ID); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	s.waitStatus(t, item.ID, StatusCancelled)
	s.wg.Wait()

	if !exists(filepath.Join(dest, "depotcache", "1.manifest")) {
		t.Fatal("a destination that existed before the install lost its content")
	}
}

func TestDismissAfterCancelDoesNotRemoveTwice(t *testing.T) {
	s, downloads, _ := newTestService(t)
	manifestArchiveSource(t, downloads)
	dest := filepath.Join(t.TempDir(), "Games", "PIGFACE")
	item := startWaiting(t, s, dest)
	s.waitJobDone(t, item.ID)
	if err := s.Cancel(item.ID); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	s.waitStatus(t, item.ID, StatusCancelled)
	s.wg.Wait()
	if exists(dest) {
		t.Fatal("destination survived the cancel")
	}

	mkFile(t, filepath.Join(dest, "Saves", "slot1.dat"), 16)
	if err := s.Dismiss(item.ID); err != nil {
		t.Fatalf("dismiss: %v", err)
	}
	s.wg.Wait()

	if !exists(filepath.Join(dest, "Saves", "slot1.dat")) {
		t.Fatal("dismiss removed a folder that appeared after the cancel cleanup")
	}
	if _, ok := s.snapshot(item.ID); ok {
		t.Fatal("record survived the dismiss")
	}
}

func TestDismissRemovesDestinationWhenCancelCouldNot(t *testing.T) {
	s, downloads, _ := newTestService(t)
	manifestArchiveSource(t, downloads)
	dest := filepath.Join(t.TempDir(), "Games", "PIGFACE")
	item := startWaiting(t, s, dest)
	s.waitJobDone(t, item.ID)

	s.mu.Lock()
	rec := s.findLocked(item.ID)
	rec.Status = StatusCancelled
	s.mu.Unlock()
	if err := s.Dismiss(item.ID); err != nil {
		t.Fatalf("dismiss: %v", err)
	}
	s.wg.Wait()

	if exists(dest) {
		t.Fatalf("destination %s survived the dismiss of a stopped install", dest)
	}
}

func TestStoppedInstallKeepsDestinationItCannotProveSafe(t *testing.T) {
	cases := []struct {
		name  string
		setup func(t *testing.T, s *Service, reg *fakeRegistrar, item *Installation)
	}{
		{
			name: "source archive is gone, destination may be the only copy",
			setup: func(t *testing.T, _ *Service, _ *fakeRegistrar, item *Installation) {
				if err := os.Remove(item.ArchivePath); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "library game lives in the destination",
			setup: func(_ *testing.T, _ *Service, reg *fakeRegistrar, item *Installation) {
				reg.put(library.Game{ID: "g1", Title: "Game", InstallDir: item.Destination})
			},
		},
		{
			name: "another completed record uses the destination",
			setup: func(_ *testing.T, s *Service, _ *fakeRegistrar, item *Installation) {
				s.items = append(s.items, &Installation{ID: "other", Destination: item.Destination, Status: StatusCompleted})
			},
		},
		{
			name: "destination is not recorded as created by the install",
			setup: func(_ *testing.T, _ *Service, _ *fakeRegistrar, item *Installation) {
				item.OwnedDestination = ""
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, _, reg := newTestService(t)
			dir := t.TempDir()
			archive := filepath.Join(dir, "game.zip")
			writeZip(t, archive, []zipEntry{{name: "a.txt", data: []byte("a")}})
			dest := filepath.Join(dir, "Games", "Game")
			mkFile(t, filepath.Join(dest, "Game.exe"), 32)
			item := Installation{
				ID: "i1", Type: TypeArchiveZip, Status: StatusCancelled, ArchivePath: archive,
				Destination: dest, OwnedDestination: dest,
			}
			s.mu.Lock()
			tc.setup(t, s, reg, &item)
			s.items = append(s.items, &item)
			release := s.holdDiscardLocked(&item)
			s.mu.Unlock()
			s.discardAfter(item, release)

			if !exists(filepath.Join(dest, "Game.exe")) {
				t.Fatal("destination content was removed")
			}
		})
	}
}

func TestJobEndingRemovesDestinationOnlyWhenCancelled(t *testing.T) {
	cases := []struct {
		name      string
		cancelled bool
		cause     error
		want      Status
		removed   bool
	}{
		{name: "cancelled by the user", cancelled: true, cause: context.Canceled, want: StatusCancelled, removed: true},
		{name: "failed after the files were committed keeps them for a retry", cause: errors.New("library is locked"), want: StatusFailed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, _, _ := newTestService(t)
			dir := t.TempDir()
			archive := filepath.Join(dir, "game.zip")
			writeZip(t, archive, []zipEntry{{name: "a.txt", data: []byte("a")}})
			dest := filepath.Join(dir, "Games", "Game")
			mkFile(t, filepath.Join(dest, "Game.exe"), 32)
			s.mu.Lock()
			s.items = append(s.items, &Installation{
				ID: "i1", Type: TypeArchiveZip, Status: StatusExtracting, ArchivePath: archive,
				Destination: dest, OwnedDestination: dest,
			})
			s.jobs["i1"] = &job{cancel: func() {}, cancelled: tc.cancelled}
			s.mu.Unlock()

			s.fail("i1", tc.cause)
			s.wg.Wait()

			got, _ := s.snapshot("i1")
			if got.Status != tc.want {
				t.Fatalf("status = %s, want %s", got.Status, tc.want)
			}
			if exists(dest) == tc.removed {
				t.Fatalf("destination exists = %v, removed want %v", exists(dest), tc.removed)
			}
		})
	}
}

func TestCancelKeepsPortableMoveDestinationWhoseSourceWasRenamedAway(t *testing.T) {
	s, _, _ := newTestService(t)
	dir := t.TempDir()
	dest := filepath.Join(dir, "Games", "Game")
	mkFile(t, filepath.Join(dest, "Game.exe"), 32)
	s.mu.Lock()
	item := &Installation{
		ID: "i1", Type: TypePortable, Mode: ModeMove, Status: StatusWaitingForUser,
		ContentRoot: filepath.Join(dir, "downloads", "Game"), Destination: dest, OwnedDestination: dest,
	}
	s.items = append(s.items, item)
	s.mu.Unlock()

	if err := s.Cancel("i1"); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	s.wg.Wait()

	if !exists(filepath.Join(dest, "Game.exe")) {
		t.Fatal("the moved download folder was removed after the cancel")
	}
}

func TestCreatedDestinationIsRecordedOnlyWhenAbsent(t *testing.T) {
	s, _, _ := newTestService(t)
	dir := t.TempDir()
	absent := filepath.Join(dir, "absent")
	present := filepath.Join(dir, "present")
	if err := os.MkdirAll(present, 0o755); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	s.items = append(s.items, &Installation{ID: "a"}, &Installation{ID: "p"})
	s.mu.Unlock()

	if err := s.rememberCreatedDestination("a", absent); err != nil {
		t.Fatal(err)
	}
	if err := s.rememberCreatedDestination("p", present); err != nil {
		t.Fatal(err)
	}
	if err := s.rememberCreatedDestination("a", ""); !errors.Is(err, errEmptyDestination) {
		t.Fatalf("empty destination: err = %v, want errEmptyDestination", err)
	}
	a, _ := s.snapshot("a")
	p, _ := s.snapshot("p")
	if a.OwnedDestination != absent || p.OwnedDestination != "" {
		t.Fatalf("owned: absent=%q present=%q", a.OwnedDestination, p.OwnedDestination)
	}
}
