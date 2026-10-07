package download

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

var layoutFixture = []string{
	"victim/secret.txt",
	"victim.txt",
	"dest/Game/data.bin",
	"dest/Game/sub/more.bin",
	"dest/Game.part",
	"dest/Keep/keep.bin",
	"dest/loose.txt",
	"dest/sub/inner.bin",
}

func writeLayout(t *testing.T) (parent, dest string) {
	t.Helper()
	parent = t.TempDir()
	for _, rel := range layoutFixture {
		full := filepath.Join(parent, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(rel), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return parent, filepath.Join(parent, "dest")
}

func survivors(t *testing.T, parent string) []string {
	t.Helper()
	var left []string
	for _, rel := range layoutFixture {
		_, err := os.Stat(filepath.Join(parent, filepath.FromSlash(rel)))
		switch {
		case err == nil:
			left = append(left, rel)
		case errors.Is(err, os.ErrNotExist):
		default:
			t.Fatalf("stat %s: %v", rel, err)
		}
	}
	return left
}

func without(gone ...string) []string {
	var out []string
	for _, rel := range layoutFixture {
		if !slices.Contains(gone, rel) {
			out = append(out, rel)
		}
	}
	return out
}

func TestRemoveContentOnlyTouchesWhatTheNameDescribes(t *testing.T) {
	cases := []struct {
		name string
		dest string
		item string
		gone []string
	}{
		{"a torrent folder and its part file", "dest", "Game", []string{"dest/Game/data.bin", "dest/Game/sub/more.bin", "dest/Game.part"}},
		{"a nested name", "dest", "sub", []string{"dest/sub/inner.bin"}},
		{"a name with nothing on disk", "dest", "Missing", nil},
		{"parent traversal", "dest", "../victim", nil},
		{"parent traversal inside the name", "dest", "Game/../../victim", nil},
		{"a bare parent", "dest", "..", nil},
		{"the current directory", "dest", ".", nil},
		{"an empty name", "dest", "", nil},
		{"a blank name", "dest", "   ", nil},
		{"an absolute name", "dest", "/victim", nil},
		{"a drive letter", "dest", `C:\victim`, nil},
		{"a backslash path", "dest", `Game\..\..\victim`, nil},
		{"an empty component", "dest", "Game//sub", nil},
		{"an empty destination", "", "Game", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			parent, dest := writeLayout(t)
			if c.dest == "" {
				t.Chdir(parent)
				dest = ""
				if err := os.MkdirAll(filepath.Join(parent, "Game"), 0o755); err != nil {
					t.Fatal(err)
				}
				defer func() {
					if _, err := os.Stat(filepath.Join(parent, "Game")); err != nil {
						t.Errorf("a path relative to the working directory was deleted: %v", err)
					}
				}()
			}

			removeContent(dest, c.item)

			if got, want := survivors(t, parent), without(c.gone...); !slices.Equal(got, want) {
				t.Fatalf("left = %v, want %v", got, want)
			}
		})
	}
}

func TestRemovalByLayout(t *testing.T) {
	unsafeFlat := []FileState{
		{Path: "../victim.txt"}, {Path: `..\victim.txt`}, {Path: "sub/../../victim.txt"},
		{Path: "/victim.txt"}, {Path: "../victim/secret.txt"}, {Path: "loose.txt"},
	}
	cases := []struct {
		name       string
		status     Status
		itemName   string
		flat       bool
		inPlace    bool
		files      []FileState
		op         func(m *Manager) error
		wantErr    error
		gone       []string
		wantRecord bool
	}{
		{"cancel an unfinished download", StatusDownloading, "Game", false, false, nil,
			func(m *Manager) error { return m.Cancel("a") }, nil,
			[]string{"dest/Game/data.bin", "dest/Game/sub/more.bin", "dest/Game.part"}, false},
		{"cancel an unfinished paused download", StatusPaused, "Game", false, false, nil,
			func(m *Manager) error { return m.Cancel("a") }, nil,
			[]string{"dest/Game/data.bin", "dest/Game/sub/more.bin", "dest/Game.part"}, false},
		{"cancel a repair that works in place", StatusDownloading, "Game", false, true, nil,
			func(m *Manager) error { return m.Cancel("a") }, nil, nil, false},
		{"cancel a finished download keeps its files", StatusCompleted, "Game", false, false, nil,
			func(m *Manager) error { return m.Cancel("a") }, nil, nil, false},
		{"remove an unfinished download keeps its files", StatusDownloading, "Game", false, false, nil,
			func(m *Manager) error { return m.Remove("a") }, nil, nil, false},
		{"remove a finished download keeps its files", StatusCompleted, "Game", false, false, nil,
			func(m *Manager) error { return m.Remove("a") }, nil, nil, false},
		{"delete the data of a finished download", StatusCompleted, "Game", false, false, nil,
			func(m *Manager) error { return m.DeleteData("a") }, nil,
			[]string{"dest/Game/data.bin", "dest/Game/sub/more.bin", "dest/Game.part"}, false},
		{"delete the data of an in-place download is refused", StatusCompleted, "Game", false, true, nil,
			func(m *Manager) error { return m.DeleteData("a") }, errUnavailable, nil, true},
		{"delete the data of a flat download takes only its own files", StatusCompleted, "Game", true, false,
			[]FileState{{Path: "loose.txt"}, {Path: "sub/inner.bin"}},
			func(m *Manager) error { return m.DeleteData("a") }, nil,
			[]string{"dest/loose.txt", "dest/sub/inner.bin"}, false},
		{"delete the data of a flat download never leaves the destination", StatusCompleted, "Game", true, false, unsafeFlat,
			func(m *Manager) error { return m.DeleteData("a") }, nil, []string{"dest/loose.txt"}, false},
		{"delete the data of a flat download without a file list is refused", StatusCompleted, "Game", true, false, nil,
			func(m *Manager) error { return m.DeleteData("a") }, errUnavailable, nil, true},
		{"a stored name that climbs out of the destination", StatusDownloading, "../victim", false, false, nil,
			func(m *Manager) error { return m.Cancel("a") }, nil, nil, false},
		{"a stored name with an embedded parent step", StatusDownloading, "Game/../../victim", false, false, nil,
			func(m *Manager) error { return m.Cancel("a") }, nil, nil, false},
		{"delete the data under a stored name that climbs out", StatusCompleted, "../victim", false, false, nil,
			func(m *Manager) error { return m.DeleteData("a") }, nil, nil, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			parent, dest := writeLayout(t)
			m := newTestManager(t, 1)
			m.addTestItem("a", c.status)
			eng := attachFake(m, "a")
			setItem(m, "a", func(d *Download) {
				d.Destination = dest
				d.Name = c.itemName
				d.Flat = c.flat
				d.InPlace = c.inPlace
				d.Files = c.files
			})

			err := c.op(m)
			m.wg.Wait()

			if c.wantErr == nil && err != nil {
				t.Fatal(err)
			}
			if c.wantErr != nil && !errors.Is(err, c.wantErr) {
				t.Fatalf("error = %v, want %v", err, c.wantErr)
			}
			_, getErr := m.Get("a")
			if tracked := getErr == nil; tracked != c.wantRecord {
				t.Fatalf("record tracked = %v, want %v", tracked, c.wantRecord)
			}
			if got, want := survivors(t, parent), without(c.gone...); !slices.Equal(got, want) {
				t.Fatalf("left on disk = %v, want %v", got, want)
			}
			if c.wantRecord == eng.wasDropped() {
				t.Fatalf("engine dropped = %v with the record tracked = %v", eng.wasDropped(), c.wantRecord)
			}
		})
	}
}

