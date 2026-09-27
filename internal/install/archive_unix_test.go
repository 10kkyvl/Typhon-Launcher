//go:build !windows

package install

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Запись в /dev/null проходит и закрывается без ошибки, а fsync символьного
// устройства не поддерживается — это единственный способ на unix развести
// успешный Close и провалившийся Sync, то есть проверить именно наличие
// Sync, а не проверку ошибки Close.
func TestWriteEntrySurfacesSyncFailure(t *testing.T) {
	cases := []struct {
		name    string
		target  string
		wantErr bool
	}{
		{"regular file", filepath.Join(t.TempDir(), "nested", "payload.bin"), false},
		{"sync unsupported", os.DevNull, true},
	}
	payload := "payload bytes that must reach the disk"
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := writeEntry(context.Background(), tc.target, 0o644,
				strings.NewReader(payload), newReporter(nil, int64(len(payload))), make([]byte, 4096))
			if tc.wantErr && err == nil {
				t.Fatal("writeEntry returned nil while the data never reached the disk")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("writeEntry: %v", err)
			}
		})
	}
}
