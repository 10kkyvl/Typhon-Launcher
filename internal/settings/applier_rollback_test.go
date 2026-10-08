package settings

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

type effects struct {
	startup bool
	tray    bool
}

func addEffectAppliers(t *testing.T, svc *Service, base Settings) *effects {
	t.Helper()
	fx := &effects{startup: base.LaunchOnStartup, tray: base.MinimizeToTray}
	if err := svc.AddApplier(func(prev, next Settings) error {
		if prev.LaunchOnStartup != next.LaunchOnStartup {
			fx.startup = next.LaunchOnStartup
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := svc.AddApplier(func(prev, next Settings) error {
		if prev.MinimizeToTray != next.MinimizeToTray {
			fx.tray = next.MinimizeToTray
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return fx
}

func flipped(base Settings) Settings {
	next := base
	next.LaunchOnStartup = !base.LaunchOnStartup
	next.MinimizeToTray = !base.MinimizeToTray
	return next
}

func TestAppliersAreUndoneWhenTheSettingsCannotBeWritten(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "cfg")
	svc := mustServiceAt(t, filepath.Join(dir, "settings.json"))
	base := svc.GetSettings()
	fx := addEffectAppliers(t, svc, base)
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dir, []byte("occupied"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := svc.SaveSettings(flipped(base)); err == nil {
		t.Fatal("expected the write to fail")
	}

	if fx.startup != base.LaunchOnStartup || fx.tray != base.MinimizeToTray {
		t.Fatalf("effects = %+v, want the previous (startup %v, tray %v): the write failed but the change stayed applied",
			*fx, base.LaunchOnStartup, base.MinimizeToTray)
	}
	if got := svc.GetSettings(); got != base {
		t.Fatalf("memory changed by a failed save: %+v", got)
	}
}

func TestAppliersAreUndoneWhenALaterApplierFails(t *testing.T) {
	svc := mustServiceAt(t, filepath.Join(t.TempDir(), "settings.json"))
	base := svc.GetSettings()
	fx := addEffectAppliers(t, svc, base)
	denied := errors.New("hotkey taken")
	deniedCalls := 0
	if err := svc.AddApplier(func(prev, next Settings) error {
		deniedCalls++
		return denied
	}); err != nil {
		t.Fatal(err)
	}
	laterCalls := 0
	if err := svc.AddApplier(func(prev, next Settings) error {
		laterCalls++
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	err := svc.SaveSettings(flipped(base))
	if !errors.Is(err, denied) {
		t.Fatalf("SaveSettings error = %v, want it to wrap %v", err, denied)
	}

	if fx.startup != base.LaunchOnStartup || fx.tray != base.MinimizeToTray {
		t.Fatalf("effects = %+v, want the previous (startup %v, tray %v): an applier before the failing one stayed applied",
			*fx, base.LaunchOnStartup, base.MinimizeToTray)
	}
	if deniedCalls != 1 {
		t.Fatalf("the failing applier ran %d times, want 1: it never succeeded, so there is nothing to undo", deniedCalls)
	}
	if laterCalls != 0 {
		t.Fatalf("an applier after the failing one ran %d times", laterCalls)
	}
}

func TestUndoRunsInReverseOrderWithSwappedSettings(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "cfg")
	svc := mustServiceAt(t, filepath.Join(dir, "settings.json"))
	base := svc.GetSettings()

	type call struct {
		name       string
		prev, next string
	}
	var calls []call
	for _, name := range []string{"first", "second", "third"} {
		if err := svc.AddApplier(func(prev, next Settings) error {
			calls = append(calls, call{name, prev.Theme, next.Theme})
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dir, []byte("occupied"), 0o600); err != nil {
		t.Fatal(err)
	}
	next := base
	next.Theme = "light"

	if err := svc.SaveSettings(next); err == nil {
		t.Fatal("expected the write to fail")
	}

	want := []call{
		{"first", "dark", "light"}, {"second", "dark", "light"}, {"third", "dark", "light"},
		{"third", "light", "dark"}, {"second", "light", "dark"}, {"first", "light", "dark"},
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("applier calls = %+v, want %+v", calls, want)
	}
}

func TestFailedUndoIsReportedTogetherWithTheOriginalError(t *testing.T) {
	svc := mustServiceAt(t, filepath.Join(t.TempDir(), "settings.json"))
	base := svc.GetSettings()
	stuck := errors.New("registry locked")
	if err := svc.AddApplier(func(prev, next Settings) error {
		if next.LaunchOnStartup == base.LaunchOnStartup {
			return stuck
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	denied := errors.New("hotkey taken")
	if err := svc.AddApplier(func(prev, next Settings) error { return denied }); err != nil {
		t.Fatal(err)
	}

	err := svc.SaveSettings(flipped(base))
	if !errors.Is(err, denied) || !errors.Is(err, stuck) {
		t.Fatalf("SaveSettings error = %v, want both %q and %q", err, denied, stuck)
	}
}
