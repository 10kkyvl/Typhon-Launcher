//go:build !windows

package overlay

import (
	"context"
	"errors"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
)

var errUnsupported = errors.New("overlay is only available on Windows")

type unsupported struct{}

func newPlatform() platform { return unsupported{} }

func (unsupported) supported() bool { return false }

func (unsupported) register(context.Context, *sync.WaitGroup, hotkey, func()) (func(), error) {
	return nil, errUnsupported
}

func (unsupported) foreground() uintptr { return 0 }

func (unsupported) monitorRect(uintptr) (rect, error) { return rect{}, errUnsupported }

func (unsupported) setForeground(uintptr) error { return errUnsupported }

func (unsupported) isWindow(uintptr) bool { return false }

func (unsupported) notificationState() (int, error) { return 0, errUnsupported }

func (unsupported) isIconic(uintptr) bool { return false }

func (unsupported) restore(uintptr) {}

func newWindow(*application.App) (window, *application.WebviewWindow) { return nil, nil }
