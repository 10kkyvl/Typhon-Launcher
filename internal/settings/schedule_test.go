package settings

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

// at returns a moment on the given weekday (0 is Sunday) of the week that
// starts on 2026-10-18, a Sunday.
func at(t *testing.T, day, hour, minute int) time.Time {
	t.Helper()
	got := time.Date(2026, time.October, 18+day, hour, minute, 0, 0, time.UTC)
	if int(got.Weekday()) != day%7 {
		t.Fatalf("2026-10-%d is %s, the helper expects weekday %d", 18+day, got.Weekday(), day%7)
	}
	return got
}

func withSchedule(w ...ScheduleWindow) Settings {
	s := Defaults()
	s.DownloadSchedule = w
	return s
}

func TestScheduleAllows(t *testing.T) {
	night := ScheduleWindow{Days: []int{5}, From: "23:00", To: "08:00"}
	work := ScheduleWindow{Days: []int{1, 2, 3, 4, 5}, From: "09:00", To: "17:30"}
	cases := []struct {
		name string
		s    Settings
		day  int
		h, m int
		want bool
	}{
		{"no schedule at all", Defaults(), 3, 4, 5, true},
		{"empty list", withSchedule(), 3, 4, 5, true},
		{"inside a same-day window", withSchedule(work), 3, 12, 0, true},
		{"before a same-day window", withSchedule(work), 3, 8, 59, false},
		{"after a same-day window", withSchedule(work), 3, 17, 30, false},
		{"day not listed", withSchedule(work), 0, 12, 0, false},
		{"From is inclusive", withSchedule(work), 3, 9, 0, true},
		{"To is exclusive", withSchedule(work), 3, 17, 30, false},
		{"last minute before To", withSchedule(work), 3, 17, 29, true},
		{"overnight, evening part", withSchedule(night), 5, 23, 30, true},
		{"overnight, From is inclusive", withSchedule(night), 5, 23, 0, true},
		{"overnight, minute before From", withSchedule(night), 5, 22, 59, false},
		{"overnight, after midnight belongs to the starting day", withSchedule(night), 6, 3, 0, true},
		{"overnight, To is exclusive", withSchedule(night), 6, 8, 0, false},
		{"overnight, last minute before To", withSchedule(night), 6, 7, 59, true},
		{"overnight, the next day is not listed so the evening is closed", withSchedule(night), 6, 23, 30, false},
		{"overnight, early morning of the starting day is closed", withSchedule(night), 5, 3, 0, false},
		{"overnight across the week end", withSchedule(ScheduleWindow{Days: []int{6}, From: "22:00", To: "02:00"}), 7, 1, 0, true},
		{"overnight ending at midnight has no tail", withSchedule(ScheduleWindow{Days: []int{2}, From: "22:00", To: "00:00"}), 3, 0, 0, false},
		{"two windows, first matches", withSchedule(night, work), 3, 10, 0, true},
		{"two windows, second matches", withSchedule(work, night), 5, 23, 10, true},
		{"two windows, none matches", withSchedule(night, work), 3, 20, 0, false},
		{"window that does not parse never matches", withSchedule(ScheduleWindow{Days: []int{3}, From: "9:00", To: "17:00"}), 3, 12, 0, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.s.ScheduleAllows(at(t, c.day, c.h, c.m)); got != c.want {
				t.Fatalf("ScheduleAllows = %v, want %v", got, c.want)
			}
		})
	}
}

func TestNextScheduleOpen(t *testing.T) {
	work := ScheduleWindow{Days: []int{1, 3}, From: "09:00", To: "17:00"}
	cases := []struct {
		name   string
		s      Settings
		now    time.Time
		want   time.Time
		wantOK bool
	}{
		{"no schedule", Defaults(), at(t, 3, 12, 0), time.Time{}, false},
		{"later the same day", withSchedule(work), at(t, 1, 7, 0), at(t, 1, 9, 0), true},
		{"the window start is not after itself", withSchedule(work), at(t, 1, 9, 0), at(t, 3, 9, 0), true},
		{"after the last window of the day", withSchedule(work), at(t, 1, 18, 0), at(t, 3, 9, 0), true},
		{"a week away", withSchedule(ScheduleWindow{Days: []int{3}, From: "09:00", To: "10:00"}), at(t, 3, 10, 0), at(t, 3, 9, 0).AddDate(0, 0, 7), true},
		{"the nearest of several windows", withSchedule(work, ScheduleWindow{Days: []int{2}, From: "06:00", To: "07:00"}), at(t, 1, 18, 0), at(t, 2, 6, 0), true},
		{"overnight window starts in the evening", withSchedule(ScheduleWindow{Days: []int{5}, From: "23:00", To: "08:00"}), at(t, 6, 3, 0), at(t, 5, 23, 0).AddDate(0, 0, 7), true},
		{"only windows that do not parse", withSchedule(ScheduleWindow{Days: []int{3}, From: "x", To: "17:00"}), at(t, 3, 12, 0), time.Time{}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := c.s.NextScheduleOpen(c.now)
			if ok != c.wantOK {
				t.Fatalf("ok = %v, want %v", ok, c.wantOK)
			}
			if !got.Equal(c.want) {
				t.Fatalf("next open = %v, want %v", got, c.want)
			}
		})
	}
}

