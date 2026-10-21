package download

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"typhon/internal/settings"
)

// A Monday. The schedule below is open on Mondays from 09:00 to 17:00.
var mondayMorning = time.Date(2026, time.October, 19, 8, 0, 0, 0, time.UTC)

func mondayAt(h, m int) time.Time {
	return time.Date(2026, time.October, 19, h, m, 0, 0, time.UTC)
}

func scheduleSettings(mode string, max int) settings.Settings {
	cfg := playSettings(mode, 1<<20, 1<<19)
	cfg.MaxActiveDownloads = max
	cfg.DownloadSchedule = []settings.ScheduleWindow{{Days: []int{1}, From: "09:00", To: "17:00"}}
	return cfg
}

// scheduleManager returns a manager whose clock is the returned pointer, so a
// test moves time by assigning to it.
func scheduleManager(t *testing.T, max int, mode string, start time.Time) (*Manager, *time.Time) {
	t.Helper()
	now := start
	m := newTestManager(t, max)
	m.mu.Lock()
	m.clock = func() time.Time { return now }
	m.mu.Unlock()
	m.applySettings(scheduleSettings(mode, max))
	return m, &now
}

var nextMonday = mondayAt(0, 0).AddDate(0, 0, 7)

// closeWindow moves the clock past the end of the window and lets the tick
// notice.
func closeWindow(m *Manager, now *time.Time) {
	*now = mondayAt(18, 0)
	m.sample(context.Background(), *now)
}

func (m *Manager) ignoresSchedule(t *testing.T, id string) bool {
	t.Helper()
	d, err := m.Get(id)
	if err != nil {
		t.Fatalf("get %s: %v", id, err)
	}
	return d.IgnoreSchedule
}

func storedIgnore(t *testing.T, m *Manager, id string) bool {
	t.Helper()
	records, err := m.store.load()
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range records {
		if r.ID == id {
			return r.IgnoreSchedule
		}
	}
	t.Fatalf("download %s is not in the stored list", id)
	return false
}

func TestOutsideTheWindowTheGatesAreClosed(t *testing.T) {
	m, now := scheduleManager(t, 3, settings.PlayDownloadOff, mondayAt(12, 0))
	eng := addActive(m, "a")
	gates(t, eng, true, true, "in the window")
	if st := m.PlayThrottle(); st.Active || st.Schedule == nil || st.Schedule.Blocked {
		t.Fatalf("state in the window = %+v", st)
	}

	closeWindow(m, now)
	gates(t, eng, false, false, "after the window")
	if got := m.statusOf(t, "a"); got != StatusDownloading {
		t.Fatalf("status = %s, the stored status must not change", got)
	}
	st := m.PlayThrottle()
	if !st.Active || st.Schedule == nil || !st.Schedule.Blocked {
		t.Fatalf("state = %+v, want active and blocked by the schedule", st)
	}
	if want := nextMonday.Add(9 * time.Hour).Format(time.RFC3339); st.Schedule.NextOpenAt != want {
		t.Fatalf("next open = %q, want %q", st.Schedule.NextOpenAt, want)
	}
	if st.Mode != settings.PlayDownloadOff {
		t.Fatalf("mode = %q, the game did not cause this", st.Mode)
	}
}

func TestTheWindowOpeningFreesEveryDownload(t *testing.T) {
	m, now := scheduleManager(t, 3, settings.PlayDownloadOff, mondayAt(12, 0))
	a := addActive(m, "a")
	b := addActive(m, "b")
	seed := seedingEngine(m, "seed")
	closeWindow(m, now)
	gates(t, a, false, false, "a before the window")
	gates(t, b, false, false, "b before the window")
	gates(t, seed, false, false, "seed before the window")

	*now = nextMonday.Add(8*time.Hour + 59*time.Minute)
	m.sample(context.Background(), *now)
	gates(t, a, false, false, "a a minute before the window")

	*now = nextMonday.Add(9 * time.Hour)
	m.sample(context.Background(), *now)
	gates(t, a, true, true, "a in the window")
	gates(t, b, true, true, "b in the window")
	gates(t, seed, false, true, "seed in the window")
	st := m.PlayThrottle()
	if st.Active || st.Schedule == nil || st.Schedule.Blocked || st.Schedule.NextOpenAt != "" {
		t.Fatalf("state in the window = %+v", st)
	}
}

