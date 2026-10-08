//go:build !windows

package download

import (
	"errors"
	"testing"
)

func TestInterfaceModeIsRefusedWhereSocketsAreNotHeldToTheAdapter(t *testing.T) {
	if err := systemHostCheck(ifaceInfo{Name: "eth0", Index: 2}, true, false); !errors.Is(err, errNetIfaceUnsupport) {
		t.Fatalf("systemHostCheck = %v, want errNetIfaceUnsupport", err)
	}
}
