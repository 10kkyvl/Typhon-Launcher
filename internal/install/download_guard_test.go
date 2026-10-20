package install

import (
	"errors"
	"path/filepath"
	"testing"

	"typhon/internal/download"
	"typhon/internal/library"
)

func TestCheckDownloadDeletable(t *testing.T) {
	dest := t.TempDir()
	root := filepath.Join(dest, "Pack")
	d := download.Download{ID: "d1", Name: "Pack", Destination: dest, Status: download.StatusCompleted}.WithRoot("Pack")

	cases := []struct {
		name  string
		setup func(s *Service, r *fakeRegistrar)
		d     download.Download
		want  error
	}{
		{"nothing refers to it", func(*Service, *fakeRegistrar) {}, d, nil},
		{"install job reads this download", func(s *Service, _ *fakeRegistrar) {
			s.items = append(s.items, &Installation{ID: "i1", DownloadID: "d1", Status: StatusExtracting})
		}, d, errDownloadInUse},
		{"waiting install of this download", func(s *Service, _ *fakeRegistrar) {
			s.items = append(s.items, &Installation{ID: "i1", DownloadID: "d1", Status: StatusWaitingForUser})
		}, d, errDownloadInUse},
		{"install job reads from inside the folder", func(s *Service, _ *fakeRegistrar) {
			s.items = append(s.items, &Installation{ID: "i1", DownloadID: "other", Status: StatusInstalling, SourcePath: filepath.Join(root, "setup")})
		}, d, errDownloadInUse},
		{"finished install does not hold it", func(s *Service, _ *fakeRegistrar) {
			s.items = append(s.items, &Installation{ID: "i1", DownloadID: "d1", Status: StatusCompleted})
		}, d, nil},
		{"game installed into the folder", func(_ *Service, r *fakeRegistrar) {
			r.put(library.Game{ID: "g1", Title: "Pack", InstallDir: root})
		}, d, errDownloadIsInstall},
		{"game installed inside the folder", func(_ *Service, r *fakeRegistrar) {
			r.put(library.Game{ID: "g1", Title: "Pack", InstallDir: filepath.Join(root, "bin")})
		}, d, errDownloadIsInstall},
		{"game installed elsewhere from this download", func(_ *Service, r *fakeRegistrar) {
			r.put(library.Game{ID: "g1", InstallDir: t.TempDir(), SourceDownloadID: "d1"})
		}, d, nil},
		{"unrelated game is ignored", func(_ *Service, r *fakeRegistrar) {
			r.put(library.Game{ID: "g1", InstallDir: filepath.Join(dest, "Packed")})
		}, d, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s, _, registrar := newTestService(t)
			s.mu.Lock()
			c.setup(s, registrar)
			s.mu.Unlock()
			if err := s.CheckDownloadDeletable(c.d); !errors.Is(err, c.want) {
				t.Fatalf("CheckDownloadDeletable = %v, want %v", err, c.want)
			}
		})
	}
}

func TestCheckDownloadDeletableWithoutLibraryRefuses(t *testing.T) {
	s, _, _ := newTestService(t)
	s.library = nil
	d := download.Download{ID: "d1", Destination: t.TempDir()}.WithRoot("Pack")
	if err := s.CheckDownloadDeletable(d); !errors.Is(err, errNoGameList) {
		t.Fatalf("CheckDownloadDeletable = %v, want errNoGameList", err)
	}
}
