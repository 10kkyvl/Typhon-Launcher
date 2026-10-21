package platform

import (
	"fmt"
	"os"
	"strings"

	"golang.org/x/sys/windows"
)

// FatalDialog tells the user why the launcher is about to exit. It is for
// failures before any window or log exists: the GUI executable has no console,
// so stderr alone reaches nobody.
func FatalDialog(title, text string) {
	titlePtr, err := windows.UTF16PtrFromString(strings.ReplaceAll(title, "\x00", ""))
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %s\n", title, text)
		return
	}
	textPtr, err := windows.UTF16PtrFromString(strings.ReplaceAll(text, "\x00", ""))
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %s\n", title, text)
		return
	}
	if _, err := windows.MessageBox(0, textPtr, titlePtr, windows.MB_OK|windows.MB_ICONERROR|windows.MB_SETFOREGROUND); err != nil {
		fmt.Fprintf(os.Stderr, "%s: %s (message box: %v)\n", title, text, err)
	}
}
