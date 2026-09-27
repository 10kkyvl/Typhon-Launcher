//go:build windows

// installguard is the Win32 bridge used inside CrossOver.
package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"typhon/internal/installguard"
)

//nolint:forbidigo // this is the standalone helper main entry point.
func main() { os.Exit(run()) }
func run() int {
	if len(os.Args) < 5 || os.Args[3] != "--" {
		fmt.Fprintln(os.Stderr, "usage: installguard [quiet|music] cancel-file -- installer [arguments]")
		return 1
	}
	limit := strings.HasPrefix(os.Args[1], "repack-")
	mode := strings.TrimPrefix(os.Args[1], "repack-")
	hidden := mode == "quiet"
	if !hidden && mode != "music" {
		return 1
	}
	cancelFile := os.Args[2]
	//nolint:forbidigo // standalone UI/helper operation owns its lifetime; cancellation is handled by its dialog or cancel marker.
	stop := installguard.Start(context.Background(), os.Getpid(), hidden)
	defer stop()
	code, stopped, err := installguard.RunJob(os.Args[4:], cancelFile, hidden, limit)
	if stopped {
		//nolint:forbidigo,gosec // G703: local user-selected game/helper path; no network path input or privileged filesystem access. fresh private IPC file passed by the launcher, never persistent user state.
		if e := os.WriteFile(cancelFile+".stopped", []byte("stopped\n"), 0600); e != nil {
			fmt.Fprintln(os.Stderr, e)
			return 1
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return code
}
