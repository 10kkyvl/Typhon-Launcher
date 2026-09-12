//go:build !windows && !darwin

package messaging

import (
	"context"
	"github.com/wailsapp/wails/v3/pkg/application"
	"os/exec"
)

func showWithoutActivation(w *application.WebviewWindow) { w.Show() }
func playTone(ctx context.Context, path string)          { _ = exec.CommandContext(ctx, "paplay", path).Run() }
