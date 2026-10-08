package install

import (
	"errors"
	"math"
	"path/filepath"
	"testing"
)

func TestCheckFreeSpace(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		name    string
		path    string
		needed  int64
		wantErr bool
	}{
		{"fits", dir, 1, false},
		{"zero", dir, 0, false},
		{"negative", dir, -1, true},
		{"too big", dir, math.MaxInt64, true},
		{"empty path", "", 1, true},
		{"missing dir uses nearest parent", filepath.Join(dir, "a", "b"), 1, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := CheckFreeSpace(c.path, c.needed)
			if (err != nil) != c.wantErr {
				t.Fatalf("CheckFreeSpace(%q, %d) = %v, wantErr %v", c.path, c.needed, err, c.wantErr)
			}
			if err != nil && !errors.Is(err, ErrNotEnoughSpace) {
				t.Fatalf("error %v does not wrap ErrNotEnoughSpace", err)
			}
		})
	}
}
