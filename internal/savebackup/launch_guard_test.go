package savebackup

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"typhon/internal/hashdir"
	"typhon/internal/library"
	"typhon/internal/uierr"
)

func TestListReportsPlayTimeAfterEachSnapshotWithOneQuery(t *testing.T) {
	h := newHarness(t)
	first := h.create()
	writeFile(t, filepath.Join(h.saves, "slot1.sav"), "later")
	second := h.create()
	calls := 0
	h.svc.SetPlaytime(func(id string, since []time.Time) ([]Played, error) {
		calls++
		if id != gameID {
			t.Errorf("asked for game %q", id)
		}
		out := make([]Played, len(since))
		for i, at := range since {
			switch {
			case at.Equal(first.CreatedAt):
				out[i] = Played{Seconds: 3600, Known: true}
			case at.Equal(second.CreatedAt):
				out[i] = Played{Known: true}
			}
		}
		return out, nil
	})

	byID := map[string]Snapshot{}
	for _, s := range h.list() {
		byID[s.ID] = s
	}

	if calls != 1 {
		t.Fatalf("play log asked %d times, want once per List", calls)
	}
	if s := byID[first.ID]; !s.PlayedAfterKnown || s.PlayedAfterSeconds != 3600 {
		t.Fatalf("first = %+v, want 3600 s known", s)
	}
	if s := byID[second.ID]; !s.PlayedAfterKnown || s.PlayedAfterSeconds != 0 {
		t.Fatalf("second = %+v, want a known zero", s)
	}
}

func TestListMarksPlayTimeUnknownWhenThePlayLogFails(t *testing.T) {
	cases := []struct {
		name string
		fn   PlaytimeFunc
	}{
		{"play log errors", func(string, []time.Time) ([]Played, error) {
			return []Played{{Seconds: 99, Known: true}}, errors.New("degraded")
		}},
		{"wrong number of answers", func(string, []time.Time) ([]Played, error) { return nil, nil }},
		{"moment unknown", func(string, []time.Time) ([]Played, error) { return []Played{{Seconds: 99}}, nil }},
		{"no play log wired", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness(t)
			h.create()
			h.svc.SetPlaytime(c.fn)

			list := h.list()

			if len(list) != 1 || list[0].PlayedAfterKnown || list[0].PlayedAfterSeconds != 0 {
				t.Fatalf("list = %+v, want unknown and no number", list)
			}
		})
	}
}

func TestSessionStartingReturnsAtTheDeadlineWhenAStepIgnoresContext(t *testing.T) {
	h := newHarness(t)
	h.svc.launchTimeout = 20 * time.Millisecond
	release := make(chan struct{})
	h.svc.build = func(ctx context.Context, root string, p func(hashdir.Progress)) (hashdir.Manifest, error) {
		<-release
		return hashdir.Build(context.WithoutCancel(ctx), root, p)
	}

	err := h.starting(context.Background())

	if !errors.Is(err, errLaunchTimeout) {
		t.Fatalf("err = %v, want the launch timeout while a step is stuck", err)
	}
	if ev := h.next(); ev.Code != "savebackup.launch_timeout" {
		t.Fatalf("event = %+v, want launch_timeout", ev)
	}
	close(release)
	h.svc.wg.Wait()
	if n := len(h.list()); n != 0 {
		t.Fatalf("%d copies, the abandoned attempt must not publish one", n)
	}
	if leftovers, err := filepath.Glob(filepath.Join(h.gameDir(), "*"+partialSuffix)); err != nil || len(leftovers) != 0 {
		t.Fatalf("leftover partial copies %v (glob error %v)", leftovers, err)
	}
}

func TestSessionStartingRefusesToLaunchOverAnUnfinishedRollback(t *testing.T) {
	h := newHarness(t)
	writeFile(t, h.svc.journalPath(gameID), "{not json")

	err := h.starting(context.Background())

	var blocked *library.LaunchBlockedError
	if !errors.As(err, &blocked) || uierr.Code(err) != "savebackup.recovery_pending" {
		t.Fatalf("err = %v, want a blocking recovery_pending", err)
	}
	ev := h.next()
	if ev.Status != StatusFailed || ev.Code != "savebackup.recovery_pending" || ev.Kind != KindLaunch {
		t.Fatalf("event = %+v, want failed recovery_pending", ev)
	}
}

func TestSessionStartingTimeoutOverAnUnfinishedRollbackStillBlocks(t *testing.T) {
	h := newHarness(t)
	h.svc.launchTimeout = 20 * time.Millisecond
	writeFile(t, h.svc.journalPath(gameID), "{not json")
	release := h.svc.lockGame(gameID)
	defer release()

	err := h.starting(context.Background())

	var blocked *library.LaunchBlockedError
	if !errors.As(err, &blocked) {
		t.Fatalf("err = %v, want a blocking error: a journal means saves may be mid-swap", err)
	}
}

func TestSessionStartingSkippedWarningIsRecordedOncePerGame(t *testing.T) {
	h := newHarness(t)
	h.games.setResult(gameID, library.SavesResult{Candidates: []string{"a", "b"}})

	for range 3 {
		if err := h.starting(context.Background()); err != nil {
			t.Fatal(err)
		}
	}

	if got := len(h.drain()); got != 3 {
		t.Fatalf("%d events, want one skipped event per launch", got)
	}
	if n := len(h.svc.warned); n != 1 {
		t.Fatalf("%d warnings recorded, want 1", n)
	}
}

func TestRealFailureIsNotReportedAsTimeout(t *testing.T) {
	h := newHarness(t)
	h.games.setResult(gameID, library.SavesResult{Path: filepath.Join(t.TempDir(), "gone")})

	err := h.starting(context.Background())

	if err == nil || errors.Is(err, errLaunchTimeout) {
		t.Fatalf("err = %v, want the real failure", err)
	}
}

func TestSnapshotFileKeepsOnlyStoredFields(t *testing.T) {
	h := newHarness(t)
	h.svc.largeAt = 1
	snap := h.create()

	data := readFile(t, filepath.Join(filepath.Dir(snap.Path), snapshotFile))

	for _, key := range []string{"large", "playedAfter"} {
		if strings.Contains(data, key) {
			t.Fatalf("snapshot.json contains derived field %q:\n%s", key, data)
		}
	}
}
