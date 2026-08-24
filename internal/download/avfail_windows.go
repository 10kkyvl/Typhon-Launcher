package download

import (
	"errors"

	"golang.org/x/sys/windows"
)

func classifyAntivirusError(err error) error {
	if errors.Is(err, windows.ERROR_VIRUS_INFECTED) || errors.Is(err, windows.ERROR_VIRUS_DELETED) {
		return errBlockedByAV
	}
	return nil
}
