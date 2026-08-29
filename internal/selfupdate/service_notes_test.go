package selfupdate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func newNotesService(t *testing.T, dir, currentVersion string) *Service {
	t.Helper()
	return &Service{
		dir:            dir,
		store:          mustStore(t, dir),
		notes:          mustNotesStore(t, dir),
		client:         mustQuietClient(t),
		currentVersion: currentVersion,
	}
}

func TestStoreReleaseNotesSeedsLastSeenVersion(t *testing.T) {
	dir := t.TempDir()
	s := newNotesService(t, dir, "1.0.0")

	if err := s.storeReleaseNotes([]ReleaseNote{testNote("1.0.0")}); err != nil {
		t.Fatalf("storeReleaseNotes() error = %v", err)
	}

	stored, err := s.notes.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if stored.LastSeenVersion != "1.0.0" {
		t.Fatalf("LastSeenVersion = %q, want the running version", stored.LastSeenVersion)
	}

	view, err := s.GetReleaseNotes()
	if err != nil {
		t.Fatalf("GetReleaseNotes() error = %v", err)
	}
	if len(view.Unseen) != 0 {
		t.Fatalf("Unseen = %v, want nothing on a fresh install", versionsOf(view.Unseen))
	}
}

// The notes of the version being offered are stored at check time, so the
// launcher can show them right after the restart without another round trip.
func TestReleaseNotesSurviveTheUpdate(t *testing.T) {
	dir := t.TempDir()
	before := newNotesService(t, dir, "1.0.0")
	if err := before.storeReleaseNotes([]ReleaseNote{testNote("1.1.0"), testNote("1.0.0")}); err != nil {
		t.Fatalf("storeReleaseNotes() error = %v", err)
	}

	after := newNotesService(t, dir, "1.1.0")
	loaded, err := after.notes.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	after.notesState = loaded

	view, err := after.GetReleaseNotes()
	if err != nil {
		t.Fatalf("GetReleaseNotes() error = %v", err)
	}
	if strings.Join(versionsOf(view.Unseen), ",") != "1.1.0" {
		t.Fatalf("Unseen = %v, want [1.1.0]", versionsOf(view.Unseen))
	}
	if view.CurrentVersion != "1.1.0" {
		t.Fatalf("CurrentVersion = %q, want 1.1.0", view.CurrentVersion)
	}
	if len(view.History) != 2 {
		t.Fatalf("History = %v, want both versions", versionsOf(view.History))
	}
}

func TestAcknowledgeReleaseNotesPersists(t *testing.T) {
	dir := t.TempDir()
	s := newNotesService(t, dir, "1.1.0")
	s.notesState = notesState{Releases: []ReleaseNote{testNote("1.1.0")}, LastSeenVersion: "1.0.0"}

	if err := s.AcknowledgeReleaseNotes(); err != nil {
		t.Fatalf("AcknowledgeReleaseNotes() error = %v", err)
	}

	stored, err := s.notes.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if stored.LastSeenVersion != "1.1.0" {
		t.Fatalf("LastSeenVersion on disk = %q, want 1.1.0", stored.LastSeenVersion)
	}

	view, err := s.GetReleaseNotes()
	if err != nil {
		t.Fatalf("GetReleaseNotes() error = %v", err)
	}
	if len(view.Unseen) != 0 {
		t.Fatalf("Unseen = %v, want nothing after acknowledging", versionsOf(view.Unseen))
	}
}

func TestAcknowledgeReleaseNotesKeepsMemoryOnFailedSave(t *testing.T) {
	dir := t.TempDir()
	s := newNotesService(t, dir, "1.1.0")
	s.notesState = notesState{Releases: []ReleaseNote{testNote("1.1.0")}, LastSeenVersion: "1.0.0"}
	s.notes.readOnly = true

	if err := s.AcknowledgeReleaseNotes(); !errors.Is(err, ErrReadOnly) {
		t.Fatalf("AcknowledgeReleaseNotes() error = %v, want ErrReadOnly", err)
	}
	if s.notesState.LastSeenVersion != "1.0.0" {
		t.Fatalf("LastSeenVersion = %q, want the value from before the failed save", s.notesState.LastSeenVersion)
	}

	view, err := s.GetReleaseNotes()
	if err != nil {
		t.Fatalf("GetReleaseNotes() error = %v", err)
	}
	if strings.Join(versionsOf(view.Unseen), ",") != "1.1.0" {
		t.Fatalf("Unseen = %v, want the notes to stay unseen", versionsOf(view.Unseen))
	}
}

