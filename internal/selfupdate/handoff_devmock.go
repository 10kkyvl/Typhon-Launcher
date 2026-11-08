//go:build devmock && !windows

package selfupdate

import (
	"context"
	"syscall"
)

// The mock build simulates the Windows swap with the same rename dance: the
// artifact is a raw executable whatever its declared kind (see appliedKinds).
func handoffMethodFor(art Artifact, _ bool) handoffMethod {
	switch art.Kind {
	case KindInstaller, KindBundle, KindExe:
		return methodExeFile
	}
	return ""
}

func handoffTargetFor(exe string, _ handoffMethod) (string, error) { return exe, nil }

func handoffOldDir(exe string) string { return pathDir(exe) }

func handoffProcAttr() *syscall.SysProcAttr { return &syscall.SysProcAttr{Setsid: true} }

func swapBundleParked(context.Context, handoffPlan, handoffJournal) (func() error, error) {
	return nil, ErrHandoffUnsupported
}

func undoBundleSwap(handoffJournal) error { return ErrHandoffUnsupported }
