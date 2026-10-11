package install

import (
	"context"
	"errors"
	"testing"
)

func TestStartupReleasesWaitingInstallWhoseDownloadIsGone(t *testing.T) {
	r := newRig(t)
	r.add(waitingFor("orphan", "d1"))
	r.restart()
	r.s.wg.Wait()

	if got := itemStatus(t, r, "orphan"); got != StatusCancelled {
		t.Fatalf("status = %s, want %s", got, StatusCancelled)
	}
	if got := r.diskItem("orphan").Status; got != StatusCancelled {
		t.Fatalf("saved status = %s, want %s", got, StatusCancelled)
	}
	if err := r.s.Dismiss("orphan"); err != nil {
		t.Fatalf("Dismiss of the released install: %v", err)
	}
}

func TestStartupKeepsWaitingInstallsItCannotProveOrphaned(t *testing.T) {
	cases := []struct {
		name  string
		setup func(r *rig)
		item  Installation
	}{
		{
			name:  "download still exists",
			setup: func(r *rig) { r.download("d1", "Game", t.TempDir()) },
			item:  waitingFor("w", "d1"),
		},
		{
			name:  "lookup fails with another error",
			setup: func(r *rig) { r.downloads.getErr["d1"] = errors.New("manager is busy") },
			item:  waitingFor("w", "d1"),
		},
		{
			name:  "download gone but the game already sits in its destination",
			setup: func(*rig) {},
			item: func() Installation {
				item := waitingFor("w", "d1")
				item.Destination = t.TempDir()
				return item
			}(),
		},
		{
			name:  "no download recorded",
			setup: func(*rig) {},
			item:  waitingFor("w", ""),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newRig(t)
			tc.setup(r)
			r.add(tc.item)
			r.restart()
			r.s.wg.Wait()

			if got := itemStatus(t, r, "w"); got != StatusWaitingForUser {
				t.Fatalf("status = %s, want %s", got, StatusWaitingForUser)
			}
			if got := r.diskItem("w").Status; got != StatusWaitingForUser {
				t.Fatalf("saved status = %s, want %s", got, StatusWaitingForUser)
			}
		})
	}
}

func TestReleaseOrphanedWaitingStopsOnCancelledContext(t *testing.T) {
	r := newRig(t)
	r.add(waitingFor("orphan", "d1"))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := r.s.releaseOrphanedWaiting(ctx, []string{"d1"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("releaseOrphanedWaiting = %v, want %v", err, context.Canceled)
	}
	if got := itemStatus(t, r, "orphan"); got != StatusWaitingForUser {
		t.Fatalf("status = %s, want %s", got, StatusWaitingForUser)
	}
}
