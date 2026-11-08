//go:build !windows && !devmock && !darwin

package selfupdate

import (
	"context"
	"syscall"
)

func handoffMethodFor(Artifact, bool) handoffMethod { return "" }

func handoffTargetFor(exe string, _ handoffMethod) (string, error) { return exe, nil }

func handoffOldDir(exe string) string { return pathDir(exe) }

func handoffProcAttr() *syscall.SysProcAttr { return nil }

func swapBundleParked(context.Context, handoffPlan, handoffJournal) (func() error, error) {
	return nil, ErrHandoffUnsupported
}

func undoBundleSwap(handoffJournal) error { return ErrHandoffUnsupported }
