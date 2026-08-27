package install

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"typhon/internal/download"
	"typhon/internal/usagestats"

	"github.com/wailsapp/wails/v3/pkg/application"
)

type usageRecorder struct {
	mu     sync.Mutex
	events []usagestats.Event
}

func (r *usageRecorder) record(ev usagestats.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, ev)
}

func (r *usageRecorder) all() []usagestats.Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]usagestats.Event(nil), r.events...)
}

func (r *usageRecorder) byType(typ string) []usagestats.Event {
	var out []usagestats.Event
	for _, ev := range r.all() {
		if ev.Type == typ {
			out = append(out, ev)
		}
	}
	return out
}

func setDownloadOrigin(d *fakeDownloads, id string, origin download.Origin) {
	d.mu.Lock()
	defer d.mu.Unlock()
	item := d.items[id]
	item.Origin = origin
	d.items[id] = item
}

func TestInstallerType(t *testing.T) {
	cases := []struct {
		name string
		in   Type
		want string
	}{
		{"portable", TypePortable, "portable"},
		{"archive zip", TypeArchiveZip, "archive_zip"},
		{"archive 7z", TypeArchive7z, "archive_7z"},
		{"archive rar", TypeArchiveRar, "archive_rar"},
		{"exe installer", TypeExeInstaller, "exe_installer"},
		{"msi installer", TypeMsiInstaller, "msi_installer"},
		{"unknown type constant", TypeUnknown, "unknown"},
		{"future unrecognized type", Type("something_new"), "unknown"},
		{"empty type", Type(""), "unknown"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := installerType(tc.in); got != tc.want {
				t.Fatalf("installerType(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestStartEmitsUsageStartedEvent(t *testing.T) {
	s, downloads, _ := newTestService(t)
	rec := &usageRecorder{}
	s.SetUsageRecorder(rec.record)

	root := t.TempDir()
	portableSource(t, root, "Game")
	downloads.add("d1", "Game", root)
	setDownloadOrigin(downloads, "d1", download.Origin{GameID: "77"})

	dest := filepath.Join(t.TempDir(), "Game")
	item, err := s.Start("d1", StartOptions{Destination: dest, Mode: ModeCopy})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	s.waitStatus(t, item.ID, StatusCompleted)

	started := rec.byType(usagestats.TypeInstallStarted)
	if len(started) != 1 {
		t.Fatalf("started events = %d, want 1: %+v", len(started), rec.all())
	}
	if got := started[0].Properties.GameID; got != "77" {
		t.Fatalf("game id = %q, want %q", got, "77")
	}
	if got := started[0].Properties.InstallerType; got != "portable" {
		t.Fatalf("installer type = %q, want %q", got, "portable")
	}
}

func TestCompleteEmitsUsageEventWithDuration(t *testing.T) {
	s, downloads, _ := newTestService(t)
	rec := &usageRecorder{}
	s.SetUsageRecorder(rec.record)

	root := t.TempDir()
	portableSource(t, root, "Game")
	downloads.add("d1", "Game", root)
	setDownloadOrigin(downloads, "d1", download.Origin{GameID: "88"})

	dest := filepath.Join(t.TempDir(), "Game")
	item, err := s.Start("d1", StartOptions{Destination: dest, Mode: ModeCopy})
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	backdated := time.Now().Add(-9 * time.Second)
	s.mu.Lock()
	if it := s.findLocked(item.ID); it != nil {
		it.StartedAt = backdated
	}
	s.mu.Unlock()

	s.waitStatus(t, item.ID, StatusCompleted)

	// StatusCompleted становится видимым до вызова recordUsage (complete()
	// сперва отпускает s.mu и эмитит eventCompleted, событие usagestats летит
	// следом в том же потоке) — ждём само событие, а не статус.
	var completed []usagestats.Event
	waitFor(t, "install_completed usage event", func() bool {
		completed = rec.byType(usagestats.TypeInstallCompleted)
		return len(completed) == 1
	})
	ev := completed[0]
	if ev.Properties.GameID != "88" {
		t.Fatalf("game id = %q, want %q", ev.Properties.GameID, "88")
	}
	if ev.Properties.InstallerType != "portable" {
		t.Fatalf("installer type = %q, want %q", ev.Properties.InstallerType, "portable")
	}
	if ev.Properties.DurationSeconds < 9 {
		t.Fatalf("duration = %d, want >= 9", ev.Properties.DurationSeconds)
	}
}

func newUsageFailService(t *testing.T) (*Service, *usageRecorder) {
	t.Helper()
	s, _, _ := newTestService(t)
	rec := &usageRecorder{}
	s.SetUsageRecorder(rec.record)
	return s, rec
}

func addRawInstallation(s *Service, item *Installation) {
	s.mu.Lock()
	s.items = append(s.items, item)
	s.mu.Unlock()
}

func TestFailDefaultBranchEmitsUsageEvent(t *testing.T) {
	s, rec := newUsageFailService(t)

	started := time.Now().Add(-11 * time.Second)
	item := &Installation{
		ID:        "fail-default",
		Type:      TypeExeInstaller,
		Origin:    download.Origin{GameID: "5"},
		StartedAt: started,
	}
	addRawInstallation(s, item)

	cause := errors.New("установщик завершился неудачно")
	s.fail(item.ID, cause)

	events := rec.byType(usagestats.TypeInstallFailed)
	if len(events) != 1 {
		t.Fatalf("failed events = %d, want 1: %+v", len(events), rec.all())
	}
	ev := events[0]
	if ev.Properties.GameID != "5" {
		t.Fatalf("game id = %q, want %q", ev.Properties.GameID, "5")
	}
	if ev.Properties.InstallerType != "exe_installer" {
		t.Fatalf("installer type = %q, want %q", ev.Properties.InstallerType, "exe_installer")
	}
	wantCode := usagestats.Classify(cause)
	if ev.Properties.ErrorCode != wantCode {
		t.Fatalf("error code = %q, want %q", ev.Properties.ErrorCode, wantCode)
	}
	if ev.Properties.DurationSeconds < 11 {
		t.Fatalf("duration = %d, want >= 11", ev.Properties.DurationSeconds)
	}

	got, ok := s.snapshot(item.ID)
	if !ok || got.Status != StatusFailed {
		t.Fatalf("status = %+v, ok=%v", got, ok)
	}
}

func TestFailInstallerNotConfirmedStoppedEmitsUsageEvent(t *testing.T) {
	s, rec := newUsageFailService(t)

	item := &Installation{
		ID:        "fail-stuck",
		Type:      TypeMsiInstaller,
		Origin:    download.Origin{GameID: "6"},
		StartedAt: time.Now().Add(-3 * time.Second),
	}
	addRawInstallation(s, item)

	s.fail(item.ID, errInstallerNotConfirmedStopped)

	events := rec.byType(usagestats.TypeInstallFailed)
	if len(events) != 1 {
		t.Fatalf("failed events = %d, want 1: %+v", len(events), rec.all())
	}
	if got := events[0].Properties.InstallerType; got != "msi_installer" {
		t.Fatalf("installer type = %q, want %q", got, "msi_installer")
	}
	wantCode := usagestats.Classify(errInstallerNotConfirmedStopped)
	if got := events[0].Properties.ErrorCode; got != wantCode {
		t.Fatalf("error code = %q, want %q", got, wantCode)
	}
}

func TestFailUserCancelSkipsUsageEvent(t *testing.T) {
	s, rec := newUsageFailService(t)

	item := &Installation{
		ID:        "fail-cancel",
		Type:      TypeArchiveZip,
		Status:    StatusExtracting,
		Origin:    download.Origin{GameID: "7"},
		StartedAt: time.Now(),
	}
	addRawInstallation(s, item)
	s.mu.Lock()
	s.jobs[item.ID] = &job{cancel: func() {}, cancelled: true}
	s.mu.Unlock()

	s.fail(item.ID, context.Canceled)

	if events := rec.byType(usagestats.TypeInstallFailed); len(events) != 0 {
		t.Fatalf("failed events = %+v, want none for user cancel", events)
	}
	got, ok := s.snapshot(item.ID)
	if !ok || got.Status != StatusCancelled {
		t.Fatalf("status = %+v, ok=%v, want cancelled", got, ok)
	}
}

func TestFailClosingSkipsUsageEvent(t *testing.T) {
	s, rec := newUsageFailService(t)

	item := &Installation{
		ID:        "fail-closing",
		Type:      TypeArchiveZip,
		Origin:    download.Origin{GameID: "8"},
		StartedAt: time.Now(),
	}
	addRawInstallation(s, item)
	s.mu.Lock()
	s.closing = true
	s.mu.Unlock()

	s.fail(item.ID, context.Canceled)

	if events := rec.byType(usagestats.TypeInstallFailed); len(events) != 0 {
		t.Fatalf("failed events = %+v, want none while launcher is closing", events)
	}
}

func TestUsageRecorderNilByDefault(t *testing.T) {
	s := mustServiceAt(t, t.TempDir())
	if err := s.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatalf("startup: %v", err)
	}
	defer func() {
		if err := s.ServiceShutdown(); err != nil {
			t.Errorf("shutdown: %v", err)
		}
	}()

	item := &Installation{
		ID:        "no-recorder",
		Type:      TypeExeInstaller,
		Origin:    download.Origin{GameID: "9"},
		StartedAt: time.Now(),
	}
	addRawInstallation(s, item)

	s.fail(item.ID, errors.New("boom"))

	got, ok := s.snapshot(item.ID)
	if !ok || got.Status != StatusFailed {
		t.Fatalf("status = %+v, ok=%v", got, ok)
	}
}
