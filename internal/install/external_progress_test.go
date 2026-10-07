package install

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func writeSized(t *testing.T, path string, size int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Errorf("mkdir for %s: %v", path, err)
		return
	}
	if err := os.WriteFile(path, make([]byte, size), 0o644); err != nil {
		t.Errorf("write %s: %v", path, err)
	}
}

func TestExternalInstallerReportsBytesWithoutTotal(t *testing.T) {
	s, downloads, _ := newTestService(t)
	old := installPollInterval
	installPollInterval = time.Millisecond
	t.Cleanup(func() { installPollInterval = old })

	root := t.TempDir()
	innoSource(t, root)
	downloads.add("d1", "Game", root)

	games := t.TempDir()
	dest := filepath.Join(games, "Game")
	s.roots = []string{games}

	firstWritten := make(chan struct{})
	goOn := make(chan struct{})
	secondWritten := make(chan struct{})
	finish := make(chan struct{})
	openGoOn := sync.OnceFunc(func() { close(goOn) })
	openFinish := sync.OnceFunc(func() { close(finish) })
	// A failed assertion must not leave the runner blocked: shutdown waits for it.
	t.Cleanup(func() {
		openGoOn()
		openFinish()
	})
	s.runner = &fakeRunner{act: func(runSpec) {
		writeSized(t, filepath.Join(dest, "Game.exe"), 8192)
		close(firstWritten)
		<-goOn
		writeSized(t, filepath.Join(dest, "data", "content.pak"), 4096)
		close(secondWritten)
		<-finish
	}}

	item, err := s.Start("d1", StartOptions{Destination: dest})
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	for _, step := range []struct {
		written <-chan struct{}
		bytes   int64
	}{{firstWritten, 8192}, {secondWritten, 8192 + 4096}} {
		<-step.written
		var seen Installation
		waitFor(t, "recorded install size", func() bool {
			seen, _ = s.snapshot(item.ID)
			return seen.Status == StatusInstalling && seen.BytesDone == step.bytes
		})
		if seen.BytesTotal != 0 || seen.Progress != 0 {
			t.Fatalf("installed size is unknown: bytesTotal = %d, progress = %v, want 0 and 0 at %d bytes",
				seen.BytesTotal, seen.Progress, step.bytes)
		}
		if step.bytes == 8192 {
			openGoOn()
		}
	}
	openFinish()
	s.waitStatus(t, item.ID, StatusCompleted)
	s.waitJobDone(t, item.ID)
}

func TestExternalInstallerKeepsSpaceEstimate(t *testing.T) {
	s, downloads, _ := newTestService(t)
	root := t.TempDir()
	innoSource(t, root)
	downloads.add("d1", "Game", root)

	info, err := s.InspectDownload("d1")
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if info.Plan.EstimatedSize <= 0 || info.RequiredBytes <= info.Plan.EstimatedSize {
		t.Fatalf("space check lost its estimate: estimated = %d, required = %d", info.Plan.EstimatedSize, info.RequiredBytes)
	}
}

func TestProgressTotal(t *testing.T) {
	for _, tc := range []struct {
		name string
		kind Type
		want int64
	}{
		{"exe installer has no known total", TypeExeInstaller, 0},
		{"msi installer has no known total", TypeMsiInstaller, 0},
		{"zip keeps the extracted size", TypeArchiveZip, 500},
		{"portable keeps the copied size", TypePortable, 500},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := progressTotal(tc.kind, Plan{EstimatedSize: 500}); got != tc.want {
				t.Fatalf("progressTotal(%s) = %d, want %d", tc.kind, got, tc.want)
			}
		})
	}
}

func TestRetryResetsExternalTotal(t *testing.T) {
	s, downloads, _ := newTestService(t)
	root := t.TempDir()
	innoSource(t, root)
	downloads.add("d1", "Game", root)

	games := t.TempDir()
	dest := filepath.Join(games, "Game")
	s.roots = []string{games}
	s.runner = &fakeRunner{code: 1}

	item, err := s.Start("d1", StartOptions{Destination: dest})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	s.waitStatus(t, item.ID, StatusFailed)
	s.waitJobDone(t, item.ID)

	s.mu.Lock()
	s.findLocked(item.ID).BytesTotal = 999
	s.mu.Unlock()

	if err := s.Retry(item.ID); err != nil {
		t.Fatalf("retry: %v", err)
	}
	s.waitStatus(t, item.ID, StatusFailed)
	s.waitJobDone(t, item.ID)
	again, _ := s.snapshot(item.ID)
	if again.BytesTotal != 0 {
		t.Fatalf("bytesTotal after retry = %d, want 0", again.BytesTotal)
	}
}

func TestStartupClearsStaleExternalTotal(t *testing.T) {
	dir := t.TempDir()
	st := newStore(dir)
	if err := st.save([]Installation{
		{ID: "ext", Name: "Game", Type: TypeExeInstaller, Status: StatusInstalling, BytesTotal: 371 << 20},
		{ID: "zip", Name: "Other", Type: TypeArchiveZip, Status: StatusFailed, BytesTotal: 1 << 20},
	}); err != nil {
		t.Fatal(err)
	}

	s := mustServiceAt(t, dir)
	if err := s.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatalf("startup: %v", err)
	}
	t.Cleanup(func() {
		if err := s.ServiceShutdown(); err != nil {
			t.Errorf("shutdown: %v", err)
		}
	})

	ext, _ := s.snapshot("ext")
	if ext.BytesTotal != 0 {
		t.Fatalf("interrupted external installer keeps total %d, want 0", ext.BytesTotal)
	}
	zip, _ := s.snapshot("zip")
	if zip.BytesTotal != 1<<20 {
		t.Fatalf("archive total = %d, want it untouched", zip.BytesTotal)
	}
}
