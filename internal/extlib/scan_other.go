//go:build !windows

package extlib

import "context"

const scanSupported = false

func Scan(context.Context) ([]ProviderScan, error) {
	return nil, ErrUnsupported
}
