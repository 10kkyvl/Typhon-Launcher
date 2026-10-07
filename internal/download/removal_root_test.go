package download

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// rootFixture lays out a destination with the data of a three file torrent
// named Game and, next to it, a folder that has nothing to do with it but
// carries the title the feed gave the release.
func rootFixture(t *testing.T) (dest string, game, feed string) {
	t.Helper()
	_, dest = threeFileTorrent(t)
	game = filepath.Join(dest, "Game")
	feed = filepath.Join(dest, "Feed Title")
	if err := os.MkdirAll(feed, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(feed, "keep.txt"), "not part of the torrent")
	return dest, game, feed
}

func exists(t *testing.T, path string) bool {
	t.Helper()
	_, err := os.Stat(path)
	if err == nil {
		return true
	}
	if errors.Is(err, os.ErrNotExist) {
		return false
	}
	t.Fatalf("stat %s: %v", path, err)
	return false
}

func addFeedTask(t *testing.T, m *Manager, dest string) Download {
	t.Helper()
	mi, _ := threeFileTorrent(t)
	d, err := m.AddTask(t.Context(), AddRequest{Source: writeTorrentFile(t, mi), Destination: dest, Name: "Feed Title"})
	if err != nil {
		t.Fatalf("AddTask: %v", err)
	}
	if d.Name != "Feed Title" {
		t.Fatalf("setup: the task is shown as %q", d.Name)
	}
	return d
}

func TestCancelDeletesTheFolderTheTorrentWroteNotTheOneNamedLikeTheFeedTitle(t *testing.T) {
	dest, game, feed := rootFixture(t)
	m := managerWithClient(t, 1)
	d := addFeedTask(t, m, dest)

	if err := m.Cancel(d.ID); err != nil {
		t.Fatal(err)
	}
	m.wg.Wait()

	if exists(t, game) {
		t.Error("the data of the torrent is still there")
	}
	if !exists(t, filepath.Join(feed, "keep.txt")) {
		t.Fatal("a folder that only shares the feed title with the download was deleted")
	}
}

func TestDeleteDataRemovesTheFolderTheTorrentWroteNotTheOneNamedLikeTheFeedTitle(t *testing.T) {
	dest, game, feed := rootFixture(t)
	m := managerWithClient(t, 1)
	d := addFeedTask(t, m, dest)
	setItem(m, d.ID, func(d *Download) { d.Status = StatusCompleted })

	if err := m.DeleteData(d.ID); err != nil {
		t.Fatal(err)
	}
	m.wg.Wait()

	if exists(t, game) {
		t.Error("the data of the torrent is still there")
	}
	if !exists(t, filepath.Join(feed, "keep.txt")) {
		t.Fatal("a folder that only shares the feed title with the download was deleted")
	}
}

func TestADownloadStartedFromMetadataRemovesItsOwnFolder(t *testing.T) {
	mi, dest := threeFileTorrent(t)
	hash := mi.HashInfoBytes().HexString()
	m := managerWithClient(t, 1)
	if _, err := m.FetchMetadata(writeTorrentFile(t, mi)); err != nil {
		t.Fatal(err)
	}
	d, err := m.StartDownload(hash, dest, nil)
	if err != nil {
		t.Fatal(err)
	}

	if err := m.Cancel(d.ID); err != nil {
		t.Fatal(err)
	}
	m.wg.Wait()

	if exists(t, filepath.Join(dest, "Game")) {
		t.Fatal("the data of the torrent is still there")
	}
}

func TestTheFolderOfADownloadSurvivesARestartWithoutTheCachedTorrent(t *testing.T) {
	dest, game, feed := rootFixture(t)
	first := managerWithClient(t, 1)
	d := addFeedTask(t, first, dest)
	if got := persistedRecords(t, first)[0].Root; got != "Game" {
		t.Fatalf("persisted root = %q, want the name the torrent carries", got)
	}
	if err := os.RemoveAll(filepath.Join(first.store.dir, "torrents")); err != nil {
		t.Fatal(err)
	}

	second := loadStored(t, first.store.dir)
	if err := second.Cancel(d.ID); err != nil {
		t.Fatal(err)
	}
	second.wg.Wait()

	if exists(t, game) {
		t.Error("the data of the torrent is still there")
	}
	if !exists(t, filepath.Join(feed, "keep.txt")) {
		t.Fatal("a folder that only shares the feed title with the download was deleted")
	}
}

