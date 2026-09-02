//go:build !windows && !devmock

package install

func systemExecutable(string) (string, error) {
	return "", errWindowsOnly
}
