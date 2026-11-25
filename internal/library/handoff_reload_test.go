package library

import (
	"os"
	"path/filepath"
	"testing"

	"typhon/internal/handoffgate"
	"typhon/internal/storage"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func seedLibrary(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "library.json")
	if err := storage.WriteAtomic(path, []byte(`[{"id":"a","title":"A"},{"id":"b","title":"B"}]`)); err != nil {
		t.Fatal(err)
	}
	return path
}

func favourite(t *testing.T, s *Service, id string) bool {
	t.Helper()
	for _, g := range s.GetGames() {
		if g.ID == id {
			return g.Favorite
		}
	}
	t.Fatalf("game %s is not in the library", id)
	return false
}

// openGate lets the held-back starts run and waits for them.
func openGate(t *testing.T) {
	t.Helper()
	handoffgate.Open()
	if err := handoffgate.Settled(t.Context()); err != nil {
		t.Fatal(err)
	}
}

// The successor builds its library while the old launcher still runs; the
// old one then writes (a session ends, an install finishes). The successor
// reads the file again when its gate opens, so its first save keeps that
// write.
func TestSuccessorLibraryKeepsAWriteTheOldLauncherMadeInTheOverlap(t *testing.T) {
	path := seedLibrary(t)
	old := mustServiceAt(t, path)
	handoffgate.Arm()
	t.Cleanup(handoffgate.Open)
	succ := mustServiceAt(t, path)
	if err := succ.ServiceStartup(t.Context(), application.ServiceOptions{}); err != nil {
		t.Fatal(err)
	}

	if _, err := old.SetFavorite("a", true); err != nil {
		t.Fatal(err)
	}
	openGate(t)
	if !favourite(t, succ, "a") {
		t.Fatal("the successor does not see the old launcher's write after its gate opened")
	}
	if _, err := succ.SetFavorite("b", true); err != nil {
		t.Fatal(err)
	}

	again := mustServiceAt(t, path)
	if !favourite(t, again, "a") || !favourite(t, again, "b") {
		t.Fatal("the successor's save lost the old launcher's write, or its own")
	}
}

