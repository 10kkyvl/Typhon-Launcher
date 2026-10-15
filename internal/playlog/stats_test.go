package playlog

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"typhon/internal/storage"
	"typhon/internal/uierr"
)

func fixedNow() time.Time { return time.Date(2026, 9, 10, 15, 0, 0, 0, time.UTC) }

func newTestService(t *testing.T) *Service {
	t.Helper()
	s, err := newServiceAt(filepath.Join(t.TempDir(), "playlog.json"), fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestMigrateV1BuildsDays(t *testing.T) {
	path := filepath.Join(t.TempDir(), "playlog.json")
	sessions := []Session{
		{GameID: "a", StartedAt: time.Date(2026, 9, 8, 10, 0, 0, 0, time.UTC), EndedAt: time.Date(2026, 9, 8, 11, 0, 0, 0, time.UTC)},
		{GameID: "a", StartedAt: time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC), EndedAt: time.Date(2026, 9, 8, 14, 0, 0, 0, time.UTC)},
		{GameID: "b", StartedAt: time.Date(2026, 9, 9, 22, 0, 0, 0, time.UTC), EndedAt: time.Date(2026, 9, 10, 1, 0, 0, 0, time.UTC)},
	}
	if err := storage.Save(path, 1, sessions); err != nil {
		t.Fatal(err)
	}
	s, err := newServiceAt(path, fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Since(time.Time{})) != 3 {
		t.Fatalf("sessions lost in migration: %+v", s.sessions)
	}
	want := map[string]DayEntry{
		"2026-09-08/a": {Date: "2026-09-08", GameID: "a", Seconds: 10800, Longest: 7200, Sessions: 2},
		"2026-09-09/b": {Date: "2026-09-09", GameID: "b", Seconds: 7200, Longest: 10800, Sessions: 1},
		"2026-09-10/b": {Date: "2026-09-10", GameID: "b", Seconds: 3600},
	}
	if len(s.days) != len(want) {
		t.Fatalf("days = %+v, want %d entries", s.days, len(want))
	}
	for _, day := range s.days {
		if got := want[day.Date+"/"+day.GameID]; got != day {
			t.Fatalf("day %s/%s = %+v, want %+v", day.Date, day.GameID, day, got)
		}
	}
}

func TestMigrateV1EmptyDocument(t *testing.T) {
	path := filepath.Join(t.TempDir(), "playlog.json")
	if err := storage.Save(path, 1, []Session{}); err != nil {
		t.Fatal(err)
	}
	s, err := newServiceAt(path, fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.days) != 0 {
		t.Fatalf("days = %+v", s.days)
	}
}

func TestMigrateV1CorruptSessionsFail(t *testing.T) {
	path := filepath.Join(t.TempDir(), "playlog.json")
	if err := storage.Save(path, 1, map[string]string{"x": "y"}); err != nil {
		t.Fatal(err)
	}
	if _, err := newServiceAt(path, fixedNow); err == nil {
		t.Fatal("a v1 payload that is not a session list must fail the constructor")
	}
}

func TestRecordSplitsSessionAtMidnight(t *testing.T) {
	s := newTestService(t)
	s.Record("a", time.Date(2026, 9, 9, 23, 0, 0, 0, time.UTC), time.Date(2026, 9, 10, 2, 0, 0, 0, time.UTC))
	if len(s.days) != 2 {
		t.Fatalf("days = %+v, want two", s.days)
	}
	first, second := s.days[0], s.days[1]
	if first.Date != "2026-09-09" || first.Seconds != 3600 || first.Sessions != 1 || first.Longest != 10800 {
		t.Fatalf("first day = %+v", first)
	}
	if second.Date != "2026-09-10" || second.Seconds != 7200 || second.Sessions != 0 {
		t.Fatalf("second day = %+v", second)
	}
}

func TestRecordRollsBackDays(t *testing.T) {
	root := t.TempDir()
	s, err := newServiceAt(filepath.Join(root, "playlog.json"), fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	now := fixedNow()
	s.Record("a", now.Add(-2*time.Hour), now.Add(-time.Hour))

	blocker := filepath.Join(root, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	s.path = filepath.Join(blocker, "playlog.json")
	s.mu.Unlock()
	s.Record("a", now.Add(-4*time.Hour), now.Add(-3*time.Hour))

	if len(s.days) != 1 || s.days[0].Seconds != 3600 || s.days[0].Sessions != 1 {
		t.Fatalf("days = %+v, want rollback to the first record", s.days)
	}
}

func TestStatsPeriods(t *testing.T) {
	s := newTestService(t)
	s.SetTitleResolver(func(id string) (string, bool) {
		if id == "gone" {
			return "", false
		}
		return "Game " + id, true
	})
	now := fixedNow()
	record := func(id string, daysAgo int, hours int) {
		end := now.AddDate(0, 0, -daysAgo).Add(-time.Hour)
		s.Record(id, end.Add(-time.Duration(hours)*time.Hour), end)
	}
	record("a", 0, 2)
	record("a", 6, 1)
	record("b", 7, 4)
	record("gone", 29, 1)
	record("b", 30, 5)
	record("b", 80, 4)

	cases := []struct {
		period   string
		days     int
		total    int64
		sessions int
		topFirst string
	}{
		{"week", 7, 3 * 3600, 2, "a"},
		{"month", 30, 8 * 3600, 4, "b"},
		{"year", 365, 17 * 3600, 6, "b"},
	}
	for _, tc := range cases {
		t.Run(tc.period, func(t *testing.T) {
			got, err := s.Stats(tc.period)
			if err != nil {
				t.Fatal(err)
			}
			if len(got.Days) != tc.days || got.To != "2026-09-10" || got.Days[tc.days-1].Date != got.To || got.Days[0].Date != got.From {
				t.Fatalf("window = %s..%s, %d days", got.From, got.To, len(got.Days))
			}
			if got.TotalSeconds != tc.total || got.Sessions != tc.sessions {
				t.Fatalf("total = %d sessions = %d, want %d / %d", got.TotalSeconds, got.Sessions, tc.total, tc.sessions)
			}
			if got.Top[0].GameID != tc.topFirst || got.Top[0].Title != "Game "+tc.topFirst {
				t.Fatalf("top = %+v", got.Top)
			}
			var sum int64
			for _, d := range got.Days {
				sum += d.Seconds
			}
			if sum != got.TotalSeconds {
				t.Fatalf("days sum %d != total %d", sum, got.TotalSeconds)
			}
		})
	}

	year, err := s.Stats("year")
	if err != nil {
		t.Fatal(err)
	}
	if year.Longest == nil || year.Longest.GameID != "b" || year.Longest.Seconds != 5*3600 || year.Longest.Title != "Game b" {
		t.Fatalf("longest = %+v", year.Longest)
	}
	month, err := s.Stats("month")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, g := range month.Top {
		if g.GameID == "gone" {
			found = true
			if g.Title != "" {
				t.Fatalf("deleted game must have an empty title: %+v", g)
			}
		}
	}
	if !found {
		t.Fatalf("deleted game missing from top: %+v", month.Top)
	}
}

func TestStatsEmptyLog(t *testing.T) {
	s := newTestService(t)
	for period, days := range map[string]int{"week": 7, "month": 30, "year": 365} {
		got, err := s.Stats(period)
		if err != nil {
			t.Fatal(err)
		}
		if len(got.Days) != days || got.TotalSeconds != 0 || got.Sessions != 0 || len(got.Top) != 0 || got.Longest != nil {
			t.Fatalf("%s = %+v", period, got)
		}
	}
}

func TestStatsTopLimit(t *testing.T) {
	s := newTestService(t)
	now := fixedNow()
	for i := range 12 {
		id := string(rune('a' + i))
		s.Record(id, now.Add(-time.Duration(i+2)*time.Hour), now.Add(-time.Hour))
	}
	got, err := s.Stats("week")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Top) != topLimit || got.Top[0].GameID != "l" {
		t.Fatalf("top = %+v", got.Top)
	}
}

func TestStatsBadPeriod(t *testing.T) {
	s := newTestService(t)
	_, err := s.Stats("decade")
	if uierr.Code(err) != "playlog.bad_period" {
		t.Fatalf("err = %v", err)
	}
}

func TestDaysPrunedAfterRetention(t *testing.T) {
	path := filepath.Join(t.TempDir(), "playlog.json")
	old := fixedNow().AddDate(0, 0, -daysRetention-2).Format(dayLayout)
	fresh := fixedNow().AddDate(0, 0, -daysRetention+2).Format(dayLayout)
	err := storage.Save(path, playlogVersion, document{Days: []DayEntry{
		{Date: old, GameID: "a", Seconds: 1},
		{Date: fresh, GameID: "a", Seconds: 1},
	}})
	if err != nil {
		t.Fatal(err)
	}
	s, err := newServiceAt(path, fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.days) != 1 || s.days[0].Date != fresh {
		t.Fatalf("days = %+v", s.days)
	}
}
