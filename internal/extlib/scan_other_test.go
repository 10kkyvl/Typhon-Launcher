//go:build !windows

package extlib

import (
	"context"
	"errors"
	"testing"
)

func TestScanUnsupportedOutsideWindows(t *testing.T) {
	if _, err := Scan(context.Background()); !errors.Is(err, ErrUnsupported) || !errors.Is(err, errors.ErrUnsupported) {
		t.Fatalf("error = %v, want ErrUnsupported", err)
	}
}
