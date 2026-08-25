//go:build !windows

package download

// Windows-specific antivirus error codes (ERROR_VIRUS_INFECTED/DELETED) have
// no equivalent on other platforms, so there is nothing to classify here.
func classifyAntivirusError(err error) error {
	return nil
}
