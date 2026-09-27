//go:build !windows && !darwin

package messaging

import (
	"context"
	"github.com/wailsapp/wails/v3/pkg/application"
	"log/slog"
	"os/exec"
)

func showWithoutActivation(w *application.WebviewWindow) { w.Show() }
func playTone(ctx context.Context, path string) {
	// #nosec G204 -- Fixed system player; path is the internally generated temporary WAV, passed as one argument without a shell.
	if err := exec.CommandContext(ctx, "/usr/bin/paplay", path).Run(); err != nil && ctx.Err() == nil {
		slog.Debug("play chat notification sound", "error", err)
	}
}
