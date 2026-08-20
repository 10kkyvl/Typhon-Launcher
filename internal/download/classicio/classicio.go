package classicio

import "os"

func init() {
	if os.Getenv("TORRENT_STORAGE_DEFAULT_FILE_IO") == "" {
		os.Setenv("TORRENT_STORAGE_DEFAULT_FILE_IO", "classic")
	}
}
