package storage

import (
	"errors"
	"os"
	"time"

	"golang.org/x/sys/windows"
)

// Readers and virus scanners can briefly hold a destination without
// FILE_SHARE_DELETE. Retry the atomic rename, never delete the old file.
func replaceFile(from, to string) error {
	for attempt := 0; ; attempt++ {
		err := os.Rename(from, to)
		transient := errors.Is(err, windows.ERROR_SHARING_VIOLATION) ||
			errors.Is(err, windows.ERROR_LOCK_VIOLATION) || errors.Is(err, windows.ERROR_ACCESS_DENIED)
		if err == nil || !transient || attempt >= 8 {
			return err
		}
		timer := time.NewTimer(time.Duration(1<<min(attempt, 5)) * 5 * time.Millisecond)
		<-timer.C
	}
}
