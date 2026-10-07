package download

import (
	"errors"
	"math"
	"testing"
	"time"

	"typhon/internal/settings"

	"golang.org/x/time/rate"
)

func TestMaxActiveIsClamped(t *testing.T) {
	cases := []struct{ in, want int }{
		{-5, 1}, {0, 1}, {1, 1}, {4, 4}, {10, 10}, {11, 10}, {99, 10},
	}
	for _, c := range cases {
		cfg := settings.Defaults()
		cfg.MaxActiveDownloads = c.in
		if got := maxActive(cfg); got != c.want {
			t.Errorf("maxActive(%d) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestChangingTheActiveLimitMovesTheQueueButStopsNothing(t *testing.T) {
	m := newTestManager(t, 1)
	for _, id := range []string{"a", "b", "c", "d"} {
		m.addTestDownload(id)
	}
	assertStatuses(t, m, map[string]Status{"a": StatusDownloading, "b": StatusQueued, "c": StatusQueued, "d": StatusQueued})

	next := settings.Defaults()
	next.MaxActiveDownloads = 3
	m.applySettings(next)
	assertStatuses(t, m, map[string]Status{"a": StatusDownloading, "b": StatusDownloading, "c": StatusDownloading, "d": StatusQueued})

	next.MaxActiveDownloads = 1
	m.applySettings(next)
	assertStatuses(t, m, map[string]Status{"a": StatusDownloading, "b": StatusDownloading, "c": StatusDownloading, "d": StatusQueued})

	if err := m.Pause("a"); err != nil {
		t.Fatal(err)
	}
	if err := m.Pause("b"); err != nil {
		t.Fatal(err)
	}
	assertStatuses(t, m, map[string]Status{"c": StatusDownloading, "d": StatusQueued})
	if err := m.Pause("c"); err != nil {
		t.Fatal(err)
	}
	assertStatuses(t, m, map[string]Status{"d": StatusDownloading})
}

func TestRateLimitChangesReachTheLiveClient(t *testing.T) {
	cases := []struct {
		name          string
		down, up      int64
		wantDown      rate.Limit
		wantUp        rate.Limit
		wantDownBurst int
		wantUpBurst   int
	}{
		{"limits set", 1 << 20, 2 << 20, rate.Limit(1 << 20), rate.Limit(2 << 20), 1 << 20, 2 << 20},
		{"a low limit still gets a usable burst", 1000, 1000, 1000, 1000, minLimiterBurst, minLimiterBurst},
		{"zero means unlimited", 0, 0, rate.Inf, rate.Inf, math.MaxInt, math.MaxInt},
		{"negative means unlimited", -1, -1, rate.Inf, rate.Inf, math.MaxInt, math.MaxInt},
		{"only one direction limited", 0, 5 << 20, rate.Inf, rate.Limit(5 << 20), math.MaxInt, 5 << 20},
	}
	m := newTestManager(t, 1)
	m.client = offlineClient(t)
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			next := settings.Defaults()
			next.DownloadRateLimit, next.UploadRateLimit = c.down, c.up

			m.applySettings(next)

			if got := m.client.down.Limit(); got != c.wantDown {
				t.Errorf("download limit = %v, want %v", got, c.wantDown)
			}
			if got := m.client.up.Limit(); got != c.wantUp {
				t.Errorf("upload limit = %v, want %v", got, c.wantUp)
			}
			if got := m.client.down.Burst(); got != c.wantDownBurst {
				t.Errorf("download burst = %d, want %d", got, c.wantDownBurst)
			}
			if got := m.client.up.Burst(); got != c.wantUpBurst {
				t.Errorf("upload burst = %d, want %d", got, c.wantUpBurst)
			}
		})
	}
}

func TestFetchMetadataThatEndsWithoutMetadataLeavesNothingBehind(t *testing.T) {
	const hash = "a748597437835a2fd0d2e06f8edd86fee316a84d"
	source := "magnet:?xt=urn:btih:" + hash

	cases := []struct {
		name string
		end  func(t *testing.T, m *Manager) error
	}{
		{"the wait runs out", func(t *testing.T, m *Manager) error {
			old := metadataTimeout
			metadataTimeout = 10 * time.Millisecond
			t.Cleanup(func() { metadataTimeout = old })
			_, err := m.FetchMetadata(source)
			return err
		}},
		{"the window is closed", func(t *testing.T, m *Manager) error {
			result := make(chan error, 1)
			go func() {
				_, err := m.FetchMetadata(source)
				result <- err
			}()
			waitUntil(t, "the fetch to reserve the hash", func() bool {
				m.mu.Lock()
				defer m.mu.Unlock()
				return m.reserved[hash]
			})
			m.CancelFetchMetadata(source)
			return <-result
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := managerWithClient(t, 1)

			err := c.end(t, m)

			if !errors.Is(err, errNoMetadata) {
				t.Fatalf("error = %v, want errNoMetadata", err)
			}
			if got := len(m.client.cl.Torrents()); got != 0 {
				t.Fatalf("torrents left in the client: %d", got)
			}
			if pending, reserved := m.bookkeeping(); pending != 0 || reserved != 0 {
				t.Fatalf("pending = %d, reserved = %d", pending, reserved)
			}
			m.mu.Lock()
			inFlight := len(m.fetching)
			m.mu.Unlock()
			if inFlight != 0 {
				t.Fatalf("fetches still registered as in flight: %d", inFlight)
			}
			if m.store.hasMetainfo(hash) {
				t.Fatal("metadata that never arrived was cached")
			}
		})
	}
}