func TestARecordWithoutARootTakesItFromTheCachedTorrent(t *testing.T) {
	mi, _ := threeFileTorrent(t)
	hash := mi.HashInfoBytes().HexString()
	dest, game, feed := rootFixture(t)
	dir := t.TempDir()
	s := newStore(dir)
	if err := s.saveMetainfo(hash, mi); err != nil {
		t.Fatal(err)
	}
	stored := storedRecord(hash, nil, 60000)
	stored.Name, stored.Destination = "Feed Title", dest
	if err := s.save([]record{stored}); err != nil {
		t.Fatal(err)
	}
	m := loadStored(t, dir)

	if err := m.Cancel("a"); err != nil {
		t.Fatal(err)
	}
	m.wg.Wait()

	if exists(t, game) {
		t.Error("the data of the torrent is still there")
	}
	if !exists(t, filepath.Join(feed, "keep.txt")) {
		t.Fatal("a folder that only shares the feed title with the download was deleted")
	}
}

func TestNothingIsDeletedWhenTheFolderOfADownloadCannotBeDetermined(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(r *record)
	}{
		{"an old record whose torrent is not cached", func(r *record) {}},
		{"a stored root that climbs out of the destination", func(r *record) { r.Root = "../Feed Title" }},
		{"a stored root with a separator in it", func(r *record) { r.Root = "Game/../Feed Title" }},
		{"a flat download has no folder of its own", func(r *record) { r.Flat, r.Root = true, "Game" }},
	}
	for _, c := range cases {
		t.Run(c.name+" is not cancelled with its data", func(t *testing.T) {
			dest, game, feed := rootFixture(t)
			dir := t.TempDir()
			stored := storedRecord("aaaa", nil, 60000)
			stored.Name, stored.Destination = "Feed Title", dest
			c.mutate(&stored)
			if err := newStore(dir).save([]record{stored}); err != nil {
				t.Fatal(err)
			}
			m := loadStored(t, dir)

			if err := m.Cancel("a"); err != nil {
				t.Fatalf("Cancel stops the download whatever becomes of its files: %v", err)
			}
			m.wg.Wait()

			if _, err := m.Get("a"); !errors.Is(err, errNotFound) {
				t.Fatalf("the download is still tracked: %v", err)
			}
			if !exists(t, filepath.Join(game, "a.bin")) || !exists(t, filepath.Join(feed, "keep.txt")) {
				t.Fatal("files were deleted although nothing says which folder is the download's")
			}
		})
		if c.name == "a flat download has no folder of its own" {
			continue
		}
		t.Run(c.name+" refuses to delete the data of a finished download", func(t *testing.T) {
			dest, game, feed := rootFixture(t)
			dir := t.TempDir()
			stored := storedRecord("aaaa", nil, 60000)
			stored.Name, stored.Destination, stored.Status = "Feed Title", dest, StatusCompleted
			c.mutate(&stored)
			if err := newStore(dir).save([]record{stored}); err != nil {
				t.Fatal(err)
			}
			m := loadStored(t, dir)

			err := m.DeleteData("a")
			m.wg.Wait()

			if !errors.Is(err, errRootUnknown) {
				t.Fatalf("error = %v, want errRootUnknown", err)
			}
			if _, getErr := m.Get("a"); getErr != nil {
				t.Fatalf("the record was dropped although nothing was deleted: %v", getErr)
			}
			if !exists(t, filepath.Join(game, "a.bin")) || !exists(t, filepath.Join(feed, "keep.txt")) {
				t.Fatal("files were deleted although nothing says which folder is the download's")
			}
		})
	}
}
