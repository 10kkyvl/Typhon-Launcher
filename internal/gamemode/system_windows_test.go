package gamemode

import (
	"os"
	"testing"
	"time"
)

func TestSystemPowerReadsActiveScheme(t *testing.T) {
	p := systemPower{}
	active, err := p.Active()
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 36 {
		t.Fatalf("active scheme %q is not a GUID", active)
	}
	has, err := p.Has(active)
	if err != nil {
		t.Fatal(err)
	}
	if !has {
		t.Fatalf("active scheme %s missing from the enumeration", active)
	}
	if err := p.Set(active); err != nil {
		t.Fatalf("setting the already active scheme: %v", err)
	}
	if got, err := p.Active(); err != nil || got != active {
		t.Fatalf("active after no-op set = %q, %v", got, err)
	}
	if has, err := p.Has("00000000-0000-0000-0000-000000000001"); err != nil || has {
		t.Fatalf("unknown scheme reported present: %v, %v", has, err)
	}
}

func TestSystemPriorityOnOwnProcess(t *testing.T) {
	p := systemPriority{}
	pid := uint32(os.Getpid()) //nolint:gosec // G115: PID from the OS fits uint32 on Windows
	class, err := p.Get(pid)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Set(pid, class); err != nil {
		t.Fatalf("setting the current class: %v", err)
	}
	created, err := p.Created(pid)
	if err != nil {
		t.Fatal(err)
	}
	if created.IsZero() || created.After(time.Now()) {
		t.Fatalf("creation time %v is not a past moment", created)
	}
	again, err := p.Created(pid)
	if err != nil || !again.Equal(created) {
		t.Fatalf("creation time is not stable: %v vs %v, %v", created, again, err)
	}
	if _, err := p.Get(0xFFFFFFF0); err == nil {
		t.Fatal("expected an error for a pid that does not exist")
	}
}
