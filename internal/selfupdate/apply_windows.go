//go:build windows

package selfupdate

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"syscall"

	"typhon/internal/settings"
)

var errInstallerPathUnsafe = errors.New("selfupdate: installer path cannot be quoted on a command line")

// The target parameter is unused here: NSIS decides where the launcher binary
// lands from installDir, the worker only checks afterwards that it changed.
func Apply(ctx context.Context, installerPath, installDir, _ string) error {
	dir, err := settings.ConfigDir()
	if err != nil {
		return err
	}
	if err := validateInstallerPath(dir, installerPath); err != nil {
		return err
	}
	cmdLine, err := installerCmdLine(installerPath, installDir)
	if err != nil {
		return err
	}

	store, err := NewStore(dir)
	if err != nil {
		return err
	}
	st, err := store.Load()
	if err != nil {
		return err
	}
	if st.Artifact == nil || st.ReadyPath == "" || st.ReadyPath != installerPath {
		return ErrNotReady
	}

	if err := VerifyFile(ctx, installerPath, *st.Artifact); err != nil {
		return err
	}

	//nolint:gosec // G204: installerPath was just verified against the recorded manifest hash inside our own cache dir (invariant 32/33)
	cmd := exec.CommandContext(ctx, installerPath)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CmdLine: cmdLine}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("selfupdate: run installer: %w", err)
	}
	return nil
}

// installerCmdLine builds the command line by hand because NSIS reads
// everything after /D= literally to the end of the line: the switch has to come
// last and the path must stay unquoted, which exec.Cmd.Args cannot express — it
// quotes any argument holding a space. Without /D= a silent install ignores
// where the launcher actually lives and drops the new build into the compiled
// default directory, leaving the running installation untouched.
func installerCmdLine(installerPath, installDir string) (string, error) {
	if hasUnsafePathChars(installerPath) {
		return "", errInstallerPathUnsafe
	}
	if err := validateInstallDir(installDir); err != nil {
		return "", err
	}
	return `"` + installerPath + `" /S /D=` + installDir, nil
}