func TestCancelDeletesNothingWhileTheDownloadsJobStillRuns(t *testing.T) {
	parent, dest := writeLayout(t)
	m := newTestManager(t, 1)
	m.addTestItem("a", StatusVerifying)
	eng := attachFake(m, "a")
	setItem(m, "a", func(d *Download) { d.Destination, d.Name = dest, "Game" })
	job := &jobState{cancel: func() {}, done: make(chan struct{})}
	m.mu.Lock()
	m.jobs["a"] = job
	m.mu.Unlock()

	if err := m.Cancel("a"); err != nil {
		t.Fatal(err)
	}
	m.mu.Lock()
	torndown := m.teardowns["a"]
	m.mu.Unlock()
	if torndown == nil {
		t.Fatal("the teardown is not tracked")
	}
	select {
	case <-torndown:
		t.Fatal("the teardown finished while the job of the download was still running")
	case <-time.After(50 * time.Millisecond):
	}
	if got := survivors(t, parent); !slices.Equal(got, layoutFixture) {
		t.Fatalf("files were deleted under a running job: left %v", got)
	}
	if eng.wasDropped() {
		t.Fatal("the engine was dropped under a running job")
	}

	close(job.done)
	<-torndown
	m.wg.Wait()
	if got, want := survivors(t, parent), without("dest/Game/data.bin", "dest/Game/sub/more.bin", "dest/Game.part"); !slices.Equal(got, want) {
		t.Fatalf("left after the job ended = %v, want %v", got, want)
	}
	if !eng.wasDropped() {
		t.Fatal("the engine was not dropped once the job ended")
	}
}

func TestRemovalThatCannotBePersistedDeletesNoFiles(t *testing.T) {
	cases := []struct {
		name   string
		status Status
		op     func(m *Manager) error
	}{
		{"cancel", StatusDownloading, func(m *Manager) error { return m.Cancel("a") }},
		{"delete data", StatusCompleted, func(m *Manager) error { return m.DeleteData("a") }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			parent, dest := writeLayout(t)
			m := newTestManager(t, 1)
			m.addTestItem("a", c.status)
			eng := attachFake(m, "a")
			setItem(m, "a", func(d *Download) { d.Destination, d.Name = dest, "Game" })
			gone := make(chan string, 1)
			m.SetOnGone(func(id string) { gone <- id })
			breakStore(t, m)

			err := c.op(m)
			m.wg.Wait()

			if err == nil {
				t.Fatal("the persist failure was swallowed")
			}
			if got := survivors(t, parent); !slices.Equal(got, layoutFixture) {
				t.Fatalf("files were deleted although the record could not be removed: left %v", got)
			}
			if eng.wasDropped() {
				t.Fatal("the engine was dropped although the record could not be removed")
			}
			if len(gone) != 0 {
				t.Fatal("onGone fired for a download that is still tracked")
			}
			if _, err := m.Get("a"); err != nil {
				t.Fatalf("download lost: %v", err)
			}
		})
	}
}
