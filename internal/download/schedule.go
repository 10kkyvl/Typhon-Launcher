package download

import (
	"fmt"
	"log/slog"
	"time"
)

func (m *Manager) schedulePausedLocked(d *Download) bool {
	return m.scheduleBlocked && !d.IgnoreSchedule
}

// pausedLocked is the one question every gate asks: the game and the schedule
// each hold a download back, and a download runs only when neither does.
func (m *Manager) pausedLocked(d *Download) bool {
	return m.playPausedLocked() || m.schedulePausedLocked(d)
}

// scheduleTime is the moment the schedule is judged at. A test sets clock; in
// production the tick's own time is used.
func (m *Manager) scheduleTime(tick time.Time) time.Time {
	if m.clock != nil {
		return m.clock()
	}
	return tick
}

// refreshScheduleLocked recomputes whether the schedule holds downloads back
// and reports whether that changed.
func (m *Manager) refreshScheduleLocked(now time.Time) bool {
	m.scheduleMinute = now.Truncate(time.Minute)
	blocked := !m.rateCfg.ScheduleAllows(now)
	if blocked == m.scheduleBlocked {
		return false
	}
	m.scheduleBlocked = blocked
	if blocked {
		next, _ := m.rateCfg.NextScheduleOpen(now)
		slog.Info("downloads held by the schedule", "next_window", next.Format(time.RFC3339))
	} else {
		slog.Info("schedule window open, downloads released")
	}
	return true
}

func (m *Manager) syncScheduleLocked(now time.Time) {
	if m.refreshScheduleLocked(now) {
		m.syncPlayLocked()
		m.schedule()
	}
}

// sampleScheduleLocked runs on every tick but works once a minute: windows
// have minute resolution.
func (m *Manager) sampleScheduleLocked(tick time.Time) {
	now := m.scheduleTime(tick)
	if now.Truncate(time.Minute).Equal(m.scheduleMinute) {
		return
	}
	m.syncScheduleLocked(now)
}

// DownloadNow lets one download run outside the schedule windows until it is
// finished.
func (m *Manager) DownloadNow(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	d := m.findLocked(id)
	if d == nil {
		return errNotFound
	}
	if d.Status == StatusCompleted {
		return errUnavailable
	}
	if d.IgnoreSchedule {
		return nil
	}
	d.IgnoreSchedule = true
	if err := m.persistLocked(); err != nil {
		d.IgnoreSchedule = false
		return fmt.Errorf("скачать сейчас: %w", err)
	}
	m.syncPlayLocked()
	m.schedule()
	emit(eventUpdated, snapshot(d))
	return nil
}
