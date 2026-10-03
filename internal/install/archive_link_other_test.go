//go:build !windows

package install

import "os"

func makeDirLink(link, target string) error {
	return os.Symlink(target, link)
}
