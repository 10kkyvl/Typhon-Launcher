package classicio

import (
	"log/slog"
	"os"
)

func init() {
	if os.Getenv("TORRENT_STORAGE_DEFAULT_FILE_IO") == "" {
		// Best-effort default for the torrent library's storage backend; if it
		// fails to set, the library falls back to its own default and torrents
		// still work, just with a different file I/O implementation.
		if err := os.Setenv("TORRENT_STORAGE_DEFAULT_FILE_IO", "classic"); err != nil {
			slog.Warn("set torrent storage file io env", "error", err)
		}
	}
}
