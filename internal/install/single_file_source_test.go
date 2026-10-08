package install

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSingleFileDownloadIsInspectedAlone(t *testing.T) {
	cases := []struct {
		name    string
		file    string
		write   func(t *testing.T, path string)
		want    Type
		archive bool
	}{
		{
			name: "zip archive",
			file: "Hardware Tycoon v0.2.12.zip",
			write: func(t *testing.T, path string) {
				writeZip(t, path, []zipEntry{
					{name: "Hardware Tycoon/HardwareTycoon.exe", data: []byte("MZ")},
					{name: "Hardware Tycoon/data.pak", data: []byte("payload")},
				})
			},
			want:    TypeArchiveZip,
			archive: true,
		},
		{
			name: "installer exe",
			file: "setup_game.exe",
			write: func(t *testing.T, path string) {
				if err := os.WriteFile(path, []byte("MZ"), 0o600); err != nil {
					t.Fatal(err)
				}
			},
			want: TypeExeInstaller,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s, downloads, _ := newTestService(t)
			shared := t.TempDir()
			other := filepath.Join(shared, "Other Game")
			if err := os.MkdirAll(other, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(other, "setup_other.exe"), make([]byte, 4096), 0o600); err != nil {
				t.Fatal(err)
			}
			writeZip(t, filepath.Join(shared, "Computer Tycoon.zip"), []zipEntry{
				{name: "Computer Tycoon/ComputerTycoon.exe", data: make([]byte, 1<<16)},
			})
			if err := os.WriteFile(filepath.Join(shared, "setup_computer_tycoon.exe"), append([]byte("MZ"), make([]byte, 1<<16)...), 0o600); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(shared, c.file)
			c.write(t, path)
			downloads.add("d1", c.file, shared)

			info, err := s.InspectDownload("d1")
			if err != nil {
				t.Fatalf("inspect: %v", err)
			}
			if info.Plan.Type != c.want {
				t.Fatalf("type = %s, want %s: the shared downloads folder was inspected instead of the file", info.Plan.Type, c.want)
			}
			if c.archive && info.Plan.ArchivePath != path {
				t.Fatalf("archive = %q, want %q", info.Plan.ArchivePath, path)
			}
			if !c.archive && info.Plan.InstallerPath != path {
				t.Fatalf("installer = %q, want %q: another download's installer was picked", info.Plan.InstallerPath, path)
			}
		})
	}
}

func TestSingleFileArchiveInstallsByExtraction(t *testing.T) {
	s, downloads, _ := newTestService(t)
	shared := t.TempDir()
	if err := os.MkdirAll(filepath.Join(shared, "Other Game"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(shared, "Other Game", "big.bin"), make([]byte, 1<<16), 0o600); err != nil {
		t.Fatal(err)
	}
	writeZip(t, filepath.Join(shared, "Computer Tycoon.zip"), []zipEntry{
		{name: "Computer Tycoon/ComputerTycoon.exe", data: make([]byte, 1<<16)},
	})
	writeZip(t, filepath.Join(shared, "Hardware Tycoon.zip"), []zipEntry{
		{name: "Hardware Tycoon/HardwareTycoon.exe", data: []byte("MZ")},
	})
	downloads.add("d1", "Hardware Tycoon.zip", shared)

	dest := filepath.Join(t.TempDir(), "Games", "Hardware Tycoon")
	item, err := s.Start("d1", StartOptions{Destination: dest, Mode: ModeCopy})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	done := s.waitStatus(t, item.ID, StatusCompleted)
	if done.Type != TypeArchiveZip {
		t.Fatalf("type = %s", done.Type)
	}
	if !exists(filepath.Join(dest, "HardwareTycoon.exe")) && !exists(filepath.Join(dest, "Hardware Tycoon", "HardwareTycoon.exe")) {
		t.Fatal("the archive of this download was not extracted")
	}
	if !exists(filepath.Join(shared, "Other Game", "big.bin")) || !exists(filepath.Join(shared, "Computer Tycoon.zip")) {
		t.Fatal("another download was touched")
	}
}
