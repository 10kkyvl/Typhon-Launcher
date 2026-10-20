package download

import (
	"context"
	"strconv"
	"sync"
	"testing"
	"time"

	"typhon/internal/library"
	"typhon/internal/settings"

	"golang.org/x/time/rate"
)

func playSettings(mode string, down, up int64) settings.Settings {
	cfg := settings.Defaults()
	cfg.PlayDownloadMode = mode
	cfg.PlayDownloadRateLimit = down
	cfg.PlayUploadRateLimit = up
	cfg.UploadWhileDownloading = true
	cfg.SeedAfterDownload = true
	return cfg
}

func gates(t *testing.T, eng *fakeTorrent, wantDown, wantUp bool, what string) {
	t.Helper()
	if got := eng.isDownloading(); got != wantDown {
		t.Errorf("%s: download gate open = %v, want %v", what, got, wantDown)
	}
	if got := eng.isUploading(); got != wantUp {
		t.Errorf("%s: upload gate open = %v, want %v", what, got, wantUp)
	}
}

func seedingEngine(m *Manager, id string) *fakeTorrent {
	eng := &fakeTorrent{size: 100}
	d := m.addTestItem(id, StatusCompleted)
	m.mu.Lock()
	d.Seeding = true
	m.engines[id] = eng
	m.mu.Unlock()
	eng.allowUpload()
	return eng
}

// addActive adds a download that is already running and opens its upload gate
// the way a settings save with upload-while-downloading does.
func addActive(m *Manager, id string) *fakeTorrent {
	eng := m.addTestDownload(id)
	m.applySettings(m.rateCfg)
	return eng
}

func playManager(t *testing.T, mode string) *Manager {
	t.Helper()
	m := newTestManager(t, 3)
	m.applySettings(playSettings(mode, 1<<20, 1<<19))
	return m
}

func TestPauseModeClosesTheGatesAndRestoresThem(t *testing.T) {
	m := playManager(t, settings.PlayDownloadPause)
	active := addActive(m, "active")
	seed := seedingEngine(m, "seed")
	gates(t, active, true, true, "before the game")
	gates(t, seed, false, true, "before the game seed")

	m.SetGamesRunning(true)
	gates(t, active, false, false, "during the game")
	gates(t, seed, false, false, "during the game seed")
	if got := m.statusOf(t, "active"); got != StatusDownloading {
		t.Fatalf("status during the game = %s, the stored status must not change", got)
	}
	if st := m.PlayThrottle(); !st.Active || st.Mode != settings.PlayDownloadPause {
		t.Fatalf("state = %+v", st)
	}

	m.SetGamesRunning(false)
	gates(t, active, true, true, "after the game")
	gates(t, seed, false, true, "after the game seed")
	if st := m.PlayThrottle(); st.Active || st.Mode != settings.PlayDownloadOff {
		t.Fatalf("state after the game = %+v", st)
	}
}

func TestUserPauseSurvivesTheGameAndResumeDuringItStaysGated(t *testing.T) {
	m := playManager(t, settings.PlayDownloadPause)
	paused := addActive(m, "paused")
	if err := m.Pause("paused"); err != nil {
		t.Fatal(err)
	}
	m.SetGamesRunning(true)
	m.SetGamesRunning(false)
	if got := m.statusOf(t, "paused"); got != StatusPaused {
		t.Fatalf("status = %s, want paused", got)
	}
	gates(t, paused, false, false, "user-paused torrent after the game")

	m.SetGamesRunning(true)
	if err := m.Resume("paused"); err != nil {
		t.Fatal(err)
	}
	if got := m.statusOf(t, "paused"); got != StatusDownloading {
		t.Fatalf("status = %s, want downloading", got)
	}
	gates(t, paused, false, false, "resumed during the game")

	m.SetGamesRunning(false)
	gates(t, paused, true, true, "resumed torrent after the game")
}

func TestQueueStartDuringTheGameStaysGated(t *testing.T) {
	m := playManager(t, settings.PlayDownloadPause)
	m.SetGamesRunning(true)
	eng := addActive(m, "late")
	if got := m.statusOf(t, "late"); got != StatusDownloading {
		t.Fatalf("status = %s", got)
	}
	gates(t, eng, false, false, "started during the game")
	m.SetGamesRunning(false)
	gates(t, eng, true, true, "after the game")
}

func TestSettingsChangeDuringTheGameKeepsTheOverlay(t *testing.T) {
	m := playManager(t, settings.PlayDownloadPause)
	eng := addActive(m, "a")
	seed := seedingEngine(m, "seed")
	m.SetGamesRunning(true)

	next := playSettings(settings.PlayDownloadPause, 1<<20, 1<<19)
	next.SeedAfterDownload = true
	next.MaxActiveDownloads = 5
	m.applySettings(next)

	gates(t, eng, false, false, "after an unrelated settings change")
	gates(t, seed, false, false, "seed after an unrelated settings change")
}

