package diskusage

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// walkSize adds up the regular files under root. A link inside the tree is
// never followed, so a loop or a link to another drive cannot be walked, and
// it is not counted at its own tiny size either: the walk reports that it met
// one, and the total is then a lower bound.
func walkSize(ctx context.Context, root string) (size int64, sawLink bool, err error) {
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if d.Type()&(os.ModeSymlink|os.ModeIrregular) != 0 {
			sawLink = true
			return nil
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return fmt.Errorf("stat %s: %w", path, err)
		}
		size += info.Size()
		return nil
	})
	if err != nil {
		return 0, false, fmt.Errorf("measure %s: %w", root, err)
	}
	return size, sawLink, nil
}
