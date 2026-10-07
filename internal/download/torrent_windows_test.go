package download

import (
	"errors"
	"testing"

	"github.com/anacrolix/torrent/metainfo"
)

func TestValidateInfoRejectsWindowsDeviceNames(t *testing.T) {
	for _, name := range []string{"NUL", "con", "COM1", "aux.txt", "LPT9.log", "prn"} {
		t.Run(name, func(t *testing.T) {
			asFile := &metainfo.Info{Name: "Game", Files: []metainfo.FileInfo{{Length: 1, Path: []string{"sub", name}}}}
			if err := validateInfo(asFile); !errors.Is(err, errBadPaths) {
				t.Errorf("a file named %q: error = %v, want errBadPaths", name, err)
			}
			asName := &metainfo.Info{Name: name, Length: 1}
			if err := validateInfo(asName); !errors.Is(err, errBadPaths) {
				t.Errorf("a torrent named %q: error = %v, want errBadPaths", name, err)
			}
		})
	}
}
