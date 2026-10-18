//go:build !windows

package platform

import "errors"

func OpenURI(string) error {
	return errors.ErrUnsupported
}
