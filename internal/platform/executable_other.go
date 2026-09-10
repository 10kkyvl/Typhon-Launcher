//go:build !darwin

package platform

import "errors"

func SelectGameExecutable(string, string, string) (string, error) {
	return "", errors.New("CrossOver executable picker is only available on macOS")
}
