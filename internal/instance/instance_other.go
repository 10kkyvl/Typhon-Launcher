//go:build !windows

package instance

func systemAPI() api { return nil }
