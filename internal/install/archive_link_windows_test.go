//go:build windows

package install

import (
	"fmt"
	"os/exec"
)

// Символическую ссылку Windows создаёт только с правами администратора или в
// режиме разработчика, а точка соединения — такой же reparse point —
// доступна любому пользователю.
func makeDirLink(link, target string) error {
	cmd, err := systemExecutable("cmd.exe")
	if err != nil {
		return err
	}
	//nolint:gosec // G204, invariant 33: cmd.exe is resolved through the system directory; link and target are t.TempDir() paths of the calling test
	out, err := exec.Command(cmd, "/c", "mklink", "/J", link, target).CombinedOutput()
	if err != nil {
		return fmt.Errorf("mklink /J %s: %w: %s", link, err, out)
	}
	return nil
}
