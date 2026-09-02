//go:build darwin

package platform

import (
	"fmt"

	"golang.org/x/sys/unix"
)

func GetSystemInfo() (SystemInfo, error) {
	info := baseSystemInfo()

	ram, err := unix.SysctlUint64("hw.memsize")
	if err != nil {
		return info, fmt.Errorf("sysctl hw.memsize: %w", err)
	}
	info.RAMBytes = ram

	// CPU and OS names are cosmetic and best-effort, like on Windows (storage_windows.go):
	// a missing sysctl key keeps the base values rather than failing the call.
	if cpu, err := unix.Sysctl("machdep.cpu.brand_string"); err == nil && cpu != "" {
		info.CPU = cpu
	}
	if osVersion, err := unix.Sysctl("kern.osproductversion"); err == nil && osVersion != "" {
		info.OS = "macOS " + osVersion
	}
	return info, nil
}
