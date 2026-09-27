//go:build darwin

// Package asset embeds the Win32 bridge so installed Macs need no compiler.
package asset

import (
	"bytes"
	"compress/gzip"
	_ "embed"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
)

//go:generate go run generate.go
//go:embed helper.exe.gz
var helper []byte

// Extract uses a private per-run directory. Its owner removes it after Wine exits.
func Extract(base string) (string, func(), error) {
	dir, err := os.MkdirTemp(base, "typhon-installguard-")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() {
		if err := os.RemoveAll(dir); err != nil {
			slog.Warn("remove temporary helper", "error", err)
		}
	}
	r, err := gzip.NewReader(bytes.NewReader(helper))
	if err != nil {
		cleanup()
		return "", nil, err
	}
	//nolint:errcheck // read-only in-memory/file reader cleanup cannot affect the result.
	defer r.Close()
	data, err := io.ReadAll(r)
	if err != nil {
		cleanup()
		return "", nil, err
	}
	path := filepath.Join(dir, "installguard.exe")
	//nolint:forbidigo // fresh private temporary helper file/IPC marker, never persistent user state.
	if err = os.WriteFile(path, data, 0o700); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("write installer bridge: %w", err)
	}
	return path, cleanup, nil
}
