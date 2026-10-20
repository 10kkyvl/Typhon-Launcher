package install

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"typhon/internal/diskreason"
	"typhon/internal/download"
)

func TestPartialVerdict(t *testing.T) {
	base := t.TempDir()
	dest := filepath.Join(base, "Game")
	partial := dest + partialSuffix
	content := filepath.Join(base, "content")
	if err := os.MkdirAll(content, 0o755); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		item *Installation
		path string
		want bool
		code string
	}{
		{"finished record", &Installation{ID: "i", Type: TypePortable, Status: StatusFailed, Destination: dest, Mode: ModeCopy}, partial, true, ""},
		{"move with the source still there", &Installation{ID: "i", Type: TypePortable, Status: StatusFailed, Destination: dest, Mode: ModeMove, ContentRoot: content}, partial, true, ""},
		{"live install", &Installation{ID: "i", Type: TypePortable, Status: StatusExtracting, Destination: dest}, partial, false, diskreason.OwnedByInstall},
		{"move consumed the source", &Installation{ID: "i", Type: TypePortable, Status: StatusFailed, Destination: dest, Mode: ModeMove, ContentRoot: filepath.Join(base, "gone")}, partial, false, diskreason.OnlyCopy},
		{"no record at all", nil, partial, false, diskreason.NoRecord},
		{"record of another destination", &Installation{ID: "i", Type: TypePortable, Status: StatusFailed, Destination: filepath.Join(base, "Other")}, partial, false, diskreason.NoRecord},
		{"external installer has no partial", &Installation{ID: "i", Type: TypeExeInstaller, Status: StatusFailed, Destination: dest}, partial, false, diskreason.NoRecord},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s, _, _ := newTestService(t)
			if c.item != nil {
				s.mu.Lock()
				s.items = append(s.items, c.item)
				s.mu.Unlock()
			}
			got, code, reason := s.PartialVerdict(c.path)
			if got != c.want || code != c.code {
				t.Fatalf("PartialVerdict = %v %q (%s), want %v %q", got, code, reason, c.want, c.code)
			}
			if !got && reason == "" {
				t.Fatal("a refusal must say why")
			}
		})
	}
}

func TestPartialVerdictReadsTheDiskWithoutHoldingTheLock(t *testing.T) {
	s, _, _ := newTestService(t)
	base := t.TempDir()
	dest := filepath.Join(base, "Game")
	content := filepath.Join(base, "content")
	if err := os.MkdirAll(content, 0o755); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	s.items = append(s.items, &Installation{ID: "i", Type: TypePortable, Status: StatusFailed, Destination: dest, Mode: ModeMove, ContentRoot: content})
	s.mu.Unlock()

	old := rootExists
	held := false
	rootExists = func(path string) bool {
		if s.mu.TryLock() {
			s.mu.Unlock()
		} else {
			held = true
		}
		return old(path)
	}
	t.Cleanup(func() { rootExists = old })

	if ok, code, reason := s.PartialVerdict(dest + partialSuffix); !ok {
		t.Fatalf("PartialVerdict = %v %q (%s)", ok, code, reason)
	}
	if held {
		t.Fatal("the source folder was stat-ed under s.mu")
	}
}

func TestPartialVerdictJudgesARecordThatChangedWhileTheDiskWasRead(t *testing.T) {
	s, _, _ := newTestService(t)
	base := t.TempDir()
	dest := filepath.Join(base, "Game")
	content := filepath.Join(base, "content")
	if err := os.MkdirAll(content, 0o755); err != nil {
		t.Fatal(err)
	}
	item := &Installation{ID: "i", Type: TypePortable, Status: StatusFailed, Destination: dest, Mode: ModeMove, ContentRoot: content}
	s.mu.Lock()
	s.items = append(s.items, item)
	s.mu.Unlock()

	old := rootExists
	rootExists = func(path string) bool {
		found := old(path)
		s.mu.Lock()
		item.ContentRoot = filepath.Join(base, "consumed")
		s.mu.Unlock()
		return found
	}
	t.Cleanup(func() { rootExists = old })

	ok, code, _ := s.PartialVerdict(dest + partialSuffix)
	if ok || code != diskreason.OnlyCopy {
		t.Fatalf("PartialVerdict = %v %q, want the changed record judged by its new source", ok, code)
	}
}

func TestPathInUse(t *testing.T) {
	s, _, _ := newTestService(t)
	base := t.TempDir()
	dest := filepath.Join(base, "Game")
	s.mu.Lock()
	s.items = append(s.items,
		&Installation{ID: "live", Type: TypePortable, Status: StatusExtracting, Destination: dest, SourcePath: filepath.Join(base, "dl", "pack")},
		&Installation{ID: "done", Type: TypePortable, Status: StatusCompleted, Destination: filepath.Join(base, "Finished")})
	s.mu.Unlock()
	release, err := s.reservePartialForTest(filepath.Join(base, "Reserved") + partialSuffix)
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	for path, want := range map[string]bool{
		dest:                                            true,
		filepath.Join(dest, "inner"):                    true,
		filepath.Join(base, "dl", "pack"):               true,
		filepath.Join(base, "dl"):                       true,
		filepath.Join(base, "Finished"):                 false,
		filepath.Join(base, "Elsewhere"):                false,
		filepath.Join(base, "Reserved") + partialSuffix: true,
	} {
		if got := s.PathInUse(path); got != want {
			t.Fatalf("PathInUse(%s) = %v, want %v", path, got, want)
		}
	}
}

func (s *Service) reservePartialForTest(partial string) (func(), error) {
	dest := partial[:len(partial)-len(partialSuffix)]
	s.mu.Lock()
	s.items = append(s.items, &Installation{ID: "r", Type: TypePortable, Status: StatusFailed, Destination: dest, Mode: ModeCopy})
	s.mu.Unlock()
	return s.ReservePartial(partial)
}

func TestAddedDownloadGuardVetoesTheDeletion(t *testing.T) {
	s, _, _ := newTestService(t)
	veto := errors.New("an update reads it")
	s.AddDownloadGuard(func(d download.Download) error {
		if d.ID == "d1" {
			return veto
		}
		return nil
	})
	d := download.Download{ID: "d1", Destination: t.TempDir(), Status: download.StatusCompleted}.WithRoot("Pack")
	if err := s.CheckDownloadDeletable(d); !errors.Is(err, veto) {
		t.Fatalf("CheckDownloadDeletable = %v, want the guard error", err)
	}
	d.ID = "d2"
	if err := s.CheckDownloadDeletable(d); err != nil {
		t.Fatalf("CheckDownloadDeletable = %v", err)
	}
}
