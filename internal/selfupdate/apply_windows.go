//go:build windows

package selfupdate

import (
	"context"
	"fmt"
	"os/exec"
	"syscall"

	"typhon/internal/settings"
)

func Apply(ctx context.Context, installerPath string) error {
	dir, err := settings.ConfigDir()
	if err != nil {
		return err
	}
	if err := validateInstallerPath(dir, installerPath); err != nil {
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
	cmd := exec.CommandContext(ctx, installerPath, "/S")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("selfupdate: run installer: %w", err)
	}
	return nil
}
