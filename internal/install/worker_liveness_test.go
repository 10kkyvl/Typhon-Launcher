package install

import (
	"errors"
	"os"
	"testing"
)

// A state file that cannot be read says nothing about the worker. Treating it
// as "dead" let Retry remove the worker's files and start a second installer
// into the same folder, and let Dismiss delete the files of a live worker.
func TestUnreadableWorkerStateIsNotTreatedAsADeadWorker(t *testing.T) {
	const id = "unknown1"
	newCase := func(t *testing.T) *Service {
		t.Helper()
		s, downloads, _ := newTestService(t)
		downloads.add("d1", "Pack", t.TempDir())
		s.mu.Lock()
		s.items = append(s.items, &Installation{ID: id, DownloadID: "d1", Name: "Game", Type: TypeExeInstaller, Status: StatusFailed, Error: "boom"})
		s.mu.Unlock()
		// A directory where the state file should be: ReadFile fails, which is
		// neither "no worker" nor "worker alive".
		if err := os.MkdirAll(s.workerStatePath(id), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := writeWorkerFile(s.workerCancelPath(id), []byte{}); err != nil {
			t.Fatal(err)
		}
		return s
	}

	t.Run("retry refuses instead of starting a second installer", func(t *testing.T) {
		s := newCase(t)
		spawned := false
		withWorkerSeams(t, func(runSpec) (workerHandle, error) {
			spawned = true
			return nil, errWorkerStatePath
		})
		err := s.Retry(id)
		if !errors.Is(err, errWorkerStateUnknown) {
			t.Fatalf("Retry = %v, want %v", err, errWorkerStateUnknown)
		}
		if spawned {
			t.Fatal("a second worker was started over an installer whose state could not be read")
		}
		if _, err := os.Stat(s.workerCancelPath(id)); err != nil {
			t.Fatalf("retry removed the worker's files: %v", err)
		}
	})

	t.Run("dismiss refuses and keeps the worker files", func(t *testing.T) {
		s := newCase(t)
		if err := s.Dismiss(id); !errors.Is(err, errWorkerStateUnknown) {
			t.Fatalf("Dismiss = %v, want %v", err, errWorkerStateUnknown)
		}
		if _, err := os.Stat(s.workerCancelPath(id)); err != nil {
			t.Fatalf("dismiss removed the files of a worker whose state could not be read: %v", err)
		}
		if got := s.List(); len(got) != 1 || got[0].ID != id {
			t.Fatalf("dismiss dropped the record over an installer whose state could not be read: %+v", got)
		}
	})
}
