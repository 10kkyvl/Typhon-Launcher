package settings

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
)

func TestUpdateKeepsASettingSavedWhileItWasInFlight(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	svc := mustServiceAt(t, path)
	fx := addEffectAppliers(t, svc, svc.GetSettings())
	reached, release := gateFirst(t, svc, func(_, next Settings) bool { return next.LaunchOnStartup })

	done := make(chan error, 1)
	var saved Settings
	go func() {
		var err error
		saved, err = svc.Update(func(s *Settings) error {
			s.LaunchOnStartup = true
			return nil
		})
		done <- err
	}()
	<-reached

	other := svc.GetSettings()
	other.MinimizeToTray = false
	if err := svc.SaveSettings(other); err != nil {
		t.Fatalf("concurrent save: %v", err)
	}
	release()
	if err := <-done; err != nil {
		t.Fatalf("Update: %v", err)
	}

	if !saved.LaunchOnStartup || saved.MinimizeToTray {
		t.Fatalf("returned settings = startup %v, tray %v, want startup on and tray off", saved.LaunchOnStartup, saved.MinimizeToTray)
	}
	if !fx.startup || fx.tray {
		t.Fatalf("effects = %+v, want startup on and tray off: they must follow what was stored", *fx)
	}
	for name, got := range map[string]Settings{
		"memory": svc.GetSettings(),
		"disk":   mustServiceAt(t, path).GetSettings(),
	} {
		if !got.LaunchOnStartup || got.MinimizeToTray {
			t.Errorf("%s: startup %v, tray %v, want startup on and tray off", name, got.LaunchOnStartup, got.MinimizeToTray)
		}
	}
}

func TestConcurrentUpdatesOfDifferentFieldsAllSurvive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	svc := mustServiceAt(t, path)
	mutations := []func(*Settings){
		func(s *Settings) { s.Theme = "light" },
		func(s *Settings) { s.MaxActiveDownloads = 9 },
		func(s *Settings) { s.AccentColor = "#112233" },
		func(s *Settings) { s.AutoInstall = true },
		func(s *Settings) { s.SaveBackupLimit = 12 },
		func(s *Settings) { s.PresenceStatus = PresenceBusy },
		func(s *Settings) { s.SeedAfterDownload = true },
		func(s *Settings) { s.Language = LanguageEN },
	}

	start := make(chan struct{})
	errs := make(chan error, len(mutations))
	var wg sync.WaitGroup
	for _, mutate := range mutations {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := svc.Update(func(s *Settings) error {
				mutate(s)
				return nil
			})
			errs <- err
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Errorf("Update: %v", err)
		}
	}

	want := Defaults()
	for _, mutate := range mutations {
		mutate(&want)
	}
	for name, got := range map[string]Settings{
		"memory": svc.GetSettings(),
		"disk":   mustServiceAt(t, path).GetSettings(),
	} {
		if got != want {
			t.Errorf("%s: %+v, want every update applied: %+v", name, got, want)
		}
	}
}

func TestUpdateChangesNothingWhenTheCallbackFails(t *testing.T) {
	refused := errors.New("refused")
	tests := []struct {
		name   string
		mutate func(*Settings) error
		want   error
	}{
		{"callback error", func(s *Settings) error { s.Theme = "light"; return refused }, refused},
		{"invalid result", func(s *Settings) error { s.OverlayHotkey = "F13"; return nil }, ErrOverlayHotkeyInvalid},
		{"nil callback", nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "settings.json")
			svc := mustServiceAt(t, path)
			base := svc.GetSettings()
			var calls atomic.Int32
			if err := svc.AddApplier(func(Settings, Settings) error {
				calls.Add(1)
				return nil
			}); err != nil {
				t.Fatal(err)
			}

			_, err := svc.Update(tt.mutate)
			if err == nil {
				t.Fatal("Update() error = nil")
			}
			if tt.want != nil && !errors.Is(err, tt.want) {
				t.Fatalf("Update() error = %v, want it to wrap %v", err, tt.want)
			}
			if got := svc.GetSettings(); got != base {
				t.Fatalf("memory changed: %+v", got)
			}
			if calls.Load() != 0 {
				t.Fatalf("appliers ran %d times for an update that never validated", calls.Load())
			}
			if _, statErr := os.Stat(path); !errors.Is(statErr, os.ErrNotExist) {
				t.Fatalf("settings file stat = %v, want nothing written", statErr)
			}
		})
	}
}

func TestUpdateUndoesAppliersWhenTheSettingsCannotBeWritten(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "cfg")
	svc := mustServiceAt(t, filepath.Join(dir, "settings.json"))
	base := svc.GetSettings()
	fx := addEffectAppliers(t, svc, base)
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dir, []byte("occupied"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := svc.Update(func(s *Settings) error {
		*s = flipped(*s)
		return nil
	})
	if err == nil {
		t.Fatal("expected the write to fail")
	}
	if fx.startup != base.LaunchOnStartup || fx.tray != base.MinimizeToTray {
		t.Fatalf("effects = %+v, want the previous: the write failed but the change stayed applied", *fx)
	}
	if got := svc.GetSettings(); got != base {
		t.Fatalf("memory changed by a failed update: %+v", got)
	}
}

func TestUpdateStopsAtTheFirstFailingApplier(t *testing.T) {
	svc := mustServiceAt(t, filepath.Join(t.TempDir(), "settings.json"))
	base := svc.GetSettings()
	fx := addEffectAppliers(t, svc, base)
	denied := errors.New("hotkey taken")
	if err := svc.AddApplier(func(Settings, Settings) error { return denied }); err != nil {
		t.Fatal(err)
	}

	_, err := svc.Update(func(s *Settings) error {
		*s = flipped(*s)
		return nil
	})
	if !errors.Is(err, denied) {
		t.Fatalf("Update() error = %v, want it to wrap %v", err, denied)
	}
	if fx.startup != base.LaunchOnStartup || fx.tray != base.MinimizeToTray {
		t.Fatalf("effects = %+v, want the previous: an applier before the failing one stayed applied", *fx)
	}
	if got := svc.GetSettings(); got != base {
		t.Fatalf("memory changed by a refused update: %+v", got)
	}
}
