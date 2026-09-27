//go:build devmock

package settings

import (
	"fmt"
	"os"
	"path/filepath"
)

func configDirOverride() (string, error) {
	p := os.Getenv("TYPHON_TEST_DATA_DIR")
	if p != "" && !filepath.IsAbs(p) {
		return "", fmt.Errorf("TYPHON_TEST_DATA_DIR must be absolute")
	}
	return p, nil
}
