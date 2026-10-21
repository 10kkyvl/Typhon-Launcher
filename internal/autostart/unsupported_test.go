package autostart

import (
	"testing"

	"typhon/internal/uierr"
)

func TestUnsupportedManagerNeverEnables(t *testing.T) {
	svc, err := NewService(Unsupported("недоступно в портативном режиме"))
	if err != nil {
		t.Fatal(err)
	}

	err = svc.Apply(true)
	if err == nil {
		t.Fatal("Apply(true) succeeded on an unsupported manager")
	}
	if got := uierr.Code(err); got != CodeUnavailable {
		t.Fatalf("Apply(true) code = %q, want %q (error: %v)", got, CodeUnavailable, err)
	}
}

func TestUnsupportedManagerReportsDisabled(t *testing.T) {
	m := Unsupported("reason")
	enabled, err := m.IsEnabled()
	if err != nil || enabled {
		t.Fatalf("IsEnabled() = %v, %v; want false, nil", enabled, err)
	}
	svc, err := NewService(m)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Apply(false); err != nil {
		t.Fatalf("Apply(false) = %v, want nil: nothing is registered", err)
	}
}

func TestUnsupportedManagerWithBlankReasonStillExplains(t *testing.T) {
	err := Unsupported("  ").Enable()
	if err == nil || uierr.Code(err) != CodeUnavailable {
		t.Fatalf("Enable() = %v, want a coded error", err)
	}
}
