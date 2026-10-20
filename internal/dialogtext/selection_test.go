package dialogtext

import (
	"errors"
	"testing"
)

func TestSelection(t *testing.T) {
	failure := errors.New("dialog failed")
	cases := []struct {
		name     string
		path     string
		err      error
		wantPath string
		wantErr  error
	}{
		{"picked", `C:\Games\a.exe`, nil, `C:\Games\a.exe`, nil},
		{"cancelled", "", errors.New("cancelled by user"), "", nil},
		{"failed", "", failure, "", failure},
		{"empty without error", "", nil, "", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path, err := Selection(c.path, c.err)
			if path != c.wantPath || !errors.Is(err, c.wantErr) || (c.wantErr == nil && err != nil) {
				t.Fatalf("Selection(%q, %v) = %q, %v; want %q, %v", c.path, c.err, path, err, c.wantPath, c.wantErr)
			}
		})
	}
}
