//go:build !windows

package install

import "os"

// No native elevated broker on these platforms; used by protocol tests.
func openBrokerInstaller(path string) (*os.File, error) { return os.Open(path) }