// A file that cannot be read when the gate opens is not taken for an empty
// library: the successor keeps what it read at construction and refuses to
// save over the file, as a failed load at construction would have.
func TestSuccessorLibraryUnreadableAtItsStartRefusesToSave(t *testing.T) {
	path := seedLibrary(t)
	handoffgate.Arm()
	t.Cleanup(handoffgate.Open)
	succ := mustServiceAt(t, path)
	if err := succ.ServiceStartup(t.Context(), application.ServiceOptions{}); err != nil {
		t.Fatal(err)
	}
	broken := []byte(`[{"id":"a",`)
	if err := storage.WriteAtomic(path, broken); err != nil {
		t.Fatal(err)
	}
	openGate(t)

	if got := len(succ.GetGames()); got != 2 {
		t.Fatalf("the successor holds %d games, want the 2 it read at construction", got)
	}
	if _, err := succ.SetFavorite("b", true); err == nil {
		t.Fatal("SetFavorite() saved over a library file that could not be read")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != string(broken) {
		t.Fatalf("library.json = %s, want it untouched", raw)
	}
	if favourite(t, succ, "b") {
		t.Fatal("the refused change stayed in memory")
	}
}

// The gate releases every held-back start at once. One of them (a move's
// recovery, an install finalized after a crash) may save the library as soon
// as it runs; the re-read that runs before the gate opens means it saves what
// the old launcher left, and the library's own start does not then swap an
// older picture over that save.
func TestAHeldBackStartThatSavesAtOnceKeepsTheOldLaunchersWrite(t *testing.T) {
	path := seedLibrary(t)
	old := mustServiceAt(t, path)
	handoffgate.Arm()
	t.Cleanup(handoffgate.Open)
	succ := mustServiceAt(t, path)
	if err := succ.ServiceStartup(t.Context(), application.ServiceOptions{}); err != nil {
		t.Fatal(err)
	}
	saved := make(chan error, 1)
	if err := handoffgate.Startup(t.Context(), "test-saves-at-once", func() error {
		_, err := succ.SetFavorite("b", true)
		saved <- err
		return err
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := old.SetFavorite("a", true); err != nil {
		t.Fatal(err)
	}
	if err := succ.ReloadBeforeGateOpens(); err != nil {
		t.Fatalf("ReloadBeforeGateOpens() = %v", err)
	}
	openGate(t)
	if err := <-saved; err != nil {
		t.Fatalf("the held-back save failed: %v", err)
	}

	if a, b := favourite(t, succ, "a"), favourite(t, succ, "b"); !a || !b {
		t.Fatalf("the successor holds favourite a=%v b=%v, want both the old launcher's write and its own save", a, b)
	}
	again := mustServiceAt(t, path)
	if !favourite(t, again, "a") || !favourite(t, again, "b") {
		t.Fatal("library.json lost the old launcher's write or the held-back save")
	}
}

// After the re-read before the gate, the library's own start does not read
// the file a second time: what it holds is what that re-read found.
func TestTheLibraryStartDoesNotReadAgainAfterTheReReadBeforeTheGate(t *testing.T) {
	path := seedLibrary(t)
	handoffgate.Arm()
	t.Cleanup(handoffgate.Open)
	succ := mustServiceAt(t, path)
	if err := succ.ServiceStartup(t.Context(), application.ServiceOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := succ.ReloadBeforeGateOpens(); err != nil {
		t.Fatal(err)
	}
	if err := storage.WriteAtomic(path, []byte(`[{"id":"a","title":"A","favorite":true}]`)); err != nil {
		t.Fatal(err)
	}
	openGate(t)

	if got, a := len(succ.GetGames()), favourite(t, succ, "a"); got != 2 || a {
		t.Fatalf("the successor holds %d games, favourite a=%v: the library's start read the file a second time", got, a)
	}
}

// A re-read before the gate that fails does not keep the launcher from coming
// up: the gate still opens, the library keeps what it read at construction and
// refuses to save over the file it could not read.
func TestAFailedReReadBeforeTheGateRefusesSaves(t *testing.T) {
	path := seedLibrary(t)
	handoffgate.Arm()
	t.Cleanup(handoffgate.Open)
	succ := mustServiceAt(t, path)
	if err := succ.ServiceStartup(t.Context(), application.ServiceOptions{}); err != nil {
		t.Fatal(err)
	}
	broken := []byte(`[{"id":"a",`)
	if err := storage.WriteAtomic(path, broken); err != nil {
		t.Fatal(err)
	}
	if err := succ.ReloadBeforeGateOpens(); err == nil {
		t.Fatal("ReloadBeforeGateOpens() = nil for a file that cannot be read")
	}
	openGate(t)

	if got := len(succ.GetGames()); got != 2 {
		t.Fatalf("the successor holds %d games, want the 2 it read at construction", got)
	}
	if _, err := succ.SetFavorite("b", true); err == nil {
		t.Fatal("SetFavorite() saved over a library file that could not be read")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != string(broken) {
		t.Fatalf("library.json = %s, want it untouched", raw)
	}
}

// An ordinary start reads the file once, at construction: its start does not
// read it again.
func TestOrdinaryLibraryStartDoesNotReadTheFileAgain(t *testing.T) {
	path := seedLibrary(t)
	s := mustServiceAt(t, path)
	if err := storage.WriteAtomic(path, []byte(`[{"id":"a","title":"A","favorite":true}]`)); err != nil {
		t.Fatal(err)
	}
	if err := s.ServiceStartup(t.Context(), application.ServiceOptions{}); err != nil {
		t.Fatal(err)
	}
	if got := len(s.GetGames()); got != 2 || favourite(t, s, "a") {
		t.Fatalf("an ordinary start read the file again: %+v", s.GetGames())
	}
}
