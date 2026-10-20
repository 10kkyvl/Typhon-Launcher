//go:build !windows

package platform

func readAdapters() ([]adapter, error) {
	return nil, ErrGPUUnsupported
}
