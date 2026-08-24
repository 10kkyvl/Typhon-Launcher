package download

import (
	"errors"
	"io/fs"
	"os"
	"testing"

	"golang.org/x/sys/windows"
)

func TestClassifyAntivirusError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want error
	}{
		{"virus infected", &fs.PathError{Op: "CreateFile", Path: "x", Err: windows.ERROR_VIRUS_INFECTED}, errBlockedByAV},
		{"virus deleted", &fs.PathError{Op: "CreateFile", Path: "x", Err: windows.ERROR_VIRUS_DELETED}, errBlockedByAV},
		{"permission denied", &fs.PathError{Op: "CreateFile", Path: "x", Err: os.ErrPermission}, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := classifyAntivirusError(c.err)
			if c.want == nil {
				if got != nil {
					t.Fatalf("classify = %v, want nil", got)
				}
				return
			}
			if !errors.Is(got, c.want) {
				t.Fatalf("classify = %v, want %v", got, c.want)
			}
		})
	}
}
