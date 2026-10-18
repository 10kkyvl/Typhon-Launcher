package screenshots

import (
	"context"
	"fmt"

	"typhon/internal/procs"
)

func processImage(ctx context.Context, pid uint32) (string, error) {
	list, _, err := procs.List(ctx)
	if err != nil {
		return "", fmt.Errorf("list processes: %w", err)
	}
	for _, p := range list {
		if p.PID != pid {
			continue
		}
		if p.PathUnknown || p.Path == "" {
			return "", fmt.Errorf("image path of pid %d is unknown", pid)
		}
		return p.Path, nil
	}
	return "", fmt.Errorf("pid %d is not running", pid)
}
