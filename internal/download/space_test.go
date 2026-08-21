package download

import (
	"errors"
	"math"
	"testing"
)

func TestRequiredBytes(t *testing.T) {
	cases := []struct {
		name  string
		files []FileState
		want  int64
		err   error
	}{
		{name: "empty"},
		{
			name:  "sum selected only",
			files: []FileState{{Size: 10, Selected: true}, {Size: 5}},
			want:  10,
		},
		{
			name:  "negative size",
			files: []FileState{{Size: -1, Selected: true}},
			err:   errBadSizes,
		},
		{
			name:  "unselected negative ignored",
			files: []FileState{{Size: -1}, {Size: 7, Selected: true}},
			want:  7,
		},
		{
			name:  "overflow",
			files: []FileState{{Size: math.MaxInt64, Selected: true}, {Size: 1, Selected: true}},
			err:   errBadSizes,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := requiredBytes(tc.files)
			if tc.err != nil {
				if !errors.Is(err, tc.err) {
					t.Fatalf("err = %v, want %v", err, tc.err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %d, want %d", got, tc.want)
			}
		})
	}
}

func TestCheckFreeSpaceNegative(t *testing.T) {
	if err := checkFreeSpace(t.TempDir(), -1); !errors.Is(err, errBadSizes) {
		t.Fatalf("err = %v, want errBadSizes", err)
	}
}

func TestCheckFreeSpaceOK(t *testing.T) {
	if err := checkFreeSpace(t.TempDir(), 1); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCheckFreeSpaceUnknownFails(t *testing.T) {
	dir := missingVolumeDir(t)
	err := checkFreeSpace(dir, 1)
	if !errors.Is(err, errNoFreeSpace) {
		t.Fatalf("err = %v, want errNoFreeSpace", err)
	}
}
