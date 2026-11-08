//go:build !darwin && !windows

package emulators

import "context"

// Elsewhere the launcher does not look: the emulator's own message stays.
var systemCopies = copyFinder{
	list:  func(context.Context, string) ([]int, error) { return nil, nil },
	ask:   func(int) error { return nil },
	force: func(int) error { return nil },
}
