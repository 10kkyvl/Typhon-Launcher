//go:build windows

package windowhandoff

import (
	"testing"

	"github.com/wailsapp/wails/v3/pkg/w32"
)

func TestLayeredBitIsWin32s(t *testing.T) {
	if exLayered != uint32(w32.WS_EX_LAYERED) {
		t.Fatalf("exLayered = %#x, want WS_EX_LAYERED %#x", exLayered, uint32(w32.WS_EX_LAYERED))
	}
	if hiddenStyle&exLayered == 0 {
		t.Fatal("paintHidden no longer makes the window layered; revealStyled reads that bit")
	}
}
