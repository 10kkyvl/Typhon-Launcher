//go:build windows

package selfupdate

import (
	"context"
	"syscall"
)

func handoffMethodFor(art Artifact, portable bool) handoffMethod {
	switch {
	case art.Kind == KindExe:
		return methodExeFile
	case art.Kind == KindPortable && portable:
		return methodExeZip
	case art.Kind == KindInstaller && !portable:
		return methodInstaller
	}
	return ""
}

func handoffTargetFor(exe string, _ handoffMethod) (string, error) { return exe, nil }

func handoffOldDir(exe string) string { return pathDir(exe) }

func handoffProcAttr() *syscall.SysProcAttr { return detachedProcAttr() }

func swapBundleParked(context.Context, handoffPlan, handoffJournal) (func() error, error) {
	return nil, ErrHandoffUnsupported
}

func undoBundleSwap(handoffJournal) error { return ErrHandoffUnsupported }
