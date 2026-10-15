//go:build !windows

package library

func killProcessTree(_ int, killRoot func() error) error { return killRoot() }
