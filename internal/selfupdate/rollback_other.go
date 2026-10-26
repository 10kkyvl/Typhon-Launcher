//go:build !windows

package selfupdate

// A rollback swaps typhon.exe and the uninstall entry of the Windows
// installer; the macOS bundle has neither, and replacing a running bundle is
// the update path's own business.
func platformRollbackSupported() error { return ErrRollbackUnsupported }

func setDisplayVersion(string) (func() error, error) {
	return func() error { return nil }, nil
}
