package relocate

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"typhon/internal/settings"
)

func holdLibraryPathWrite(t *testing.T, set *settings.Service, root string) (reached <-chan struct{}, release func()) {
	t.Helper()
	in := make(chan struct{})
	out := make(chan struct{})
	var once, free sync.Once
	release = func() { free.Do(func() { close(out) }) }
	t.Cleanup(release)
	if err := set.AddApplier(func(_, next settings.Settings) error {
		if next.LibraryPath == root {
			once.Do(func() {
				close(in)
				<-out
			})
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return in, release
}

func saveOtherSettingsWhile(t *testing.T, set *settings.Service) {
	t.Helper()
	other := set.GetSettings()
	other.Theme = "light"
	other.MaxActiveDownloads = 7
	if err := set.SaveSettings(other); err != nil {
		t.Fatalf("concurrent save: %v", err)
	}
}

func requireOtherSettingsKept(t *testing.T, set *settings.Service, wantRoot string) {
	t.Helper()
	got := set.GetSettings()
	if got.LibraryPath != wantRoot {
		t.Errorf("libraryPath = %q, want %q", got.LibraryPath, wantRoot)
	}
	if got.Theme != "light" || got.MaxActiveDownloads != 7 {
		t.Errorf("theme %q, downloads %d: the library path write put back a stale copy over a newer save", got.Theme, got.MaxActiveDownloads)
	}
}

func TestRecoverRepointKeepsASettingSavedWhileItWasInFlight(t *testing.T) {
	root := t.TempDir()
	oldRoot := filepath.Join(root, "old")
	newRoot := filepath.Join(root, "new")
	if err := os.MkdirAll(newRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	set := newTestSettings(t)
	if _, err := set.Update(func(next *settings.Settings) error {
		next.LibraryPath = oldRoot
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	reached, release := holdLibraryPathWrite(t, set, newRoot)

	job := Job{ID: "j1", Scope: ScopeLibrary, Stage: StageRepoint, GameID: itemSettings, Source: oldRoot, Target: newRoot}
	s := newRecoverTestService(t, job)
	s.settings = set

	done := make(chan struct{})
	go func() {
		defer close(done)
		s.recoverJob(context.Background(), job)
	}()
	<-reached
	saveOtherSettingsWhile(t, set)
	release()
	<-done

	requireOtherSettingsKept(t, set, newRoot)
	if got := s.List(); len(got) != 0 {
		t.Fatalf("job should complete, got %+v", got)
	}
}

func TestRecoverRepointDoesNotContinueWhenTheSettingsWriteFails(t *testing.T) {
	root := t.TempDir()
	oldRoot := filepath.Join(root, "old")
	newRoot := filepath.Join(root, "new")
	if err := os.MkdirAll(newRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	set, err := settings.NewServiceAt(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := set.Update(func(next *settings.Settings) error {
		next.LibraryPath = oldRoot
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(settingsPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(settingsPath, 0o700); err != nil {
		t.Fatal(err)
	}

	job := Job{ID: "j1", Scope: ScopeLibrary, Stage: StageRepoint, GameID: itemSettings, Source: oldRoot, Target: newRoot}
	s := newRecoverTestService(t, job)
	s.settings = set

	s.recoverJob(context.Background(), job)

	if got := s.soleJob(t); got.Stage != StageRepoint {
		t.Fatalf("stage = %s, want %s: the job moved on after a failed settings write", got.Stage, StageRepoint)
	}
	if got := set.GetSettings().LibraryPath; got != oldRoot {
		t.Fatalf("libraryPath = %q, want it unchanged at %q", got, oldRoot)
	}
}

func TestApplyLibrarySettingsKeepsASettingSavedWhileItWasInFlight(t *testing.T) {
	set := newTestSettings(t)
	s := newTestService(t, set, newTestLibrary(t), nil, nil, nil)
	oldRoot := set.GetSettings().LibraryPath
	newRoot := filepath.Join(t.TempDir(), settings.LibraryFolderName)
	if err := os.MkdirAll(newRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	job := Job{ID: "lib", Scope: ScopeLibrary, Stage: StagePrepare, GameID: itemSettings, Source: oldRoot, Target: newRoot}
	if err := s.registerJob(job); err != nil {
		t.Fatal(err)
	}
	reached, release := holdLibraryPathWrite(t, set, newRoot)

	done := make(chan error, 1)
	go func() { done <- s.applyLibrarySettings(job.ID, newRoot) }()
	<-reached
	saveOtherSettingsWhile(t, set)
	release()
	if err := <-done; err != nil {
		t.Fatalf("applyLibrarySettings: %v", err)
	}

	requireOtherSettingsKept(t, set, newRoot)
}
