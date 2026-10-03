//go:build !windows

package media

import (
	"context"
	"errors"
	"testing"
)

func TestUnsupportedPlatform(t *testing.T) {
	svc := NewService()
	st, err := svc.Current(context.Background())
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	if st != (State{}) {
		t.Fatalf("Current = %+v, want zero state", st)
	}
	for name, run := range map[string]func(context.Context) error{
		"toggle": svc.TogglePlayPause, "next": svc.Next, "previous": svc.Previous,
	} {
		if err := run(context.Background()); !errors.Is(err, ErrUnsupported) {
			t.Errorf("%s error = %v, want ErrUnsupported", name, err)
		}
	}
}
