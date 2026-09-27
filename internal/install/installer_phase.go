package install

import (
	"io"
	"os"
	"strings"
)

// Inspect only the recent Inno log, never infer verification from directory size.
func installerLogVerifying(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	//nolint:errcheck // read-only in-memory/file reader cleanup cannot affect the result.
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return false
	}
	offset := max(int64(0), info.Size()-64*1024)
	if _, err = f.Seek(offset, io.SeekStart); err != nil {
		return false
	}
	data, err := io.ReadAll(io.LimitReader(f, 64*1024))
	return err == nil && logHasVerifier(decodeInfText(data))
}

func logHasVerifier(text string) bool {
	for _, line := range strings.Split(strings.ToLower(text), "\n") {
		if at := strings.Index(line, "filename: "); at >= 0 && !strings.Contains(line[:at], "dest ") {
			path := strings.Trim(strings.TrimSpace(line[at+10:]), `"`)
			path = strings.ReplaceAll(path, `\`, "/")
			if strings.HasSuffix(path, "/quicksfv.exe") {
				return true
			}
		}
	}
	return false
}
