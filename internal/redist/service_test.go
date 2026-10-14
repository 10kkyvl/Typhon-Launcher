package redist

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"

	"typhon/internal/library"
	"typhon/internal/uierr"
)

func startedService(t *testing.T, installDir string) *Service {
	t.Helper()
	s, err := NewService(func(id string) (library.Game, error) {
		if id != "g1" {
			return library.Game{}, errors.New("not found")
		}
		return library.Game{ID: id, InstallDir: installDir}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestNewServiceRejectsNilLookup(t *testing.T) {
	if _, err := NewService(nil); err == nil {
		t.Fatal("nil lookup must be an error")
	}
}

func TestServiceScanEmptyInstallDir(t *testing.T) {
	s := startedService(t, "")
	if _, err := s.Scan("g1"); uierr.Code(err) != "redist.no_install_dir" {
		t.Fatalf("err = %v", err)
	}
}

func TestServiceInstallRefusesOutsideAndUnrecognized(t *testing.T) {
	prev := run
	launched := 0
	run = func(context.Context, string, []string, string) (int, error) {
		launched++
		return 0, nil
	}
	t.Cleanup(func() { run = prev })

	root := t.TempDir()
	other := t.TempDir()
	inside := filepath.Join(root, "redist", "vcredist_x64.exe")
	plain := filepath.Join(root, "game.exe")
	outside := filepath.Join(other, "redist", "vcredist_x64.exe")
	for _, p := range []string{inside, plain, outside} {
		touch(t, p)
	}
	s := startedService(t, root)
	cases := []struct {
		name, path, code string
	}{
		{"outside install dir", outside, "redist.outside_install"},
		{"not a redist name", plain, "redist.outside_install"},
		{"dotdot escape", filepath.Join(root, "redist", "..", "..", filepath.Base(other), "redist", "vcredist_x64.exe"), "redist.outside_install"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := s.Install("g1", tc.path); uierr.Code(err) != tc.code {
				t.Fatalf("err = %v, want %s", err, tc.code)
			}
		})
	}
	if launched != 0 {
		t.Fatalf("installer launched %d times", launched)
	}
}

func TestServiceInstallRejectsConcurrentRun(t *testing.T) {
	entered := make(chan struct{})
	var once sync.Once
	release := make(chan struct{})
	prev := run
	run = func(context.Context, string, []string, string) (int, error) {
		once.Do(func() { close(entered) })
		<-release
		return 0, nil
	}
	t.Cleanup(func() { run = prev })

	root := t.TempDir()
	path := filepath.Join(root, "redist", "vcredist_x64.exe")
	touch(t, path)
	s := startedService(t, root)

	var wg sync.WaitGroup
	wg.Add(1)
	var firstErr error
	go func() {
		defer wg.Done()
		_, firstErr = s.Install("g1", path)
	}()
	<-entered
	if _, err := s.Install("g1", path); uierr.Code(err) != "redist.busy" {
		t.Fatalf("second install err = %v", err)
	}
	close(release)
	wg.Wait()
	if firstErr != nil {
		t.Fatal(firstErr)
	}
	if _, err := s.Install("g1", path); err != nil && uierr.Code(err) == "redist.busy" {
		t.Fatal("busy flag not released")
	}
}
