//go:build windows

// installguard is the Win32 bridge used inside CrossOver.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"typhon/internal/installguard"
)

var errUsage = errors.New("usage: installguard [repack-][verify-]quiet|music cancel-file -- installer [arguments]")

type invocation struct {
	bridge     installguard.Bridge
	cancelFile string
	installer  []string
}

//nolint:forbidigo // this is the standalone helper main entry point.
func main() { os.Exit(run(os.Args)) }

func parseArgs(args []string) (invocation, error) {
	if len(args) < 5 || args[3] != "--" {
		return invocation{}, errUsage
	}
	bridge, err := installguard.ParseBridgeMode(args[1])
	if err != nil {
		return invocation{}, errors.Join(err, errUsage)
	}
	return invocation{bridge: bridge, cancelFile: args[2], installer: args[4:]}, nil
}

func run(args []string) int {
	in, err := parseArgs(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	//nolint:forbidigo // standalone UI/helper operation owns its lifetime; cancellation is handled by its dialog or cancel marker.
	stop := installguard.Start(context.Background(), os.Getpid(), in.bridge.Options)
	defer stop()
	code, stopped, err := installguard.RunJob(in.installer, in.cancelFile, in.bridge.Options.HideProgress, in.bridge.Limit32BitAddressSpace)
	if stopped {
		//nolint:forbidigo,gosec // G703: local user-selected game/helper path; no network path input or privileged filesystem access. fresh private IPC file passed by the launcher, never persistent user state.
		if e := os.WriteFile(in.cancelFile+".stopped", []byte("stopped\n"), 0600); e != nil {
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
