package install

import (
	"testing"

	"typhon/internal/download"
)

func TestAutoInstallFor(t *testing.T) {
	yes, no := true, false
	cases := []struct {
		name     string
		override *bool
		global   bool
		want     bool
	}{
		{name: "no override follows the setting on", global: true, want: true},
		{name: "no override follows the setting off", global: false, want: false},
		{name: "override wins over the setting off", override: &yes, global: false, want: true},
		{name: "override wins over the setting on", override: &no, global: true, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := download.Download{Origin: download.Origin{AutoInstall: tc.override}}
			if got := autoInstallFor(d, tc.global); got != tc.want {
				t.Errorf("autoInstallFor = %v, ожидалось %v", got, tc.want)
			}
		})
	}
}

func TestInstallerLikely(t *testing.T) {
	cases := []struct {
		name  string
		paths []string
		want  bool
	}{
		{name: "empty download"},
		{name: "portable build", paths: []string{"Game/Game.bin", "Game/data/pak0.pak"}},
		{name: "archive", paths: []string{"Game.iso", "Game.part1.rar"}},
		{name: "setup among data", paths: []string{"data.bin", "setup.exe"}, want: true},
		{name: "msi", paths: []string{"Game.msi"}, want: true},
		{name: "upper case extension", paths: []string{"SETUP.EXE"}, want: true},
		{name: "extension inside a name is not an extension", paths: []string{"exe.notes", "readme.msi.txt"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := InstallerLikely(tc.paths); got != tc.want {
				t.Errorf("InstallerLikely(%v) = %v, ожидалось %v", tc.paths, got, tc.want)
			}
		})
	}
}
