//go:build !windows

package shortcut

func SetProcessAppID(string) error { return nil }
