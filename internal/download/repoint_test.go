package download

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func (m *Manager) addRepointItem(id, destination string, status Status, eng *fakeTorrent) *Download {
	d := &Download{
		ID:          id,
		Name:        id,
		Type:        TypeTorrent,
		InfoHash:    id,
		Destination: destination,
		Status:      status,
		Total:       100,
		ETASeconds:  -1,
		AddedAt:     time.Now(),
	}
	m.mu.Lock()
	m.items = append(m.items, d)
	if eng != nil {
		m.engines[id] = eng
	}
	m.persistLocked()
	m.mu.Unlock()
	return d
}

func TestRepointRewritesDestinations(t *testing.T) {
	root := t.TempDir()
	oldRoot := filepath.Join(root, "old")
	newRoot := filepath.Join(root, "new")
	if err := os.MkdirAll(filepath.Join(oldRoot, "GameA"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(oldRoot, "GameA", "file.bin"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	m := mustManagerAt(t, t.TempDir())
	eng := &fakeTorrent{size: 100}
	m.addRepointItem("a", filepath.Join(oldRoot, "GameA"), StatusPaused, eng)
	m.addRepointItem("b", filepath.Join(root, "elsewhere"), StatusCompleted, nil)

	if err := m.Repoint(context.Background(), oldRoot, newRoot); err != nil {
		t.Fatalf("repoint: %v", err)
	}
	// Repoint hands seeding restoration to a wg-tracked background restore()
	// goroutine (invariant 19); wait for it before a second Manager reads
	// the same directory, or their concurrent writes race on Windows.
	m.wg.Wait()

	got, err := m.Get("a")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(newRoot, "GameA")
	if got.Destination != want {
		t.Fatalf("destination = %q, want %q", got.Destination, want)
	}
	unrelated, err := m.Get("b")
	if err != nil {
		t.Fatal(err)
	}
	if unrelated.Destination != filepath.Join(root, "elsewhere") {
		t.Fatalf("unrelated destination changed: %q", unrelated.Destination)
	}
	if !eng.wasDropped() {
		t.Fatal("engine for paused download was not dropped before the move")
	}
	if _, err := os.Stat(filepath.Join(newRoot, "GameA", "file.bin")); err != nil {
		t.Fatalf("moved file missing: %v", err)
	}
	if _, err := os.Stat(oldRoot); !os.IsNotExist(err) {
		t.Fatalf("old root still present: %v", err)
	}

	reloaded, err := newManagerAt(m.store.dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	withTestContext(t, reloaded)
	if err := reloaded.loadLocked(); err != nil {
		t.Fatal(err)
	}
	persisted, err := reloaded.Get("a")
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Destination != want {
		t.Fatalf("persisted destination = %q, want %q", persisted.Destination, want)
	}
}

func TestRepointRefusesWithActiveJob(t *testing.T) {
	root := t.TempDir()
	oldRoot := filepath.Join(root, "old")
	newRoot := filepath.Join(root, "new")

	m := mustManagerAt(t, t.TempDir())
	m.addRepointItem("a", filepath.Join(oldRoot, "GameA"), StatusDownloading, nil)

	err := m.Repoint(context.Background(), oldRoot, newRoot)
	if err == nil {
		t.Fatal("expected repoint to refuse while a download is active")
	}

	got, gerr := m.Get("a")
	if gerr != nil {
		t.Fatal(gerr)
	}
	if got.Destination != filepath.Join(oldRoot, "GameA") {
		t.Fatalf("destination changed despite refusal: %q", got.Destination)
	}
}

func TestRepointRejectsEmptyAndNestedPaths(t *testing.T) {
	m := mustManagerAt(t, t.TempDir())
	root := t.TempDir()

	cases := []struct {
		name string
		old  string
		new  string
	}{
		{"empty old", "", filepath.Join(root, "new")},
		{"empty new", filepath.Join(root, "old"), ""},
		{"same path", filepath.Join(root, "same"), filepath.Join(root, "same")},
		{"new inside old", filepath.Join(root, "old"), filepath.Join(root, "old", "new")},
		{"old inside new", filepath.Join(root, "new", "old"), filepath.Join(root, "new")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := m.Repoint(context.Background(), tc.old, tc.new); err == nil {
				t.Fatalf("%s: expected error", tc.name)
			}
		})
	}
}

func TestRepointNoopWhenSourceMissing(t *testing.T) {
	root := t.TempDir()
	oldRoot := filepath.Join(root, "old")
	newRoot := filepath.Join(root, "new")

	m := mustManagerAt(t, t.TempDir())
	m.addRepointItem("a", filepath.Join(root, "elsewhere"), StatusCompleted, nil)

	if err := m.Repoint(context.Background(), oldRoot, newRoot); err != nil {
		t.Fatalf("repoint with nothing to move: %v", err)
	}
	m.wg.Wait()
}

// Повторный вход после аварии между удавшимся переименованием и неудавшимся
// удалением старой папки: до фикса код уходил на второй круг копирования и
// навсегда упирался в занятый путь назначения.
func TestMoveTreeRetryAfterRenameDoesNotCopyAgain(t *testing.T) {
	root := t.TempDir()
	oldRoot := filepath.Join(root, "old")
	newRoot := filepath.Join(root, "new")
	if err := os.MkdirAll(oldRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(oldRoot, "leftover.bin"), []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(newRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(newRoot, "moved.bin"), []byte("payload"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := moveTreeIfPresent(context.Background(), oldRoot, newRoot); err != nil {
		t.Fatalf("moveTreeIfPresent: %v", err)
	}
	if _, err := os.Stat(oldRoot); !os.IsNotExist(err) {
		t.Fatalf("old root still present: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(newRoot, "moved.bin"))
	if err != nil {
		t.Fatalf("read moved file: %v", err)
	}
	if string(got) != "payload" {
		t.Fatalf("moved file = %q, want %q", got, "payload")
	}
	if _, err := os.Stat(filepath.Join(newRoot, "leftover.bin")); err == nil {
		t.Fatal("stale file from the old root was copied over the moved data")
	}
	if _, err := os.Stat(newRoot + ".repoint-staging"); err == nil {
		t.Fatal("staging directory created: the tree was copied a second time")
	}
}

// Пустой каталог назначения создаётся при настройке библиотеки, и os.Rename
// на занятый путь на Windows падает: без его очистки обычный первый перенос
// уходил бы в копирование вместо мгновенного переименования.
func TestMoveTreeClearsEmptyDestination(t *testing.T) {
	root := t.TempDir()
	oldRoot := filepath.Join(root, "old")
	newRoot := filepath.Join(root, "new")
	if err := os.MkdirAll(oldRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(oldRoot, "file.bin"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(newRoot, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := moveTreeIfPresent(context.Background(), oldRoot, newRoot); err != nil {
		t.Fatalf("moveTreeIfPresent: %v", err)
	}
	if _, err := os.Stat(oldRoot); !os.IsNotExist(err) {
		t.Fatalf("old root still present: %v", err)
	}
	if _, err := os.Stat(filepath.Join(newRoot, "file.bin")); err != nil {
		t.Fatalf("moved file missing: %v", err)
	}
	if _, err := os.Stat(newRoot + ".repoint-staging"); err == nil {
		t.Fatal("staging directory created: rename path was not taken")
	}
}
