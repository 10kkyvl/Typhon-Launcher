package torrentpath

import (
	"errors"
	"testing"

	"github.com/anacrolix/torrent/metainfo"
)

func TestValidateInfoRefusesPathsThatLeaveTheDestination(t *testing.T) {
	cases := map[string]metainfo.Info{
		"dot-dot component":   {Name: "Game", Files: []metainfo.FileInfo{{Path: []string{"..", "escaped.bin"}, Length: 1}}},
		"empty component":     {Name: "Game", Files: []metainfo.FileInfo{{Path: []string{"", "a.bin"}, Length: 1}}},
		"backslash separator": {Name: "Game", Files: []metainfo.FileInfo{{Path: []string{`sub\..\a.bin`}, Length: 1}}},
		"drive letter":        {Name: "Game", Files: []metainfo.FileInfo{{Path: []string{"C:", "a.bin"}, Length: 1}}},
		"reserved device":     {Name: "Game", Files: []metainfo.FileInfo{{Path: []string{"CON"}, Length: 1}}},
		"dot-dot name":        {Name: "..", Length: 1},
		"empty name":          {Name: " ", Length: 1},
		"file without a path": {Name: "Game", Files: []metainfo.FileInfo{{Path: nil, Length: 1}}},
	}
	for name, info := range cases {
		t.Run(name, func(t *testing.T) {
			if err := ValidateInfo(&info); !errors.Is(err, ErrUnsafe) {
				t.Fatalf("ValidateInfo = %v, want %v", err, ErrUnsafe)
			}
		})
	}
	if err := ValidateInfo(nil); !errors.Is(err, ErrUnsafe) {
		t.Fatalf("ValidateInfo(nil) = %v", err)
	}
}

func TestValidateInfoAcceptsPlainRelativeLayouts(t *testing.T) {
	single := metainfo.Info{Name: "setup.exe", Length: 1}
	if err := ValidateInfo(&single); err != nil {
		t.Fatalf("single file: %v", err)
	}
	nested := metainfo.Info{Name: "Game", Files: []metainfo.FileInfo{
		{Path: []string{"bin", "game.exe"}, Length: 1},
		{Path: []string{"data", "pak", "textures.pak"}, Length: 1},
	}}
	if err := ValidateInfo(&nested); err != nil {
		t.Fatalf("nested: %v", err)
	}
}
