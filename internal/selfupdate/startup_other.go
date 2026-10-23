//go:build !windows

package selfupdate

import "log/slog"

// The guard offers a rollback of the Windows installation; the other
// platforms have nothing to offer, so it never counts there.
var startGuardSupported = false

func holdInstanceLock(string) (bool, error) { return true, nil }

func askYesNoDialog(string, string) (bool, error) { return false, ErrRollbackUnsupported }

func showErrorDialog(title, text string) {
	slog.Error("selfupdate: dialog", "title", title, "text", text)
}

func defaultSystemLanguage() string { return "en" }
