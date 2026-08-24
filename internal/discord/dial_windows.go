package discord

import (
	"context"
	"fmt"
	"io"
	"os"
)

func platformDial(ctx context.Context) (io.ReadWriteCloser, error) {
	return dialCancelable(ctx, openNamedPipe)
}

func openNamedPipe() (io.ReadWriteCloser, error) {
	var lastErr error
	for n := 0; n < 10; n++ {
		name := fmt.Sprintf(`\\.\pipe\discord-ipc-%d`, n)
		f, err := os.OpenFile(name, os.O_RDWR, 0)
		if err == nil {
			return f, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		return nil, ErrNoTransport
	}
	return nil, fmt.Errorf("%w: %w", ErrNoTransport, lastErr)
}