func TestStoreReleaseNotesReportsAFailedSave(t *testing.T) {
	dir := t.TempDir()
	s := newNotesService(t, dir, "1.0.0")
	s.notes.readOnly = true

	if err := s.storeReleaseNotes([]ReleaseNote{testNote("1.1.0")}); !errors.Is(err, ErrReadOnly) {
		t.Fatalf("storeReleaseNotes() error = %v, want ErrReadOnly", err)
	}
	if len(s.notesState.Releases) != 0 {
		t.Fatalf("Releases = %v, want memory untouched after a failed save", versionsOf(s.notesState.Releases))
	}
}

func TestStoreReleaseNotesRejectsAnUnusableEntry(t *testing.T) {
	dir := t.TempDir()
	s := newNotesService(t, dir, "1.0.0")

	bad := []ReleaseNote{{Version: "1.1.0", Changes: []Change{{Kind: ChangeFixed, Text: "x"}}}}
	if err := s.storeReleaseNotes(bad); err == nil {
		t.Fatal("storeReleaseNotes() error = nil, want an error for a note without a date")
	}
	if _, err := os.Stat(filepath.Join(dir, "selfupdate", notesName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stat notes file = %v, want the file never written", err)
	}
}

// A release-notes file this launcher cannot read costs the changelog, not the
// ability to update: startup keeps going and every later save says why.
func TestServiceStartupSurvivesBrokenReleaseNotes(t *testing.T) {
	dir := t.TempDir()
	cache, err := CacheDir(dir)
	if err != nil {
		t.Fatalf("CacheDir: %v", err)
	}
	writeTestFile(t, filepath.Join(cache, notesName), []byte(`{"version":1,"data":{`))

	s := newNotesService(t, dir, "1.0.0")
	if err := s.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatalf("ServiceStartup() error = %v, want the launcher to start anyway", err)
	}
	t.Cleanup(func() {
		if err := s.ServiceShutdown(); err != nil {
			t.Fatalf("ServiceShutdown() error = %v", err)
		}
	})

	view, err := s.GetReleaseNotes()
	if err != nil {
		t.Fatalf("GetReleaseNotes() error = %v", err)
	}
	if len(view.History) != 0 {
		t.Fatalf("History = %v, want nothing from a broken file", versionsOf(view.History))
	}
	if err := s.AcknowledgeReleaseNotes(); !errors.Is(err, ErrReadOnly) {
		t.Fatalf("AcknowledgeReleaseNotes() error = %v, want ErrReadOnly", err)
	}
}

func TestCleanupCacheKeepsReleaseNotes(t *testing.T) {
	dir := t.TempDir()
	cache, err := CacheDir(dir)
	if err != nil {
		t.Fatalf("CacheDir: %v", err)
	}
	notesPath := filepath.Join(cache, notesName)
	writeTestFile(t, notesPath, []byte(`{"version":1,"data":{"lastSeenVersion":"1.0.0"}}`))
	writeTestFile(t, filepath.Join(cache, "leftover.tmp"), []byte("junk"))

	s := newNotesService(t, dir, "1.0.0")
	if err := s.cleanupCache(context.Background(), ""); err != nil {
		t.Fatalf("cleanupCache() error = %v", err)
	}

	if _, err := os.Stat(notesPath); err != nil {
		t.Fatalf("stat release notes = %v, want the file kept", err)
	}
	if _, err := os.Stat(filepath.Join(cache, "leftover.tmp")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stat leftover = %v, want it removed", err)
	}
}
