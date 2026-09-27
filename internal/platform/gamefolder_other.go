//go:build !darwin

package platform

func OpenGameFolder(path, executable string) error { return OpenFolder(path) }
