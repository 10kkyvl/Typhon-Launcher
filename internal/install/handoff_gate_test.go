package install

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"typhon/internal/download"
	"typhon/internal/handoffgate"
	"typhon/internal/storage"

	"github.com/wailsapp/wails/v3/pkg/application"
)

const heldBackInstallations = `[{"id":"a","name":"Game","status":"installed"}]`

func writeHeldBackInstallations(t *testing.T) (dir, path string) {
	t.Helper()
	dir = t.TempDir()
	path = filepath.Join(dir, "installations.json")
	if err := storage.WriteAtomic(path, []byte(heldBackInstallations)); err != nil {
		t.Fatal(err)
	}
	return dir, path
}

func assertInstallationsUntouched(t *testing.T, path string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != heldBackInstallations {
		t.Fatalf("installations.json = %s\nwant it untouched", got)
	}
}

func TestHeldBackInstallShutdownKeepsTheList(t *testing.T) {
	dir, path := writeHeldBackInstallations(t)
	s := mustServiceAt(t, dir)
	handoffgate.Arm()
	t.Cleanup(handoffgate.Open)
	ctx, cancel := context.WithCancel(t.Context())
	if err := s.ServiceStartup(ctx, application.ServiceOptions{}); err != nil {
		t.Fatal(err)
	}
	cancel()
	if err := s.ServiceShutdown(); err != nil {
		t.Fatalf("ServiceShutdown: %v", err)
	}
	assertInstallationsUntouched(t, path)
}

func TestHeldBackInstallRefusesChanges(t *testing.T) {
	dir, path := writeHeldBackInstallations(t)
	s := mustServiceAt(t, dir)
	handoffgate.Arm()
	t.Cleanup(handoffgate.Open)
	if err := s.ServiceStartup(t.Context(), application.ServiceOptions{}); err != nil {
		t.Fatal(err)
	}
	calls := map[string]func() error{
		"start": func() error {
			_, err := s.Start("d1", StartOptions{Destination: t.TempDir()})
			return err
		},
		"cancel":  func() error { return s.Cancel("a") },
		"retry":   func() error { return s.Retry("a") },
		"dismiss": func() error { return s.Dismiss("a") },
		"remove":  func() error { return s.RemoveGame("g1", RemoveOptions{}) },
		"gone":    func() error { return s.HandleDownloadGone("d1") },
	}
	for name, call := range calls {
		if err := call(); err == nil && name != "gone" {
			t.Errorf("%s on a held-back service = nil, want a refusal", name)
		}
	}
	s.HandleDownloadCompleted(download.Download{ID: "d1", Status: download.StatusCompleted, Origin: download.Origin{Purpose: download.PurposeRelease}})
	assertInstallationsUntouched(t, path)
}

func TestUnstartedInstallShutdownKeepsTheList(t *testing.T) {
	dir, path := writeHeldBackInstallations(t)
	s := mustServiceAt(t, dir)
	if err := s.ServiceShutdown(); err != nil {
		t.Fatalf("ServiceShutdown: %v", err)
	}
	assertInstallationsUntouched(t, path)
}
