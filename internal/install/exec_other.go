//go:build !windows

package install

func systemExecutable(string) (string, error) {
	return "", errWindowsOnly
}

func resolveExecutable(string) (string, error) {
	return "", errWindowsOnly
}
