//go:build !windows

package protocolreg

func Register(string) error { return nil }

func InstallDir() (string, bool, error) { return "", false, nil }