func TestSwitchingToOffDuringTheGameReleasesAtOnce(t *testing.T) {
	m := playManager(t, settings.PlayDownloadPause)
	eng := addActive(m, "a")
	seed := seedingEngine(m, "seed")
	m.SetGamesRunning(true)
	gates(t, eng, false, false, "during")

	m.applySettings(playSettings(settings.PlayDownloadOff, 0, 0))

	gates(t, eng, true, true, "mode off")
	gates(t, seed, false, true, "mode off seed")
	if st := m.PlayThrottle(); st.Active {
		t.Fatalf("state = %+v", st)
	}
}

func TestSwitchingToLimitDuringThePauseReleasesTheGates(t *testing.T) {
	m := playManager(t, settings.PlayDownloadPause)
	m.client = offlineClient(t)
	eng := addActive(m, "a")
	m.SetGamesRunning(true)

	m.applySettings(playSettings(settings.PlayDownloadLimit, 1<<20, 1<<19))

	gates(t, eng, true, true, "limit mode")
	if got := m.client.down.Limit(); got != rate.Limit(1<<20) {
		t.Fatalf("download limit = %v", got)
	}
}

func TestOnlyTheLastGameReleases(t *testing.T) {
	m := playManager(t, settings.PlayDownloadPause)
	eng := addActive(m, "a")
	w := m.PlayWatcher()

	w.SessionStarted(library.Game{ID: "g1"})
	w.SessionStarted(library.Game{ID: "g2"})
	w.SessionStarted(library.Game{ID: "g1"})
	gates(t, eng, false, false, "two games")

	w.SessionStopped("g1")
	gates(t, eng, false, false, "one game left")
	w.SessionStopped("unknown")
	gates(t, eng, false, false, "stop of a game that never started")

	w.SessionStopped("g2")
	gates(t, eng, true, true, "no game left")
}

func TestPlayWatcherListenersSeeOnlyTransitions(t *testing.T) {
	m := playManager(t, settings.PlayDownloadOff)
	w := m.PlayWatcher()
	var seen []bool
	w.Listen(func(running bool) { seen = append(seen, running) })

	w.SessionStarted(library.Game{ID: "g1"})
	w.SessionStarted(library.Game{ID: "g2"})
	w.SessionStopped("g1")
	w.SessionStopped("g2")

	if len(seen) != 2 || !seen[0] || seen[1] {
		t.Fatalf("transitions = %v, want [true false]", seen)
	}
}

func limitOf(v int64) rate.Limit {
	if v <= 0 {
		return rate.Inf
	}
	return rate.Limit(v)
}

func TestLimitModeUsesTheLowerOfTheTwoLimits(t *testing.T) {
	cases := []struct {
		name             string
		userDown, userUp int64
		gameDown, gameUp int64
		wantDown, wantUp int64
	}{
		{"user unlimited", 0, 0, 1 << 20, 1 << 19, 1 << 20, 1 << 19},
		{"user lower", 1 << 18, 1 << 17, 1 << 20, 1 << 19, 1 << 18, 1 << 17},
		{"user higher", 5 << 20, 5 << 20, 1 << 20, 1 << 19, 1 << 20, 1 << 19},
		{"only download limited by user", 1 << 18, 0, 1 << 20, 1 << 19, 1 << 18, 1 << 19},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := newTestManager(t, 1)
			m.client = offlineClient(t)
			cfg := playSettings(settings.PlayDownloadLimit, c.gameDown, c.gameUp)
			cfg.DownloadRateLimit, cfg.UploadRateLimit = c.userDown, c.userUp
			m.applySettings(cfg)
			if m.client.down.Limit() != limitOf(c.userDown) || m.client.up.Limit() != limitOf(c.userUp) {
				t.Fatalf("limits changed before a game: %v %v", m.client.down.Limit(), m.client.up.Limit())
			}

			m.SetGamesRunning(true)
			if got := m.client.down.Limit(); got != limitOf(c.wantDown) {
				t.Errorf("download limit = %v, want %v", got, limitOf(c.wantDown))
			}
			if got := m.client.up.Limit(); got != limitOf(c.wantUp) {
				t.Errorf("upload limit = %v, want %v", got, limitOf(c.wantUp))
			}
			st := m.PlayThrottle()
			if st.DownloadLimit != c.wantDown || st.UploadLimit != c.wantUp {
				t.Errorf("state limits = %d/%d, want %d/%d", st.DownloadLimit, st.UploadLimit, c.wantDown, c.wantUp)
			}

			cfg.DownloadRateLimit = 2 << 20
			m.applySettings(cfg)
			want := int64(2 << 20)
			if c.gameDown < want {
				want = c.gameDown
			}
			if got := m.client.down.Limit(); got != rate.Limit(want) {
				t.Errorf("limit after a settings change during the game = %v, want %v", got, rate.Limit(want))
			}

			m.SetGamesRunning(false)
			if got := m.client.down.Limit(); got != rate.Limit(2<<20) {
				t.Errorf("download limit after the game = %v, want the user's", got)
			}
			if got := m.client.up.Limit(); got != limitOf(c.userUp) {
				t.Errorf("upload limit after the game = %v, want the user's", got)
			}
		})
	}
}

