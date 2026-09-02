//go:build devmock && !windows

package devmock

const Enabled = true

const Marker = "TYPHON_DEVMOCK_ENABLED"

func Banner() string { return Marker }