func TestScheduleValidation(t *testing.T) {
	good := ScheduleWindow{Days: []int{1}, From: "09:00", To: "17:00"}
	cases := []struct {
		name string
		s    Settings
		ok   bool
	}{
		{"none", Defaults(), true},
		{"one window", withSchedule(good), true},
		{"overnight", withSchedule(ScheduleWindow{Days: []int{0, 6}, From: "23:59", To: "00:00"}), true},
		{"no days", withSchedule(ScheduleWindow{From: "09:00", To: "17:00"}), false},
		{"day below range", withSchedule(ScheduleWindow{Days: []int{-1}, From: "09:00", To: "17:00"}), false},
		{"day above range", withSchedule(ScheduleWindow{Days: []int{7}, From: "09:00", To: "17:00"}), false},
		{"repeated day", withSchedule(ScheduleWindow{Days: []int{2, 2}, From: "09:00", To: "17:00"}), false},
		{"hour 24", withSchedule(ScheduleWindow{Days: []int{1}, From: "24:00", To: "17:00"}), false},
		{"minute 60", withSchedule(ScheduleWindow{Days: []int{1}, From: "09:00", To: "17:60"}), false},
		{"no leading zero", withSchedule(ScheduleWindow{Days: []int{1}, From: "9:00", To: "17:00"}), false},
		{"seconds", withSchedule(ScheduleWindow{Days: []int{1}, From: "09:00:00", To: "17:00"}), false},
		{"wrong separator", withSchedule(ScheduleWindow{Days: []int{1}, From: "09.00", To: "17:00"}), false},
		{"sign inside", withSchedule(ScheduleWindow{Days: []int{1}, From: "+9:00", To: "17:00"}), false},
		{"empty time", withSchedule(ScheduleWindow{Days: []int{1}, From: "", To: "17:00"}), false},
		{"From equals To", withSchedule(ScheduleWindow{Days: []int{1}, From: "09:00", To: "09:00"}), false},
		{"one bad window among good ones", withSchedule(good, ScheduleWindow{Days: []int{9}, From: "09:00", To: "10:00"}), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := sanitize(c.s)
			if c.ok && err != nil {
				t.Fatalf("error = %v, want none", err)
			}
			if !c.ok && !errors.Is(err, ErrScheduleInvalid) {
				t.Fatalf("error = %v, want ErrScheduleInvalid", err)
			}
		})
	}
}

func TestScheduleValidationRefusesTooManyWindows(t *testing.T) {
	windows := make([]ScheduleWindow, maxScheduleWindows+1)
	for i := range windows {
		windows[i] = ScheduleWindow{Days: []int{1}, From: "09:00", To: "10:00"}
	}
	if _, err := sanitize(withSchedule(windows...)); !errors.Is(err, ErrScheduleInvalid) {
		t.Fatalf("error = %v, want ErrScheduleInvalid", err)
	}
	if _, err := sanitize(withSchedule(windows[:maxScheduleWindows]...)); err != nil {
		t.Fatalf("the limit itself must pass: %v", err)
	}
}

func TestSchedulePatchRefusesABadWindowAndKeepsTheOldOne(t *testing.T) {
	svc := mustServiceAt(t, filepath.Join(t.TempDir(), "settings.json"))
	good := json.RawMessage(`[{"days":[1,2],"from":"22:00","to":"06:00"}]`)
	if _, err := svc.SaveSettingsPatch(map[string]json.RawMessage{"downloadSchedule": good}); err != nil {
		t.Fatal(err)
	}
	bad := json.RawMessage(`[{"days":[1],"from":"22:00","to":"22:00"}]`)
	if _, err := svc.SaveSettingsPatch(map[string]json.RawMessage{"downloadSchedule": bad}); !errors.Is(err, ErrScheduleInvalid) {
		t.Fatalf("error = %v, want ErrScheduleInvalid", err)
	}
	got := svc.GetSettings().DownloadSchedule
	if len(got) != 1 || got[0].From != "22:00" || got[0].To != "06:00" {
		t.Fatalf("schedule after a refused save = %+v, want the earlier one", got)
	}
}
