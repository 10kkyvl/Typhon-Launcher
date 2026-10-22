package settings

import (
	"errors"
	"fmt"
	"slices"
	"time"

	"typhon/internal/uierr"
)

const (
	maxScheduleWindows = 32
)

var (
	ErrScheduleInvalid = uierr.New("settings.schedule_invalid", "расписание загрузок задано неверно")
)

// ScheduleWindow is a stretch of the week in which downloads may run. Days
// are time.Weekday values (0 is Sunday). A window whose To is earlier than
// From runs past midnight and belongs to the day it starts on.
type ScheduleWindow struct {
	Days []int  `json:"days"`
	From string `json:"from"`
	To   string `json:"to"`
}

func parseClock(s string) (int, bool) {
	if len(s) != 5 || s[2] != ':' {
		return 0, false
	}
	digits := [4]byte{s[0], s[1], s[3], s[4]}
	for _, c := range digits {
		if c < '0' || c > '9' {
			return 0, false
		}
	}
	h := int(digits[0]-'0')*10 + int(digits[1]-'0')
	m := int(digits[2]-'0')*10 + int(digits[3]-'0')
	if h > 23 || m > 59 {
		return 0, false
	}
	return h*60 + m, true
}

func (w ScheduleWindow) bounds() (from, to int, ok bool) {
	from, okFrom := parseClock(w.From)
	to, okTo := parseClock(w.To)
	return from, to, okFrom && okTo
}

func (w ScheduleWindow) onDay(day int) bool {
	return slices.Contains(w.Days, day)
}

// A window that does not parse never matches: sanitize refuses such a window
// before it can be stored, so only a hand-edited file could carry one.
func (w ScheduleWindow) contains(now time.Time) bool {
	from, to, ok := w.bounds()
	if !ok {
		return false
	}
	minute := now.Hour()*60 + now.Minute()
	day := int(now.Weekday())
	if from < to {
		return w.onDay(day) && minute >= from && minute < to
	}
	if minute >= from && w.onDay(day) {
		return true
	}
	return minute < to && w.onDay((day+6)%7)
}

// ScheduleAllows reports whether downloads may run at now. An empty schedule
// allows every moment.
func (s Settings) ScheduleAllows(now time.Time) bool {
	if len(s.DownloadSchedule) == 0 {
		return true
	}
	for _, w := range s.DownloadSchedule {
		if w.contains(now) {
			return true
		}
	}
	return false
}

// NextScheduleOpen returns the first start of a window after now. It reports
// false when there is no schedule or no window that can ever start.
func (s Settings) NextScheduleOpen(now time.Time) (time.Time, bool) {
	var best time.Time
	found := false
	for _, w := range s.DownloadSchedule {
		from, _, ok := w.bounds()
		if !ok {
			continue
		}
		for offset := 0; offset <= 7; offset++ {
			day := now.AddDate(0, 0, offset)
			if !w.onDay(int(day.Weekday())) {
				continue
			}
			start := time.Date(day.Year(), day.Month(), day.Day(), from/60, from%60, 0, 0, now.Location())
			if !start.After(now) {
				continue
			}
			if !found || start.Before(best) {
				best, found = start, true
			}
		}
	}
	return best, found
}

func sanitizeSchedule(s *Settings) error {
	if len(s.DownloadSchedule) > maxScheduleWindows {
		return fmt.Errorf("%w: слишком много окон (%d, не больше %d)", ErrScheduleInvalid, len(s.DownloadSchedule), maxScheduleWindows)
	}
	for i, w := range s.DownloadSchedule {
		if err := validateWindow(w); err != nil {
			return fmt.Errorf("%w: окно %d: %w", ErrScheduleInvalid, i+1, err)
		}
	}
	return nil
}

func validateWindow(w ScheduleWindow) error {
	if len(w.Days) == 0 {
		return errors.New("не выбран ни один день")
	}
	var seen [7]bool
	for _, d := range w.Days {
		if d < 0 || d > 6 {
			return fmt.Errorf("день %d вне диапазона 0-6", d)
		}
		if seen[d] {
			return fmt.Errorf("день %d указан дважды", d)
		}
		seen[d] = true
	}
	from, okFrom := parseClock(w.From)
	if !okFrom {
		return fmt.Errorf("начало %q не в формате ЧЧ:ММ", w.From)
	}
	to, okTo := parseClock(w.To)
	if !okTo {
		return fmt.Errorf("конец %q не в формате ЧЧ:ММ", w.To)
	}
	if from == to {
		return errors.New("начало и конец совпадают")
	}
	return nil
}
