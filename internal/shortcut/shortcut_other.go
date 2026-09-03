//go:build !windows && !devmock

package shortcut

import "errors"

var errUnsupported = errors.New("ярлыки поддерживаются только в Windows")

func Supported() bool { return false }

func DesktopDir() (string, error) {
	return "", errUnsupported
}

func Create(path string, link Link) error {
	return errUnsupported
}
