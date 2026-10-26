package sources

import (
	"fmt"
	"testing"
	"time"

	"typhon/internal/catalog"
)

func TestRemoteMatchLifetimeDependsOnWhetherTheGameWasFound(t *testing.T) {
	cases := []struct {
		name   string
		gameID string
		min    time.Duration
		max    time.Duration
	}{
		{"found game", "game-1", 7 * 24 * time.Hour, 8 * 24 * time.Hour},
		{"no game", "", 24 * time.Hour, 36 * time.Hour},
	}
	at := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for i := range 200 {
				key := fmt.Sprintf("title-%d|2020", i)
				rm := &RemoteMatch{Key: key, At: at, GameID: tc.gameID, Rev: remoteMatchRev}
				life := remoteMatchLifetime(rm, key)
				if life < tc.min || life >= tc.max {
					t.Fatalf("%s lifetime = %v, want [%v, %v)", key, life, tc.min, tc.max)
				}
				if !remoteMatchFresh(rm, key, at.Add(life-time.Second)) {
					t.Fatalf("%s must still be fresh just before %v", key, life)
				}
				if remoteMatchFresh(rm, key, at.Add(life)) {
					t.Fatalf("%s must be stale at %v", key, life)
				}
			}
		})
	}
}

func TestRemoteMatchChangedKeyIsNeverFresh(t *testing.T) {
	at := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	rm := &RemoteMatch{Key: "old title|0", At: at, GameID: "game-1", Rev: remoteMatchRev}
	if remoteMatchFresh(rm, "new title|0", at.Add(time.Minute)) {
		t.Fatal("a renamed release must be asked again at once")
	}
	if remoteMatchFresh(nil, "new title|0", at) {
		t.Fatal("no cached answer is never fresh")
	}
	if remoteMatchFresh(rm, "old title|0", at.Add(-time.Minute)) {
		t.Fatal("an answer from the future (clock set back) is not fresh")
	}
}

// Walks a fake clock through a 6-hour refresh cycle (the settings default)
// and records when each key would go to the server.
func TestRemoteMatchAskIntervalAtSixHourCycles(t *testing.T) {
	const (
		cycle = 6 * time.Hour
		span  = 60 * 24 * time.Hour
	)
	cases := []struct {
		name     string
		gameID   string
		minGap   time.Duration
		maxGap   time.Duration
		maxAsked int
	}{
		{"found game is asked about once a week", "game-1", 7 * 24 * time.Hour, 8*24*time.Hour + cycle, 9},
		{"unknown title is asked about daily", "", 24 * time.Hour, 36*time.Hour + cycle, 60},
	}
	start := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for i := range 300 {
				key := fmt.Sprintf("game-%d|%d", i, 2000+i%25)
				var rm *RemoteMatch
				var asked []time.Time
				for now := start; now.Before(start.Add(span)); now = now.Add(cycle) {
					if remoteMatchFresh(rm, key, now) {
						continue
					}
					rm = &RemoteMatch{Key: key, At: now, GameID: tc.gameID, Rev: remoteMatchRev}
					asked = append(asked, now)
				}
				if len(asked) > tc.maxAsked {
					t.Fatalf("%s asked %d times in %v, want at most %d", key, len(asked), span, tc.maxAsked)
				}
				for j := 1; j < len(asked); j++ {
					if gap := asked[j].Sub(asked[j-1]); gap < tc.minGap || gap > tc.maxGap {
						t.Fatalf("%s asked again after %v, want between %v and %v", key, gap, tc.minGap, tc.maxGap)
					}
				}
			}
		})
	}
}

func TestMatchedReleaseIsNotAskedAgainWithinAWeek(t *testing.T) {
	cases := []struct {
		name      string
		known     bool
		age       time.Duration
		wantQuery int
	}{
		{"found game three days old", true, 3 * 24 * time.Hour, 1},
		{"found game six days old", true, 6 * 24 * time.Hour, 1},
		{"found game past the longest lifetime", true, remoteMatchKnownTTL + remoteMatchKnownJitterMax + time.Hour, 2},
		{"unknown title two days old", false, 2 * 24 * time.Hour, 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, cat, _ := testService(t)
			server := newFeedServer(t, feedBody(t, "Source", feedEntry{DistributionID: "a", Title: "Alpha Game v1.0", URIs: []string{magnetOf("aa")}}))
			src := addSource(t, s, server.url())
			remote := &cachingTestRemote{games: map[string]catalog.Game{}}
			if tc.known {
				remote.games["Alpha Game"] = catalog.Game{ID: "server-alpha", Title: "Alpha Game", ExternalIDs: catalog.ExternalIDs{IGDB: "1"}}
			}
			cat.SetRemoteCatalog(remote)
			if _, err := s.RefreshSource(src.ID); err != nil {
				t.Fatalf("first refresh: %v", err)
			}
			if n := remote.queryCount(); n != 1 {
				t.Fatalf("first refresh sent %d queries, want 1", n)
			}

			s.mu.Lock()
			for _, r := range s.releases[src.ID] {
				if r.RemoteMatch == nil {
					s.mu.Unlock()
					t.Fatal("release has no cached answer to age")
				}
				r.RemoteMatch.At = time.Now().Add(-tc.age)
			}
			s.mu.Unlock()
			if _, err := s.RefreshSource(src.ID); err != nil {
				t.Fatalf("second refresh: %v", err)
			}
			if n := remote.queryCount(); n != tc.wantQuery {
				t.Fatalf("cumulative queries = %d, want %d", n, tc.wantQuery)
			}
		})
	}
}
