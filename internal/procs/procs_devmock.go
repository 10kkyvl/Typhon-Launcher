//go:build devmock && !windows

package procs

import (
	"context"

	"typhon/internal/devmock"
)

func Supported() bool { return true }

func List(ctx context.Context) ([]Process, bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	entries, err := devmock.List()
	if err != nil {
		return nil, false, err
	}
	out := make([]Process, 0, len(entries))
	for _, e := range entries {
		out = append(out, Process{PID: e.PID, Path: e.Path, CreatedAt: e.CreatedAt})
	}
	return out, true, nil
}
