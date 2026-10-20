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

	info.OSUnknown = true
	if cpu, err := unix.Sysctl("machdep.cpu.brand_string"); err == nil && cpu != "" {
		info.CPU, info.CPUUnknown = cpu, false
	}
	if osVersion, err := unix.Sysctl("kern.osproductversion"); err == nil && osVersion != "" {
		info.OS, info.OSUnknown = "macOS "+osVersion, false
	}
	if cores, err := unix.SysctlUint32("hw.physicalcpu"); err == nil && cores > 0 {
		info.PhysicalCores, info.PhysicalCoresUnknown = int(cores), false
	}
	return info, nil
}
