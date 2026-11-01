package storage

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// MakeWritable undoes the read-only mode the torrent engine puts on promoted
// files: a game must be able to write next to its executable. Symlinks and
// other non-regular entries are left alone, and os.Root keeps the walk inside
// dir. It is idempotent, so a walk cut short can simply run again.
func MakeWritable(ctx context.Context, dir string) (err error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer func() {
		err = errors.Join(err, root.Close())
	}()
	return fs.WalkDir(root.FS(), ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if !d.Type().IsRegular() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.Mode().Perm()&0o200 != 0 {
			return nil
		}
		return root.Chmod(filepath.FromSlash(path), info.Mode()|0o200)
	})
}
