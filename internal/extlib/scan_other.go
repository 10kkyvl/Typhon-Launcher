//go:build !windows

package extlib

import "context"

func Scan(context.Context) ([]ProviderScan, error) {
	return nil, ErrUnsupported
}
