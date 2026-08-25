package install

import (
	"errors"
	"math"
	"testing"
)

func TestAddEntrySize(t *testing.T) {
	cases := []struct {
		name  string
		total int64
		size  uint64
		want  int64
		err   error
	}{
		{name: "zero", total: 0, size: 0, want: 0},
		{name: "sum", total: 10, size: 5, want: 15},
		{name: "max int64", total: 0, size: math.MaxInt64, want: math.MaxInt64},
		{name: "above max int64", total: 0, size: math.MaxInt64 + 1, err: errArchiveSize},
		{name: "max uint64", total: 0, size: math.MaxUint64, err: errArchiveSize},
		{name: "overflow on sum", total: math.MaxInt64, size: 1, err: errArchiveSize},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := addEntrySize(tc.total, tc.size)
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
