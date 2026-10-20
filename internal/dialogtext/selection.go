package dialogtext

// cancelledText is the message of the Windows file dialog's cancel error. Wails
// keeps that error in an internal package, so its text is the only handle.
const cancelledText = "cancelled by user"

func Selection(path string, err error) (string, error) {
	if err != nil && err.Error() == cancelledText {
		return "", nil
	}
	return path, err
}
