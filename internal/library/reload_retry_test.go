package library

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"sync/atomic"
	"testing"

	"typhon/internal/handoffgate"
	"typhon/internal/storage"
	"typhon/internal/uierr"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// heldLibraryFile makes the next failures reads of the library's files fail
// the way a file another program holds for a moment does, and counts every
// read from now on. onFail runs inside each failed read.
func heldLibraryFile(t *testing.T, failures int32, onFail func()) *atomic.Int32 {
	t.Helper()
	var reads atomic.Int32
	loadEmulatedFile = func(path string, version int, migrations map[int]storage.Migration, out any) error {
		if reads.Add(1) <= failures {
			if onFail != nil {
				onFail()
			}
			return fmt.Errorf("open %s: %w", path, fs.ErrPermission)
		}
		return storage.Load(path, version, migrations, out)
	}
	t.Cleanup(func() { loadEmulatedFile = storage.Load })
	return &reads
}

// heldSuccessor is a successor whose old launcher has just exited, its gate
// still closed, built over the library seedLibrary writes.
func heldSuccessor(t *testing.T) (*Service, string) {
	t.Helper()
	path := seedLibrary(t)
	handoffgate.Arm()
	t.Cleanup(handoffgate.Open)
	succ := mustServiceAt(t, path)
	if err := succ.ServiceStartup(t.Context(), application.ServiceOptions{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := succ.ServiceShutdown(); err != nil {
			t.Error(err)
		}
	})
	return succ, path
}

// On Windows another program (an antivirus, a backup) may hold the file for
// a moment as the old launcher exits. One failed read is tried again, and a
// second read that works leaves the library saving as usual.
func TestTheReReadBeforeTheGateTriesAgainAfterAFailedRead(t *testing.T) {
	succ, path := heldSuccessor(t)
	reads := heldLibraryFile(t, 1, nil)
	logs := captureSessionLogs(t)

	if err := succ.ReloadBeforeGateOpens(t.Context()); err != nil {
		t.Fatalf("ReloadBeforeGateOpens() = %v, want the second read to succeed", err)
	}
	openGate(t)

	if got := reads.Load(); got != 2 {
		t.Fatalf("the library was read %d times, want 2", got)
	}
	if _, err := succ.SetFavorite("b", true); err != nil {
		t.Fatalf("SetFavorite() after a read that worked on the second try = %v", err)
	}
	if !favourite(t, mustServiceAt(t, path), "b") {
		t.Fatal("library.json does not hold the save made after the retried read")
	}
	if got := strings.Count(logs.String(), "attempts="); got != 1 {
		t.Fatalf("%d log lines about the retried read, want 1:\n%s", got, logs.String())
	}
}

// Three failed reads are the old behaviour: the snapshot stays and every save
// is refused with the code the interface names.
func TestTheReReadBeforeTheGateGivesUpAfterThreeReads(t *testing.T) {
	succ, _ := heldSuccessor(t)
	reads := heldLibraryFile(t, 1000, nil)

	err := succ.ReloadBeforeGateOpens(t.Context())
	if !errors.Is(err, fs.ErrPermission) {
		t.Fatalf("ReloadBeforeGateOpens() = %v, want the read's cause", err)
	}
	openGate(t)

	if got := reads.Load(); got != 3 {
		t.Fatalf("the library was read %d times, want 3", got)
	}
	if got := len(succ.GetGames()); got != 2 {
		t.Fatalf("the successor holds %d games, want the 2 it read at construction", got)
	}
	_, err = succ.SetFavorite("b", true)
	if code := uierr.Code(err); code != "library.state_unreadable" {
		t.Fatalf("SetFavorite() = %v (code %q), want library.state_unreadable", err, code)
	}
}

// A launcher that is quitting does not wait out the pause: the re-read returns
// as soon as its context ends, with both causes, and saves stay refused.
func TestTheReReadBeforeTheGateStopsWhenItsContextEnds(t *testing.T) {
	succ, _ := heldSuccessor(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	reads := heldLibraryFile(t, 1000, cancel)

	err := succ.ReloadBeforeGateOpens(ctx)
	if !errors.Is(err, context.Canceled) || !errors.Is(err, fs.ErrPermission) {
		t.Fatalf("ReloadBeforeGateOpens() = %v, want the cancellation and the read's cause", err)
	}
	openGate(t)

	if got := reads.Load(); got != 1 {
		t.Fatalf("the library was read %d times after its context ended, want 1", got)
	}
	_, err = succ.SetFavorite("b", true)
	if code := uierr.Code(err); code != "library.state_unreadable" {
		t.Fatalf("SetFavorite() = %v (code %q), want library.state_unreadable", err, code)
	}
}
