package storage

import (
	"errors"
	"os"
	"time"

	"golang.org/x/sys/windows"
)

// Readers and virus scanners can briefly hold a freshly written file without
// sharing it. Retry the operation, never delete the old file.
func retryTransient(op func() error) error {
	for attempt := 0; ; attempt++ {
		err := op()
		transient := errors.Is(err, windows.ERROR_SHARING_VIOLATION) ||
			errors.Is(err, windows.ERROR_LOCK_VIOLATION) || errors.Is(err, windows.ERROR_ACCESS_DENIED)
		if err == nil || !transient || attempt >= 8 {
			return err
		}
		timer := time.NewTimer(time.Duration(1<<min(attempt, 5)) * 5 * time.Millisecond)
		<-timer.C
	}
}

func replaceFile(from, to string) error {
	return retryTransient(func() error { return os.Rename(from, to) })
}

func openRead(path string) (*os.File, error) {
	var f *os.File
	err := retryTransient(func() error {
		var err error
		f, err = os.Open(path)
		return err
	})
	return f, err
}
