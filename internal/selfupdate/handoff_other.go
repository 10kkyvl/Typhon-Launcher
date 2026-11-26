//go:build !windows && !devmock && !darwin

package selfupdate

import (
	"context"
	"os/exec"
	"path/filepath"
	"syscall"
)

func handoffMethodFor(Artifact, bool) handoffMethod { return "" }

func handoffTargetFor(exe string, _ handoffMethod) (string, error) { return exe, nil }

func handoffOldDir(exe string) string { return filepath.Dir(exe) }

func handoffProcAttr() *syscall.SysProcAttr { return nil }

func swapBundleParked(context.Context, handoffPlan, handoffJournal) (func() error, error) {
	return nil, ErrHandoffUnsupported
}

// handoffOutput leaves the successor without the old launcher's output: a
// released launcher writes its own log file.
func handoffOutput(*exec.Cmd) {}
