//go:build !windows

package download

import "syscall"

func unicastControl(uint32) func(network, address string, c syscall.RawConn) error { return nil }
