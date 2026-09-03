//go:build !windows && !darwin

package platform

func GetSystemInfo() (SystemInfo, error) {
	return baseSystemInfo(), nil
}