func TestTheScheduleIsJudgedOncePerMinute(t *testing.T) {
	m, now := scheduleManager(t, 3, settings.PlayDownloadOff, mondayAt(12, 0))
	closeWindow(m, now)
	m.mu.Lock()
	m.scheduleBlocked = false
	m.mu.Unlock()

	*now = mondayAt(18, 0).Add(30 * time.Second)
	m.sample(context.Background(), *now)
	m.mu.Lock()
	sameMinute := m.scheduleBlocked
	m.mu.Unlock()
	if sameMinute {
		t.Fatal("the schedule was judged again inside the same minute")
	}

	*now = mondayAt(18, 1)
	m.sample(context.Background(), *now)
	m.mu.Lock()
	nextMinute := m.scheduleBlocked
	m.mu.Unlock()
	if !nextMinute {
		t.Fatal("the schedule was not judged when the minute turned")
	}
}

func TestDownloadNowFreesOnlyThatDownload(t *testing.T) {
	m, now := scheduleManager(t, 3, settings.PlayDownloadOff, mondayAt(12, 0))
	a := addActive(m, "a")
	b := addActive(m, "b")
	closeWindow(m, now)
	gates(t, a, false, false, "a before Download now")

	if err := m.DownloadNow("a"); err != nil {
		t.Fatal(err)
	}
	gates(t, a, true, true, "a after Download now")
	gates(t, b, false, false, "b after Download now")
	if !m.ignoresSchedule(t, "a") || m.ignoresSchedule(t, "b") {
		t.Fatal("the flag must be set on a and only on a")
	}
	if !storedIgnore(t, m, "a") {
		t.Fatal("the flag was not written to the stored list")
	}
}

func TestDownloadNowSurvivesARestart(t *testing.T) {
	m, _ := scheduleManager(t, 3, settings.PlayDownloadOff, mondayMorning)
	m.addTestItem("a", StatusQueued)
	if err := m.DownloadNow("a"); err != nil {
		t.Fatal(err)
	}

	again := mustManagerAt(t, m.store.dir)
	again.mu.Lock()
	err := again.loadLocked()
	again.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if !again.ignoresSchedule(t, "a") {
		t.Fatal("the flag was lost on load")
	}
}

func TestDownloadNowUnknownAndFinishedDownloads(t *testing.T) {
	m, _ := scheduleManager(t, 3, settings.PlayDownloadOff, mondayMorning)
	m.addTestItem("done", StatusCompleted)
	if err := m.DownloadNow("nope"); !errors.Is(err, errNotFound) {
		t.Fatalf("unknown id: error = %v, want %v", err, errNotFound)
	}
	if err := m.DownloadNow("done"); !errors.Is(err, errUnavailable) {
		t.Fatalf("completed download: error = %v, want %v", err, errUnavailable)
	}
	if m.ignoresSchedule(t, "done") {
		t.Fatal("a finished download must not get the flag")
	}
}

func TestDownloadNowRollsBackOnPersistFailure(t *testing.T) {
	m, now := scheduleManager(t, 3, settings.PlayDownloadOff, mondayAt(12, 0))
	eng := addActive(m, "a")
	closeWindow(m, now)
	breakStore(t, m)

	if err := m.DownloadNow("a"); err == nil {
		t.Fatal("DownloadNow must return the persist failure")
	}
	if m.ignoresSchedule(t, "a") {
		t.Fatal("the flag stayed set although it was not stored")
	}
	gates(t, eng, false, false, "after the failed Download now")
	if st := m.degradedStatus(); !st.Degraded || st.Message == "" {
		t.Fatalf("degraded status = %+v, want degraded with a message", st)
	}
}

func TestPlayPauseHoldsADownloadThatIgnoresTheSchedule(t *testing.T) {
	m, now := scheduleManager(t, 3, settings.PlayDownloadPause, mondayAt(12, 0))
	forced := addActive(m, "forced")
	plain := addActive(m, "plain")
	closeWindow(m, now)
	if err := m.DownloadNow("forced"); err != nil {
		t.Fatal(err)
	}
	gates(t, forced, true, true, "forced before the game")

	m.SetGamesRunning(true)
	gates(t, forced, false, false, "forced during the game, outside the window")
	gates(t, plain, false, false, "plain during the game, outside the window")

	m.SetGamesRunning(false)
	gates(t, forced, true, true, "forced after the game, outside the window")
	gates(t, plain, false, false, "plain after the game, outside the window")

	*now = nextMonday.Add(10 * time.Hour)
	m.sample(context.Background(), *now)
	m.SetGamesRunning(true)
	gates(t, forced, false, false, "forced during the game, inside the window")
	gates(t, plain, false, false, "plain during the game, inside the window")

	m.SetGamesRunning(false)
	gates(t, forced, true, true, "forced after the game, inside the window")
	gates(t, plain, true, true, "plain after the game, inside the window")
}

