package install

import (
	"errors"
	"path/filepath"
	"sync"
	"testing"

	"typhon/internal/download"
	"typhon/internal/history"
)

type historyLog struct {
	mu   sync.Mutex
	recs []history.Record
	err  error
}

func (h *historyLog) record(rec history.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.recs = append(h.recs, rec)
	return h.err
}

func (h *historyLog) all() []history.Record {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]history.Record(nil), h.recs...)
}

func (r *rig) journal(err error) *historyLog {
	log := &historyLog{err: err}
	r.s.SetHistoryRecorder(log.record)
	return log
}

func TestHistoryRecordsAFinishedInstall(t *testing.T) {
	r := newRig(t)
	log := r.journal(nil)
	root := t.TempDir()
	portableSource(t, root, "Game")
	r.download("d1", "Game", root)
	r.patchDownload("d1", func(d *download.Download) { d.Origin.GameID, d.Origin.Version = "canon-1", "1.2.3" })
	item, err := r.s.Start("d1", StartOptions{Destination: filepath.Join(t.TempDir(), "Game"), Mode: ModeCopy})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	got := r.settle(item.ID)
	if got.Status != StatusCompleted {
		t.Fatalf("status = %s (%q)", got.Status, got.Error)
	}

	recs := log.all()
	if len(recs) != 1 {
		t.Fatalf("history records = %+v, want exactly one", recs)
	}
	rec := recs[0]
	if rec.Kind != history.KindInstalled || rec.GameID != got.GameID || rec.Title != "Game" || rec.ToVersion != "1.2.3" || rec.RefID != item.ID {
		t.Fatalf("history record = %+v", rec)
	}
}

func TestHistoryRecordsAFailedInstallWithItsReason(t *testing.T) {
	r := newRig(t)
	log := r.journal(nil)
	dest := silentSource(t, r, rgInnoMarker)
	r.patchDownload("d1", func(d *download.Download) { d.Origin.GameID = "canon-1" })
	r.setRunner(newRgRunner(t, rgStep{code: 1}))
	item, err := r.s.Start("d1", StartOptions{Destination: dest})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	got := r.settle(item.ID)
	if got.Status != StatusFailed {
		t.Fatalf("status = %s", got.Status)
	}

	recs := log.all()
	if len(recs) != 1 {
		t.Fatalf("history records = %+v, want exactly one", recs)
	}
	rec := recs[0]
	if rec.Kind != history.KindInstallFailed || rec.GameID != "canon-1" || rec.Title != "Game" || rec.RefID != item.ID {
		t.Fatalf("history record = %+v", rec)
	}
	if rec.Detail != got.Error || uiCode(rec.Detail) != "install.installer_failed" {
		t.Fatalf("history detail = %q, want the error the player saw (%q)", rec.Detail, got.Error)
	}
}

func TestHistoryIgnoresWhatThePlayerCancelledOrWhatWasInterrupted(t *testing.T) {
	r := newRig(t)
	log := r.journal(nil)
	dest := silentSource(t, r, rgInnoMarker)
	run := newRgRunner(t, rgStep{wait: true})
	r.setRunner(run)
	item, err := r.s.Start("d1", StartOptions{Destination: dest})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	run.entered(0)
	if err := r.s.Cancel(item.ID); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if got := r.settle(item.ID); got.Status != StatusCancelled {
		t.Fatalf("status = %s", got.Status)
	}
	if recs := log.all(); len(recs) != 0 {
		t.Fatalf("a cancelled install was journaled: %+v", recs)
	}
}

// Журнал вторичен: его отказ не превращает удавшуюся установку в неудачную и не
// прячет настоящую причину неудачи.
func TestHistoryFailureDoesNotChangeTheOutcome(t *testing.T) {
	cases := []struct {
		name string
		code int
		want Status
	}{
		{"completed install", 0, StatusCompleted},
		{"failed install", 1, StatusFailed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newRig(t)
			log := r.journal(errors.New("journal is read-only"))
			dest := silentSource(t, r, rgInnoMarker)
			r.setRunner(newRgRunner(t, rgStep{act: installGame(t, dest), code: tc.code}))
			item, err := r.s.Start("d1", StartOptions{Destination: dest})
			if err != nil {
				t.Fatalf("Start: %v", err)
			}
			got := r.settle(item.ID)
			if got.Status != tc.want {
				t.Fatalf("status = %s (%q), want %s", got.Status, got.Error, tc.want)
			}
			if tc.want == StatusCompleted && got.Error != "" {
				t.Fatalf("a journal error leaked into a completed install: %q", got.Error)
			}
			if tc.want == StatusFailed && uiCode(got.Error) != "install.installer_failed" {
				t.Fatalf("a journal error replaced the reason: %q", got.Error)
			}
			if n := len(log.all()); n != 1 {
				t.Fatalf("journal calls = %d, want 1", n)
			}
		})
	}
}
