//go:build !windows

package platform

import (
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

func GetStorageInfo(path string) (StorageInfo, error) {
	if strings.TrimSpace(path) == "" {
		return StorageInfo{}, ErrEmptyPath
	}
	dir := nearestExistingDir(path)
	var stat unix.Statfs_t
	if err := unix.Statfs(dir, &stat); err != nil {
		return StorageInfo{}, err
	}
	total := stat.Blocks * uint64(stat.Bsize)
	free := stat.Bavail * uint64(stat.Bsize)
	return StorageInfo{
		Path:       path,
		Volume:     filepath.VolumeName(dir),
		TotalBytes: total,
		FreeBytes:  free,
		UsedBytes:  total - free,
	}, nil
}

func GetSystemInfo() (SystemInfo, error) {
	return baseSystemInfo(), nil
}
