//go:build !windows

package platform

func AllowForegroundHandoff() error {
	return nil
}
