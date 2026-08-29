//go:build windows

package lan

import (
	"errors"
	"testing"
)

// Windows has no user-mode equivalent of mkfifo inside an ordinary
// directory (named pipes live under \\.\pipe\, not on the target's own
// tree), so TestBuildInfoRejectsNonRegular skips itself here.
func makeFifo(t *testing.T, path string) error {
	t.Helper()
	return errors.New("mkfifo unsupported on windows")
}
