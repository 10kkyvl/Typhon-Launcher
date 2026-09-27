package download

import (
	"errors"

	"golang.org/x/sys/windows"

	"typhon/internal/uierr"
)

var errBlockedByAV = uierr.New("download.blocked_by_av", "файл заблокирован или удалён антивирусом Windows")

func classifyAntivirusError(err error) error {
	if errors.Is(err, windows.ERROR_VIRUS_INFECTED) || errors.Is(err, windows.ERROR_VIRUS_DELETED) {
		return errBlockedByAV
	}
	return nil
}
