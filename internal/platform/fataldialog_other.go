//go:build !windows

package platform

import (
	"fmt"
	"os"
)

func FatalDialog(title, text string) {
	fmt.Fprintf(os.Stderr, "%s: %s\n", title, text)
}
