package download

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

// bencodedComment is a torrent file that is valid and exactly size bytes long:
// a dictionary with one comment field holds the padding.
func bencodedComment(size int) []byte {
	pad := size
	for len("d7:comment")+len(strconv.Itoa(pad))+len(":")+pad+len("e") > size {
		pad--
	}
	body := make([]byte, 0, size)
	body = append(body, "d7:comment"...)
	body = strconv.AppendInt(body, int64(pad), 10)
	body = append(body, ':')
	body = append(body, bytes.Repeat([]byte{'x'}, pad)...)
	return append(body, 'e')
}

func writeBencoded(t *testing.T, size int) string {
	t.Helper()
	body := bencodedComment(size)
	if len(body) != size {
		t.Fatalf("built %d bytes, want %d", len(body), size)
	}
	path := filepath.Join(t.TempDir(), "big.torrent")
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadMetainfoFileRefusesWhatIsLargerThanTheLimit(t *testing.T) {
	cases := []struct {
		name    string
		size    int
		tooLong bool
	}{
		{"a file exactly at the limit", maxMetainfoSize, false},
		{"a file one byte over", maxMetainfoSize + 1, true},
		{"a file far over", maxMetainfoSize * 2, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path := writeBencoded(t, c.size)

			mi, err := loadMetainfoFile(path)

			if c.tooLong {
				if !errors.Is(err, errMetainfoTooLarge) {
					t.Fatalf("error = %v, want errMetainfoTooLarge", err)
				}
				return
			}
			if err != nil || mi == nil || len(mi.Comment) == 0 {
				t.Fatalf("a file at the limit must load: %v", err)
			}
		})
	}

	t.Run("a missing file keeps its own error", func(t *testing.T) {
		_, err := loadMetainfoFile(filepath.Join(t.TempDir(), "nope.torrent"))
		if !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("error = %v, want os.ErrNotExist: the cache lookup tells absent from broken by it", err)
		}
	})

	t.Run("a file that is not bencode", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "x.torrent")
		if err := os.WriteFile(path, []byte("not a torrent"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := loadMetainfoFile(path); err == nil || errors.Is(err, errMetainfoTooLarge) {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestEveryPlaceThatReadsATorrentFileHonoursTheLimit(t *testing.T) {
	big := writeBencoded(t, maxMetainfoSize+1)

	t.Run("the cache of a stored download", func(t *testing.T) {
		m := newTestManager(t, 1)
		writeCacheBytes(t, m.store, "aaaa", bencodedComment(maxMetainfoSize+1))

		if _, err := m.store.loadMetainfo("aaaa"); !errors.Is(err, errMetainfoTooLarge) {
			t.Fatalf("error = %v, want errMetainfoTooLarge", err)
		}
	})

	t.Run("a torrent file chosen in the add dialog", func(t *testing.T) {
		m := managerWithClient(t, 1)

		_, err := m.FetchMetadata(big)

		if !errors.Is(err, errTorrentReadFailed) || !errors.Is(err, errMetainfoTooLarge) {
			t.Fatalf("error = %v, want errTorrentReadFailed caused by errMetainfoTooLarge", err)
		}
		if pending, reserved := m.bookkeeping(); pending != 0 || reserved != 0 {
			t.Fatalf("pending = %d, reserved = %d after a refusal", pending, reserved)
		}
	})

	t.Run("a torrent file named by a reuse request", func(t *testing.T) {
		m := managerWithClient(t, 1)

		_, err := m.InspectReuse(t.Context(), ReuseRequest{Source: big, Path: t.TempDir()}, nil)

		if !errors.Is(err, errMetainfoTooLarge) {
			t.Fatalf("error = %v, want errMetainfoTooLarge", err)
		}
	})

	t.Run("a torrent file named by a task", func(t *testing.T) {
		m := managerWithClient(t, 1)

		_, err := m.AddTask(t.Context(), AddRequest{Source: big, Destination: t.TempDir()})

		if !errors.Is(err, errMetainfoTooLarge) {
			t.Fatalf("error = %v, want errMetainfoTooLarge", err)
		}
	})
}

func TestACachedTorrentOverTheLimitIsReportedNotTreatedAsAbsent(t *testing.T) {
	m := newTestManager(t, 1)
	writeCacheBytes(t, m.store, "bbbb", bencodedComment(maxMetainfoSize+1))

	_, err := m.metainfoFor(t.Context(), nil, "", "bbbb")

	if !errors.Is(err, errMetainfoTooLarge) {
		t.Fatalf("error = %v: an unusable cache is not an absent one, and must not send the call on to the network", err)
	}
}
