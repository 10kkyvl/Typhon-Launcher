package savebackup

import (
	"testing"
	"time"

	"typhon/internal/playlog"
)

type fakePlayLog struct {
	sessions []playlog.Session
	degraded bool
}

func (f fakePlayLog) Since(t time.Time) []playlog.Session {
	var out []playlog.Session
	for _, s := range f.sessions {
		if s.EndedAt.After(t) {
			out = append(out, s)
		}
	}
	return out
}

func (f fakePlayLog) Degraded() playlog.DegradedStatus {
	return playlog.DegradedStatus{Degraded: f.degraded}
}

func TestPlaytimeFrom(t *testing.T) {
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	hour := time.Hour
	log := fakePlayLog{sessions: []playlog.Session{
		{GameID: "g1", StartedAt: now.Add(-3 * hour), EndedAt: now.Add(-2 * hour)},
		{GameID: "g1", StartedAt: now.Add(-90 * time.Minute), EndedAt: now.Add(-30 * time.Minute)},
		{GameID: "other", StartedAt: now.Add(-3 * hour), EndedAt: now.Add(-hour)},
	}}
	cases := []struct {
		name  string
		log   fakePlayLog
		since []time.Time
		want  []Played
		fail  bool
	}{
		{"counts only this game and clips the session that straddles the moment", log,
			[]time.Time{now.Add(-4 * hour), now.Add(-2 * hour), now.Add(-hour), now},
			[]Played{{Seconds: 7200, Known: true}, {Seconds: 3600, Known: true}, {Seconds: 1800, Known: true}, {Known: true}}, false},
		{"older than the log retention is unknown", log,
			[]time.Time{now.Add(-playlog.Retention - hour), now.Add(-hour)},
			[]Played{{}, {Seconds: 1800, Known: true}}, false},
		{"degraded log knows nothing", fakePlayLog{sessions: log.sessions, degraded: true},
			[]time.Time{now.Add(-hour)}, nil, true},
		{"no moments", log, nil, []Played{}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := PlaytimeFrom(c.log, clock)("g1", c.since)

			if (err != nil) != c.fail {
				t.Fatalf("err = %v, want failure: %v", err, c.fail)
			}
			if len(got) != len(c.want) {
				t.Fatalf("got %+v, want %+v", got, c.want)
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Fatalf("got %+v, want %+v", got, c.want)
				}
			}
		})
	}
}
