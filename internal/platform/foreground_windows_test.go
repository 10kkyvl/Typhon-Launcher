package platform

import (
	"errors"
	"testing"
)

func TestAllowForegroundHandoffReachesUser32(t *testing.T) {
	err := AllowForegroundHandoff()
	if err != nil && !errors.Is(err, ErrNoForegroundRight) {
		t.Fatalf("AllowForegroundHandoff = %v, want nil or ErrNoForegroundRight", err)
	}
}
