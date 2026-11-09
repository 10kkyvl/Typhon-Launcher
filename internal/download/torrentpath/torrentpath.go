// Package torrentpath validates the file layout a torrent describes before
// any of it is written under a destination directory. The metainfo comes from
// the network (a feed, a tracker, a LAN peer), so every component is checked
// for traversal, absolute paths, Windows separators and reserved names.
package torrentpath

import (
	"errors"
	"path/filepath"
	"strings"

	"github.com/anacrolix/torrent/metainfo"

	"typhon/internal/winpath"
)

// ErrUnsafe reports a torrent whose name or file paths would escape or
// corrupt the destination directory.
var ErrUnsafe = errors.New("torrent paths escape the destination")

// ValidateInfo accepts a torrent only when its name and every file path are
// plain relative paths made of safe components.
func ValidateInfo(info *metainfo.Info) error {
	if info == nil {
		return ErrUnsafe
	}
	if !IsSafe(info.BestName()) {
		return ErrUnsafe
	}
	for _, fi := range info.UpvertedFiles() {
		components := fi.BestPath()
		if len(components) == 0 {
			if info.IsDir() {
				return ErrUnsafe
			}
			continue
		}
		if !IsSafe(strings.Join(components, "/")) {
			return ErrUnsafe
		}
	}
	return nil
}

// IsSafe reports whether a slash-separated torrent path can be joined under
// a destination without leaving it: no empty, dot or dot-dot components, no
// backslashes or drive colons, nothing non-local and no Windows reserved
// device names.
func IsSafe(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	for _, component := range strings.Split(path, "/") {
		if component == "" || component == "." || component == ".." {
			return false
		}
		if strings.ContainsAny(component, `\:`) {
			return false
		}
		if !filepath.IsLocal(component) {
			return false
		}
		if winpath.Reserved(component) {
			return false
		}
	}
	return true
}