func TestFinishingADownloadClearsTheFlag(t *testing.T) {
	m, now := scheduleManager(t, 3, settings.PlayDownloadOff, mondayAt(12, 0))
	eng := addActive(m, "a")
	closeWindow(m, now)
	if err := m.DownloadNow("a"); err != nil {
		t.Fatal(err)
	}
	d, err := m.findForTest("a")
	if err != nil {
		t.Fatal(err)
	}

	m.mu.Lock()
	m.completeLocked(d)
	m.mu.Unlock()

	if m.ignoresSchedule(t, "a") {
		t.Fatal("the flag outlived the download")
	}
	if storedIgnore(t, m, "a") {
		t.Fatal("the flag was not cleared in the stored list")
	}
	gates(t, eng, false, false, "a finished download seeds only inside the window")
}

func (m *Manager) findForTest(id string) (*Download, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	d := m.findLocked(id)
	if d == nil {
		return nil, errNotFound
	}
	return d, nil
}

func TestChangingTheScheduleActsAtOnce(t *testing.T) {
	m := newTestManager(t, 3)
	now := mondayMorning
	m.mu.Lock()
	m.clock = func() time.Time { return now }
	m.mu.Unlock()
	m.applySettings(playSettings(settings.PlayDownloadOff, 1<<20, 1<<19))
	eng := addActive(m, "a")
	gates(t, eng, true, true, "no schedule")
	if st := m.PlayThrottle(); st.Schedule != nil || st.Active {
		t.Fatalf("state without a schedule = %+v", st)
	}

	m.applySettings(scheduleSettings(settings.PlayDownloadOff, 3))
	gates(t, eng, false, false, "a schedule that is closed now")

	m.applySettings(playSettings(settings.PlayDownloadOff, 1<<20, 1<<19))
	gates(t, eng, true, true, "the schedule removed")
}

func TestADownloadThatIgnoresTheScheduleTakesAFreeSlot(t *testing.T) {
	m, now := scheduleManager(t, 1, settings.PlayDownloadOff, mondayAt(12, 0))
	held := addActive(m, "held")
	waiting := addActive(m, "waiting")
	closeWindow(m, now)
	if got := m.statusOf(t, "held"); got != StatusDownloading {
		t.Fatalf("held = %s, a running download keeps its status when the window closes", got)
	}
	if got := m.statusOf(t, "waiting"); got != StatusQueued {
		t.Fatalf("waiting = %s, want queued behind the held download", got)
	}

	if err := m.DownloadNow("waiting"); err != nil {
		t.Fatal(err)
	}
	if got := m.statusOf(t, "waiting"); got != StatusDownloading {
		t.Fatalf("waiting = %s, want downloading: the held one moves no data", got)
	}
	if !waiting.isDownloading() {
		t.Fatal("waiting after Download now: download gate is closed")
	}
	gates(t, held, false, false, "held after Download now")
}

func TestQueuedDownloadsStayQueuedOutsideTheWindowAndStartInIt(t *testing.T) {
	m, now := scheduleManager(t, 1, settings.PlayDownloadOff, mondayMorning)
	m.addTestDownload("a")
	if got := m.statusOf(t, "a"); got != StatusQueued {
		t.Fatalf("status outside the window = %s, want queued", got)
	}

	*now = mondayAt(9, 0)
	m.sample(context.Background(), *now)
	if got := m.statusOf(t, "a"); got != StatusDownloading {
		t.Fatalf("status in the window = %s, want downloading", got)
	}
}

func TestScheduleStateIsSafeUnderConcurrentUse(t *testing.T) {
	m, now := scheduleManager(t, 3, settings.PlayDownloadPause, mondayAt(12, 0))
	for _, id := range []string{"a", "b", "c"} {
		addActive(m, id)
	}
	closeWindow(m, now)

	var wg sync.WaitGroup
	run := func(fn func(i int)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range 50 {
				fn(i)
			}
		}()
	}
	run(func(int) { m.sample(context.Background(), mondayAt(18, 0)) })
	run(func(i int) { m.SetGamesRunning(i%2 == 0) })
	run(func(int) { m.PlayThrottle() })
	run(func(i int) { m.applySettings(scheduleSettings(settings.PlayDownloadPause, 3)) })
	run(func(int) {
		if err := m.DownloadNow("b"); err != nil {
			t.Error(err)
		}
	})
	wg.Wait()
	if !m.ignoresSchedule(t, "b") {
		t.Fatal("the flag was lost")
	}
}
