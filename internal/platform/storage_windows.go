package platform

import (
	"fmt"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

func GetStorageInfo(path string) (StorageInfo, error) {
	dir := nearestExistingDir(path)
	dirPtr, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		return StorageInfo{}, err
	}

	var free, total, totalFree uint64
	if err := windows.GetDiskFreeSpaceEx(dirPtr, &free, &total, &totalFree); err != nil {
		return StorageInfo{}, fmt.Errorf("disk space for %s: %w", dir, err)
	}

	info := StorageInfo{
		Path:       path,
		Volume:     filepath.VolumeName(dir),
		TotalBytes: total,
		FreeBytes:  free,
		UsedBytes:  total - free,
	}

	rootPtr, err := windows.UTF16PtrFromString(filepath.VolumeName(dir) + `\`)
	if err == nil {
		fsName := make([]uint16, windows.MAX_PATH+1)
		err = windows.GetVolumeInformation(rootPtr, nil, 0, nil, nil, nil, &fsName[0], uint32(len(fsName))) //nolint:gosec // G115: len(fsName) — константа MAX_PATH+1
		if err == nil {
			info.Filesystem = windows.UTF16ToString(fsName)
		}
	}
	return info, nil
}

func GetSystemInfo() (SystemInfo, error) {
	info := baseSystemInfo()

	var mem memoryStatusEx
	mem.Length = uint32(unsafe.Sizeof(mem))
	r1, _, callErr := procGlobalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&mem)))
	if r1 == 0 {
		return info, fmt.Errorf("GlobalMemoryStatusEx: %w", callErr)
	}
	info.RAMBytes = mem.TotalPhys

	if name, err := cpuName(); err == nil && name != "" {
		info.CPU = name
	}
	if osName, err := windowsProductName(); err == nil && osName != "" {
		info.OS = osName
	}
	return info, nil
}

var (
	kernel32                 = windows.NewLazySystemDLL("kernel32.dll")
	procGlobalMemoryStatusEx = kernel32.NewProc("GlobalMemoryStatusEx")
)

type memoryStatusEx struct {
	Length               uint32
	MemoryLoad           uint32
	TotalPhys            uint64
	AvailPhys            uint64
	TotalPageFile        uint64
	AvailPageFile        uint64
	TotalVirtual         uint64
	AvailVirtual         uint64
	AvailExtendedVirtual uint64
}

func cpuName() (string, error) {
	key, err := openRegistryKey(`HARDWARE\DESCRIPTION\System\CentralProcessor\0`)
	if err != nil {
		return "", err
	}
	defer key.Close()
	name, _, err := key.GetStringValue("ProcessorNameString")
	return name, err
}

func windowsProductName() (string, error) {
	key, err := openRegistryKey(`SOFTWARE\Microsoft\Windows NT\CurrentVersion`)
	if err != nil {
		return "", err
	}
	defer key.Close()
	name, _, err := key.GetStringValue("ProductName")
	if err != nil {
		return "", err
	}
	build, _, buildErr := key.GetStringValue("CurrentBuildNumber")
	if buildErr == nil && build != "" {
		return name + " (build " + build + ")", nil
	}
	return name, nil
}
