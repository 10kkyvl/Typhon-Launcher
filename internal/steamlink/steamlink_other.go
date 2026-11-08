//go:build !windows && !(darwin && !devmock)

package steamlink

import "context"

func Supported() bool { return false }

func Root() (string, error) { return "", ErrUnsupported }

func steamRunning(context.Context) (bool, error) { return false, ErrUnsupported }
