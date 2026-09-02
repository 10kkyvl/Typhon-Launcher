//go:build devmock && !windows

package procs

import (
	"context"

	"typhon/internal/devmock"
)

func Supported() bool { return true }

func List(ctx context.Context) ([]Process, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	entries, err := devmock.List()
	if err != nil {
		return nil, err
	}
	out := make([]Process, 0, len(entries))
	for _, e := range entries {
		out = append(out, Process{PID: e.PID, Path: e.Path, CreatedAt: e.CreatedAt})
	}
	return out, nil
}
