//go:build windows

package install

import (
	"os"
	"testing"
)

// Настоящий процесс (этот же) играет живого воркера: workerProcessAlive вне
// Windows всегда отвечает «нет».
func TestDismissKeepsTheFilesOfAWorkerThatIsStillRunning(t *testing.T) {
	r := newRig(t)
	r.add(Installation{ID: "dd", Name: "D", Status: StatusFailed, Error: errInstallerStillRunning.Error()})
	files := seedWorkerFiles(t, r.dir, "dd", workerState{PID: os.Getpid()})

	if err := r.s.Dismiss("dd"); err != nil {
		t.Fatal(err)
	}

	if got := r.s.List(); len(got) != 0 {
		t.Fatalf("records after Dismiss = %+v, want none", got)
	}
	for _, path := range files {
		if !exists(path) {
			t.Fatalf("%s of a worker that is still running was removed by Dismiss", path)
		}
	}
}
