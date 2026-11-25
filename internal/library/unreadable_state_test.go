package library

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"typhon/internal/handoffgate"
	"typhon/internal/storage"
	"typhon/internal/uierr"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// unreadableSuccessor is a successor whose library file could not be read
// again when its old launcher had gone: every save must be refused with a
// code the interface can name. Game a is installed in installDir.
func unreadableSuccessor(t *testing.T) (s *Service, path, installDir string) {
	t.Helper()
	installDir = t.TempDir()
	seed, err := json.Marshal([]Game{{ID: "a", Title: "A", InstallDir: installDir}, {ID: "b", Title: "B"}})
	if err != nil {
		t.Fatal(err)
	}
	path = filepath.Join(t.TempDir(), "library.json")
	if err := storage.WriteAtomic(path, seed); err != nil {
		t.Fatal(err)
	}
	handoffgate.Arm()
	t.Cleanup(handoffgate.Open)
	s = mustServiceAt(t, path)
	if err := s.ServiceStartup(t.Context(), application.ServiceOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := storage.WriteAtomic(path, []byte(`[{"id":"a",`)); err != nil {
		t.Fatal(err)
	}
	if err := s.ReloadBeforeGateOpens(); err == nil {
		t.Fatal("setup: the re-read of a broken file succeeded")
	}
	openGate(t)
	return s, path, installDir
}

func TestASaveOfTheGamesAfterAFailedReReadHasTheUnreadableCode(t *testing.T) {
	s, _, _ := unreadableSuccessor(t)
	_, err := s.SetFavorite("b", true)
	if code := uierr.Code(err); code != "library.state_unreadable" {
		t.Fatalf("SetFavorite() = %v (code %q), want library.state_unreadable", err, code)
	}
}

func TestASaveOfTheCollectionsAfterAFailedReReadHasTheUnreadableCode(t *testing.T) {
	s, path, _ := unreadableSuccessor(t)
	_, err := s.SaveCollection(Collection{Name: "Later"})
	if code := uierr.Code(err); code != "library.state_unreadable" {
		t.Fatalf("SaveCollection() = %v (code %q), want library.state_unreadable", err, code)
	}
	if _, serr := os.Stat(collectionsPathFor(path)); !os.IsNotExist(serr) {
		t.Fatalf("collections file written after a failed re-read: %v", serr)
	}
	if got := s.GetCollections(); len(got) != 0 {
		t.Fatalf("the refused collection stayed in memory: %+v", got)
	}
}

func TestASaveOfTheExcludedFoldersAfterAFailedReReadHasTheUnreadableCode(t *testing.T) {
	s, path, _ := unreadableSuccessor(t)
	err := s.RemoveGame("a")
	if code := uierr.Code(err); code != "library.state_unreadable" {
		t.Fatalf("RemoveGame() = %v (code %q), want library.state_unreadable", err, code)
	}
	excluded, perr := excludedPathFor(path)
	if perr != nil {
		t.Fatal(perr)
	}
	if _, serr := os.Stat(excluded); !os.IsNotExist(serr) {
		t.Fatalf("excluded file written after a failed re-read: %v", serr)
	}
}
