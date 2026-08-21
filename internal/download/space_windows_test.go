package download

import (
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
