//go:build !windows && !devmock && !darwin

package install

func systemExecutable(string) (string, error) {
	return "", errWindowsOnly
}
