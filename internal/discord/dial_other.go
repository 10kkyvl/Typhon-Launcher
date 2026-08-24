//go:build !windows

package discord

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
)

func platformDial(ctx context.Context) (io.ReadWriteCloser, error) {
	var lastErr error
	dialer := net.Dialer{}
	for _, sub := range [...]string{"", "snap.discord", "app/com.discordapp.Discord"} {
		dir := filepath.Join(runtimeDir(), sub)
		for n := 0; n < 10; n++ {
			path := filepath.Join(dir, fmt.Sprintf("discord-ipc-%d", n))
			conn, err := dialer.DialContext(ctx, "unix", path)
			if err == nil {
				return conn, nil
			}
			lastErr = err
		}
	}
	if lastErr == nil {
		return nil, ErrNoTransport
	}
	return nil, fmt.Errorf("%w: %w", ErrNoTransport, lastErr)
}

func runtimeDir() string {
	for _, key := range [...]string{"XDG_RUNTIME_DIR", "TMPDIR", "TMP", "TEMP"} {
		if v := os.Getenv(key); v != "" {
			return v
		}
	}
	return "/tmp"
}
