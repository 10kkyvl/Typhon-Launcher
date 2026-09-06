package install

import (
	"io"
	"log/slog"
)

// closeReadOnly closes a handle that was only ever read from: an archive
// reader (zip/7z/rar) or a copy source opened with os.Open. There is no
// buffered write to flush and no data loss on a failed Close, unlike the
// write side (out.Close in copyFile), which is checked and returned. The
// error is logged instead of dropped so a failing antivirus hook or a
// half-closed handle still shows up in the log.
func closeReadOnly(name string, c io.Closer) {
	if err := c.Close(); err != nil {
		slog.Warn("close read-only handle", "name", name, "error", err)
	}
}
