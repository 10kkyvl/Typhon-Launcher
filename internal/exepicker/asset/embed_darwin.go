//go:build darwin

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
//go:embed picker.exe.gz
var helper []byte

func Extract(base string) (string, func(), error) {
	dir, err := os.MkdirTemp(base, "typhon-exepicker-")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() {
		if err := os.RemoveAll(dir); err != nil {
			slog.Warn("remove temporary helper", "error", err)
		}
	}
	z, err := gzip.NewReader(bytes.NewReader(helper))
	if err != nil {
		cleanup()
		return "", nil, err
	}
	//nolint:errcheck // read-only in-memory/file reader cleanup cannot affect the result.
	defer z.Close()
	data, err := io.ReadAll(z)
	if err != nil {
		cleanup()
		return "", nil, err
	}
	path := filepath.Join(dir, "exepicker.exe")
	//nolint:forbidigo // fresh private temporary helper file/IPC marker, never persistent user state.
	if err = os.WriteFile(path, data, 0700); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("write executable picker: %w", err)
	}
	return path, cleanup, nil
}
