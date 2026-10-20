package platform

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

func GetStorageInfo(path string) (StorageInfo, error) {
	if strings.TrimSpace(path) == "" {
		return StorageInfo{}, ErrEmptyPath
	}
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
	r1, _, callErr := procGlobalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&mem))) //nolint:gosec // G103: указатель на локальную структуру mem, живущую весь вызов; конверсия unsafe.Pointer->uintptr внутри самого выражения аргумента Call (единственная форма, разрешённая правилом 4 документации unsafe.Pointer)
	if r1 == 0 {
		return info, fmt.Errorf("GlobalMemoryStatusEx: %w", callErr)
	}
	info.RAMBytes = mem.TotalPhys

	info.OSUnknown = true
	if name, err := cpuName(); err == nil && name != "" {
		info.CPU, info.CPUUnknown = name, false
	}
	if osName, err := windowsProductName(); err == nil && osName != "" {
		info.OS, info.OSUnknown = osName, false
	}
	if cores, err := physicalCores(); err == nil {
		info.PhysicalCores, info.PhysicalCoresUnknown = cores, false
	}
	return info, nil
}

func physicalCores() (int, error) {
	var size uint32
	r1, _, callErr := procGetLogicalProcessorInformationEx.Call(relationProcessorCore, 0, uintptr(unsafe.Pointer(&size))) //nolint:gosec // G103: указатель на локальную переменную size, живущую весь вызов (правило 4 unsafe.Pointer)
	if r1 != 0 {
		return 0, errors.New("GetLogicalProcessorInformationEx succeeded with an empty buffer")
	}
	if !errors.Is(callErr, windows.ERROR_INSUFFICIENT_BUFFER) {
		return 0, fmt.Errorf("GetLogicalProcessorInformationEx size: %w", callErr)
	}
	buf := make([]byte, size)
	r1, _, callErr = procGetLogicalProcessorInformationEx.Call(relationProcessorCore, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size))) //nolint:gosec // G103: buf и size живут весь вызов (правило 4 unsafe.Pointer)
	if r1 == 0 {
		return 0, fmt.Errorf("GetLogicalProcessorInformationEx: %w", callErr)
	}
	return countCores(buf[:size])
}

var (
	kernel32                 = windows.NewLazySystemDLL("kernel32.dll")
	procGlobalMemoryStatusEx = kernel32.NewProc("GlobalMemoryStatusEx")

	procGetLogicalProcessorInformationEx = kernel32.NewProc("GetLogicalProcessorInformationEx")
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
	defer closeKey(key)
	name, _, err := key.GetStringValue("ProcessorNameString")
	return strings.TrimSpace(name), err
}

func windowsProductName() (string, error) {
	key, err := openRegistryKey(`SOFTWARE\Microsoft\Windows NT\CurrentVersion`)
	if err != nil {
		return "", err
	}
	defer closeKey(key)
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
