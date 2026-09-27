package download

import (
	"errors"
	"os"
	"testing"
)

func missingVolumeDir(t *testing.T) string {
	t.Helper()
	for c := 'Z'; c >= 'D'; c-- {
		root := string(c) + `:\`
		if _, err := os.Stat(root); err != nil {
			return root + `typhon-missing`
		}
	}
	t.Skip("нет свободной буквы диска для проверки ошибки GetStorageInfo")
	return ""
}

func TestCheckFreeSpaceUnknownFails(t *testing.T) {
	dir := missingVolumeDir(t)
	err := checkFreeSpace(dir, 1)
	if !errors.Is(err, errNoFreeSpace) {
		t.Fatalf("err = %v, want errNoFreeSpace", err)
	}
}
