//go:build !windows

package media

import "context"

type unsupported struct{}

func newPlatform() (platform, error) { return unsupported{}, nil }

func (unsupported) supported() bool { return false }

func (unsupported) start(context.Context) error { return nil }

func (unsupported) stop() error { return nil }

func (unsupported) current(context.Context) (Track, bool, error) {
	return Track{}, false, ErrUnsupported
}

func (unsupported) control(context.Context, command) error { return ErrUnsupported }