func TestPauseModeNeverZeroesTheLimiters(t *testing.T) {
	m := playManager(t, settings.PlayDownloadPause)
	m.client = offlineClient(t)
	m.SetGamesRunning(true)
	if m.client.down.Limit() != rate.Inf || m.client.up.Limit() != rate.Inf {
		t.Fatalf("limiters = %v %v, pause must use the gates: a zero limiter blocks dialing", m.client.down.Limit(), m.client.up.Limit())
	}
}

func TestAClientInstalledDuringTheGameGetsTheLimit(t *testing.T) {
	m := newTestManager(t, 1)
	m.applySettings(playSettings(settings.PlayDownloadLimit, 1<<20, 1<<19))
	m.SetGamesRunning(true)
	m.mu.Lock()
	m.client = offlineClient(t)
	m.applyRatesLocked()
	m.mu.Unlock()
	if got := m.client.down.Limit(); got != rate.Limit(1<<20) {
		t.Fatalf("download limit = %v", got)
	}
}

func TestPlayThrottleEventFiresOnlyOnChange(t *testing.T) {
	m := playManager(t, settings.PlayDownloadPause)
	var states []PlayThrottleState
	original := emit
	emit = func(name string, data any) {
		if st, ok := data.(PlayThrottleState); ok && name == eventPlayThrottle {
			states = append(states, st)
		}
	}
	t.Cleanup(func() { emit = original })

	m.SetGamesRunning(true)
	m.SetGamesRunning(true)
	m.applySettings(playSettings(settings.PlayDownloadPause, 1<<20, 1<<19))
	m.SetGamesRunning(false)

	if len(states) != 2 || !states[0].Active || states[1].Active {
		t.Fatalf("events = %+v", states)
	}
}

func TestConcurrentSessionsAndSettingsKeepTheGatesConsistent(t *testing.T) {
	m := playManager(t, settings.PlayDownloadPause)
	eng := addActive(m, "a")
	w := m.PlayWatcher()

	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				w.SessionStarted(library.Game{ID: id})
				w.SessionStopped(id)
			}
		}("g" + strconv.Itoa(i))
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for j := 0; j < 100; j++ {
			mode := settings.PlayDownloadPause
			if j%2 == 0 {
				mode = settings.PlayDownloadOff
			}
			m.applySettings(playSettings(mode, 1<<20, 1<<19))
			m.PlayThrottle()
		}
	}()
	wg.Wait()

	m.applySettings(playSettings(settings.PlayDownloadPause, 1<<20, 1<<19))
	gates(t, eng, true, true, "after every game stopped")
	if m.PlayThrottle().Active {
		t.Fatal("throttle still active with no game running")
	}
}

func TestGatedDownloadIsNotMarkedStalledAndTheTimerRestartsOnRelease(t *testing.T) {
	withStallAfter(t, time.Minute)
	m := playManager(t, settings.PlayDownloadPause)
	addActive(m, "a")
	base := time.Now()
	m.sample(context.Background(), base)

	m.SetGamesRunning(true)
	m.sample(context.Background(), base.Add(10*time.Minute))
	if d := mustGet(t, m, "a"); d.Stalled || d.StalledSince != nil {
		t.Fatalf("gated download marked stalled: %+v", d)
	}

	m.SetGamesRunning(false)
	m.sample(context.Background(), base.Add(10*time.Minute+30*time.Second))
	if d := mustGet(t, m, "a"); d.Stalled {
		t.Fatal("stall timer did not restart at the release")
	}
	m.sample(context.Background(), base.Add(11*time.Minute+time.Second))
	d := mustGet(t, m, "a")
	if !d.Stalled || d.StalledSince == nil || !d.StalledSince.Equal(base.Add(10*time.Minute)) {
		t.Fatalf("stall after the release = %+v", d)
	}
}

func TestStallSetBeforeTheGameIsClearedWhileGated(t *testing.T) {
	withStallAfter(t, 0)
	m := playManager(t, settings.PlayDownloadPause)
	addActive(m, "a")
	base := time.Now()
	m.sample(context.Background(), base)
	m.sample(context.Background(), base.Add(time.Millisecond))
	if !mustGet(t, m, "a").Stalled {
		t.Fatal("precondition: stalled")
	}

	m.SetGamesRunning(true)
	m.sample(context.Background(), base.Add(2*time.Millisecond))
	if d := mustGet(t, m, "a"); d.Stalled || d.StalledSince != nil {
		t.Fatalf("stall kept while gated: %+v", d)
	}
}
