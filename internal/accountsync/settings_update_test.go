package accountsync

import (
	"errors"
	"net/http"
	"path/filepath"
	"sync"
	"testing"

	"typhon/internal/settings"

	"github.com/wailsapp/wails/v3/pkg/application"
)

type liveSettings struct{ svc *settings.Service }

func (l liveSettings) Get() settings.Settings { return l.svc.GetSettings() }

func (l liveSettings) Update(mutate func(*settings.Settings) error) (settings.Settings, error) {
	return l.svc.Update(mutate)
}

func newLiveSettings(t *testing.T) liveSettings {
	t.Helper()
	svc, err := settings.NewServiceAt(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("new settings: %v", err)
	}
	if _, err := svc.Update(func(next *settings.Settings) error {
		next.AccountSync = true
		return nil
	}); err != nil {
		t.Fatalf("enable account sync: %v", err)
	}
	return liveSettings{svc}
}

func holdSettingsWrite(t *testing.T, svc *settings.Service, match func(prev, next settings.Settings) bool) (reached <-chan struct{}, release func()) {
	t.Helper()
	in := make(chan struct{})
	out := make(chan struct{})
	var once, free sync.Once
	release = func() { free.Do(func() { close(out) }) }
	t.Cleanup(release)
	if err := svc.AddApplier(func(prev, next settings.Settings) error {
		if match(prev, next) {
			once.Do(func() {
				close(in)
				<-out
			})
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return in, release
}

func saveUnrelatedSetting(t *testing.T, svc *settings.Service) {
	t.Helper()
	other := svc.GetSettings()
	other.MaxActiveDownloads = 7
	other.LANSharing = true
	if err := svc.SaveSettings(other); err != nil {
		t.Fatalf("concurrent save: %v", err)
	}
}

func requireUnrelatedSettingKept(t *testing.T, svc *settings.Service) {
	t.Helper()
	got := svc.GetSettings()
	if got.MaxActiveDownloads != 7 || !got.LANSharing {
		t.Errorf("downloads %d, lan %v: the sync write put back a stale copy over a newer save", got.MaxActiveDownloads, got.LANSharing)
	}
}

func startedHarnessWith(t *testing.T, port SettingsPort) *harness {
	t.Helper()
	h := newHarness(t)
	h.service.settings = port
	if err := h.service.ServiceStartup(t.Context(), application.ServiceOptions{}); err != nil {
		t.Fatalf("ServiceStartup: %v", err)
	}
	t.Cleanup(func() {
		if err := h.service.ServiceShutdown(); err != nil {
			t.Fatalf("ServiceShutdown: %v", err)
		}
	})
	return h
}

func TestApplyRemoteSettingsKeepsASettingSavedWhileItWasInFlight(t *testing.T) {
	live := newLiveSettings(t)
	h := newHarness(t)
	h.service.settings = live
	reached, release := holdSettingsWrite(t, live.svc, func(_, next settings.Settings) bool { return next.Theme == "light" })

	light := "light"
	done := make(chan error, 1)
	go func() { done <- h.service.applyRemoteSettings(settings.Portable{Theme: &light}) }()
	<-reached
	saveUnrelatedSetting(t, live.svc)
	release()
	if err := <-done; err != nil {
		t.Fatalf("applyRemoteSettings: %v", err)
	}

	if got := live.svc.GetSettings().Theme; got != "light" {
		t.Errorf("theme = %q, want the remote value", got)
	}
	requireUnrelatedSettingKept(t, live.svc)
}

func TestForgetRemoteKeepsASettingSavedWhileTheToggleIsCleared(t *testing.T) {
	live := newLiveSettings(t)
	h := startedHarnessWith(t, live)
	seedState(t, h)
	reached, release := holdSettingsWrite(t, live.svc, func(prev, next settings.Settings) bool { return prev.AccountSync && !next.AccountSync })

	done := make(chan error, 1)
	go func() { done <- h.service.ForgetRemote() }()
	<-reached
	saveUnrelatedSetting(t, live.svc)
	release()
	if err := <-done; err != nil {
		t.Fatalf("ForgetRemote: %v", err)
	}

	if live.svc.GetSettings().AccountSync {
		t.Error("account sync stayed on after the wipe")
	}
	requireUnrelatedSettingKept(t, live.svc)
}

func TestForgetRemoteReportsAFailedSettingsWrite(t *testing.T) {
	h := startedHarness(t)
	seedState(t, h)
	diskFull := errors.New("disk full")
	h.settings.mu.Lock()
	h.settings.saveErr = diskFull
	h.settings.mu.Unlock()

	err := h.service.ForgetRemote()
	if !errors.Is(err, diskFull) {
		t.Fatalf("ForgetRemote = %v, want the settings write error", err)
	}
}

func TestForgetRemoteLeavesSettingsAloneWhenSyncIsAlreadyOff(t *testing.T) {
	h := startedHarness(t)
	seedState(t, h)
	h.settings.mu.Lock()
	h.settings.value.AccountSync = false
	h.settings.saveErr = errors.New("disk full")
	h.settings.mu.Unlock()

	if err := h.service.ForgetRemote(); err != nil {
		t.Fatalf("ForgetRemote = %v, want nil: there was nothing to switch off", err)
	}
	h.settings.mu.Lock()
	defer h.settings.mu.Unlock()
	if h.settings.saveCalls != 0 {
		t.Fatalf("settings written %d times, want 0", h.settings.saveCalls)
	}
}

func TestSyncReportsAFailedSettingsWrite(t *testing.T) {
	h := newHarness(t)
	diskFull := errors.New("disk full")
	h.settings.saveErr = diskFull
	light := "light"
	h.server.get = func(w http.ResponseWriter) {
		writeJSON(w, http.StatusOK, snapshotBody{
			SettingsRevision: 7,
			Settings:         settings.Portable{Theme: &light},
			Games:            []wireGame{},
		})
	}
	h.server.put = echoPut(http.StatusOK)

	err := h.service.Sync(t.Context())
	if !errors.Is(err, diskFull) {
		t.Fatalf("Sync = %v, want the settings write error", err)
	}
	if h.settings.value.Theme == "light" {
		t.Fatal("remote theme applied although the write failed")
	}
}
