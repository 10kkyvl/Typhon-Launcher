//go:build windows

package selfupdate

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"golang.org/x/sys/windows"
)

var startGuardSupported = true

const idYes = 6

// held keeps the instance mutex for the life of the process; the system
// releases it when the process ends, which is the point.
var held []windows.Handle

// holdInstanceLock takes a named mutex and reports whether this process is the
// first to hold it. A mutex that already exists belongs to another launcher
// started from the same data directory.
func holdInstanceLock(name string) (bool, error) {
	namePtr, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return false, err
	}
	handle, err := windows.CreateMutex(nil, false, namePtr)
	if errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		if cerr := windows.CloseHandle(handle); cerr != nil {
			slog.Warn("close instance lock handle", "error", cerr)
		}
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("create mutex %s: %w", name, err)
	}
	held = append(held, handle)
	return true, nil
}

func messageBox(title, text string, flags uint32) (int32, error) {
	titlePtr, err := windows.UTF16PtrFromString(strings.ReplaceAll(title, "\x00", ""))
	if err != nil {
		return 0, err
	}
	textPtr, err := windows.UTF16PtrFromString(strings.ReplaceAll(text, "\x00", ""))
	if err != nil {
		return 0, err
	}
	return windows.MessageBox(0, textPtr, titlePtr, flags)
}

// askYesNoDialog defaults to "No": going back replaces data written since the
// update, so it should not happen on a stray Enter.
func askYesNoDialog(title, text string) (bool, error) {
	ret, err := messageBox(title, text, windows.MB_YESNO|windows.MB_ICONQUESTION|windows.MB_DEFBUTTON2|windows.MB_SETFOREGROUND)
	if err != nil {
		return false, err
	}
	return ret == idYes, nil
}

func showErrorDialog(title, text string) {
	if _, err := messageBox(title, text, windows.MB_OK|windows.MB_ICONERROR|windows.MB_SETFOREGROUND); err != nil {
		fmt.Fprintf(os.Stderr, "%s: %s (message box: %v)\n", title, text, err)
	}
}

func defaultSystemLanguage() string {
	langs, err := windows.GetUserPreferredUILanguages(windows.MUI_LANGUAGE_NAME)
	if err != nil {
		slog.Warn("selfupdate: system UI language", "error", err)
		return "en"
	}
	if len(langs) > 0 && strings.HasPrefix(strings.ToLower(langs[0]), "ru") {
		return "ru"
	}
	return "en"
}
