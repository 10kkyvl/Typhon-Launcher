package install

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestRemoveWorkerFiles(t *testing.T) {
	dir := t.TempDir()
	present := filepath.Join(dir, "present")
	mkText(t, present, "x")
	stuck := filepath.Join(dir, "stuck")
	mkFile(t, filepath.Join(stuck, "inside.bin"), 8)

	cases := []struct {
		name    string
		paths   []string
		wantErr bool
	}{
		{"no paths", nil, false},
		{"empty path is skipped", []string{""}, false},
		{"missing file is not an error", []string{filepath.Join(dir, "missing")}, false},
		{"present file is removed", []string{present}, false},
		{"directory with content is an error", []string{stuck}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := removeWorkerFiles(tc.paths...)
			if (err != nil) != tc.wantErr {
				t.Fatalf("removeWorkerFiles(%v) = %v, wantErr %v", tc.paths, err, tc.wantErr)
			}
			if tc.wantErr && errors.Is(err, os.ErrNotExist) {
				t.Fatalf("a removal failure must not look like a missing file: %v", err)
			}
		})
	}
	if exists(present) {
		t.Fatal("removeWorkerFiles left a regular file in place")
	}
}
