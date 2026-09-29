package install

import (
	"errors"
	"fmt"

	"typhon/internal/platform"
)

var ErrNotEnoughSpace = errors.New("недостаточно свободного места")

// CheckFreeSpace refuses when free space cannot be determined as well as when
// it is too small: a backup started on a volume nobody measured is the same
// broken promise as one started on a full disk.
func CheckFreeSpace(path string, needed int64) error {
	if needed < 0 {
		return fmt.Errorf("%w: отрицательный размер %d", ErrNotEnoughSpace, needed)
	}
	info, err := platform.GetStorageInfo(path)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrNotEnoughSpace, err)
	}
	//nolint:gosec // G115: needed >= 0 checked above, the int64->uint64 conversion is exact
	if info.FreeBytes < uint64(needed) {
		return fmt.Errorf("%w: нужно %d байт, свободно %d", ErrNotEnoughSpace, needed, info.FreeBytes)
	}
	return nil
}
