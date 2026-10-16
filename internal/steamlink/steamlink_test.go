package steamlink

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"typhon/internal/procs"
	"typhon/internal/storage"
)

type stand struct {
	t       *testing.T
	root    string
	running bool
	runErr  error
	write   func(path string, data []byte) error
}

func newStand(t *testing.T, users ...string) *stand {
	t.Helper()
	s := &stand{t: t, root: t.TempDir()}
	if err := os.MkdirAll(filepath.Join(s.root, "userdata"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, u := range users {
		if err := os.MkdirAll(filepath.Join(s.root, "userdata", u), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return s
}

func (s *stand) manager() *manager {
	write := storage.WriteAtomic
	if s.write != nil {
		write = s.write
	}
	return &manager{
		root:    func() (string, error) { return s.root, nil },
		running: func(context.Context) (bool, error) { return s.running, s.runErr },
		write:   write,
	}
}

func (s *stand) vdf(user string) string {
	return filepath.Join(s.root, "userdata", user, "config", "shortcuts.vdf")
}

func (s *stand) grid(user, name string) string {
	return filepath.Join(s.root, "userdata", user, "config", "grid", name)
}

func (s *stand) put(path string, data []byte) {
	s.t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		s.t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		s.t.Fatal(err)
	}
}

func (s *stand) read(path string) []byte {
	s.t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		s.t.Fatal(err)
	}
	return data
}

func (s *stand) exists(path string) bool {
	s.t.Helper()
	_, err := os.Stat(path)
	if err == nil {
		return true
	}
	if !errors.Is(err, os.ErrNotExist) {
		s.t.Fatal(err)
	}
	return false
}

func (s *stand) image(name string) string {
	path := filepath.Join(s.t.TempDir(), name)
	s.put(path, []byte("image:"+name))
	return path
}

func TestAddWritesEveryUser(t *testing.T) {
	s := newStand(t, "111", "222", "0", "notdigits")
	if err := os.WriteFile(filepath.Join(s.root, "userdata", "333"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	foreign := sampleFile(foreignEntry("0", 0x80000001, "Other"))
	s.put(s.vdf("111"), foreign)
	sc := sampleShortcut()
	art := Art{Cover: s.image("cover.PNG"), Hero: s.image("hero.jpg")}

	if err := s.manager().add(context.Background(), sc, art, Sync{}); err != nil {
		t.Fatalf("add: %v", err)
	}

	if got, want := s.read(s.vdf("111")), sampleFile(foreignEntry("0", 0x80000001, "Other"), newEntry("1", sc)); !bytes.Equal(got, want) {
		t.Fatalf("user 111:\n got %x\nwant %x", got, want)
	}
	if got, want := s.read(s.vdf("222")), sampleFile(newEntry("0", sc)); !bytes.Equal(got, want) {
		t.Fatalf("user 222:\n got %x\nwant %x", got, want)
	}
	if got := s.read(s.vdf("111") + backupSuffix); !bytes.Equal(got, foreign) {
		t.Fatalf("backup differs from the original: %x", got)
	}
	if s.exists(s.vdf("222") + backupSuffix) {
		t.Fatal("backup created for a file that did not exist")
	}
	id := strconv.FormatUint(uint64(sc.AppID), 10)
	for _, user := range []string{"111", "222"} {
		for name, want := range map[string]string{
			id + "p.png":     "image:cover.PNG",
			id + "_hero.jpg": "image:hero.jpg",
			id + ".jpg":      "image:hero.jpg",
		} {
			if got := string(s.read(s.grid(user, name))); got != want {
				t.Fatalf("user %s grid %s = %q, want %q", user, name, got, want)
			}
		}
	}
	for _, skipped := range []string{"0", "notdigits"} {
		if s.exists(filepath.Join(s.root, "userdata", skipped, "config")) {
			t.Fatalf("user %q was written", skipped)
		}
	}
}

func TestAddRefusals(t *testing.T) {
	sc := sampleShortcut()
	cases := []struct {
		name  string
		setup func(*stand)
		want  error
	}{
		{"steam running", func(s *stand) { s.running = true }, ErrSteamRunning},
		{"running check fails", func(s *stand) { s.runErr = errors.New("registry down") }, nil},
		{"no users", func(s *stand) {
			if err := os.RemoveAll(filepath.Join(s.root, "userdata", "111")); err != nil {
				s.t.Fatal(err)
			}
		}, ErrNoUsers},
		{"no userdata directory", func(s *stand) {
			if err := os.RemoveAll(filepath.Join(s.root, "userdata")); err != nil {
				s.t.Fatal(err)
			}
		}, ErrNoUsers},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newStand(t, "111")
			tc.setup(s)
			err := s.manager().add(context.Background(), sc, Art{}, Sync{})
			if err == nil {
				t.Fatal("add succeeded")
			}
			if tc.want != nil && !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
			if tc.want == nil && (errors.Is(err, ErrSteamRunning) || !errors.Is(err, s.runErr)) {
				t.Fatalf("err = %v, want the running-check failure", err)
			}
			if s.exists(s.vdf("111")) {
				t.Fatal("shortcuts.vdf written despite the refusal")
			}
		})
	}
}

func TestAddRejectsBadShortcut(t *testing.T) {
	s := newStand(t, "111")
	for name, sc := range map[string]Shortcut{
		"no app id": {Name: "x", Exe: "y"},
		"no name":   {AppID: 1, Exe: "y"},
		"no exe":    {AppID: 1, Name: "x"},
	} {
		if err := s.manager().add(context.Background(), sc, Art{}, Sync{}); err == nil {
			t.Fatalf("%s: add succeeded", name)
		}
	}
}

func TestAddWithDamagedFileTouchesNobody(t *testing.T) {
	s := newStand(t, "111", "222")
	good := sampleFile(foreignEntry("0", 0x80000001, "Other"))
	s.put(s.vdf("111"), good)
	bad := good[:len(good)-1]
	s.put(s.vdf("222"), bad)

	err := s.manager().add(context.Background(), sampleShortcut(), Art{Cover: s.image("c.png")}, Sync{})
	if !errors.Is(err, errVDFTruncated) {
		t.Fatalf("err = %v, want errVDFTruncated", err)
	}
	if !bytes.Equal(s.read(s.vdf("111")), good) {
		t.Fatal("healthy user was modified")
	}
	if !bytes.Equal(s.read(s.vdf("222")), bad) {
		t.Fatal("damaged file was overwritten")
	}
	if s.exists(s.vdf("111") + backupSuffix) {
		t.Fatal("backup written although nothing changed")
	}
	if s.exists(filepath.Join(s.root, "userdata", "111", "config", "grid")) {
		t.Fatal("artwork copied although the operation failed")
	}
}

func TestAddArtHandling(t *testing.T) {
	sc := sampleShortcut()
	id := strconv.FormatUint(uint64(sc.AppID), 10)

	t.Run("unsupported extension is skipped", func(t *testing.T) {
		s := newStand(t, "111")
		if err := s.manager().add(context.Background(), sc, Art{Cover: s.image("cover.gif"), Hero: s.image("hero.webp")}, Sync{}); err != nil {
			t.Fatalf("add: %v", err)
		}
		if s.exists(filepath.Join(s.root, "userdata", "111", "config", "grid")) {
			t.Fatal("grid created for unsupported images")
		}
		if !s.exists(s.vdf("111")) {
			t.Fatal("shortcut not written")
		}
	})

	t.Run("missing art file fails before anything is written", func(t *testing.T) {
		s := newStand(t, "111")
		err := s.manager().add(context.Background(), sc, Art{Cover: filepath.Join(t.TempDir(), "gone.png")}, Sync{})
		if !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("err = %v, want not-exist", err)
		}
		if s.exists(s.vdf("111")) {
			t.Fatal("shortcuts.vdf written")
		}
	})

	t.Run("new extension replaces the old image", func(t *testing.T) {
		s := newStand(t, "111")
		s.put(s.grid("111", id+"p.png"), []byte("old"))
		if err := s.manager().add(context.Background(), sc, Art{Cover: s.image("cover.jpg")}, Sync{}); err != nil {
			t.Fatalf("add: %v", err)
		}
		if s.exists(s.grid("111", id+"p.png")) {
			t.Fatal("old png still shadows the new jpg")
		}
		if got := string(s.read(s.grid("111", id+"p.jpg"))); got != "image:cover.jpg" {
			t.Fatalf("cover = %q", got)
		}
	})
}

func TestAddRollsBackWhenALaterUserFails(t *testing.T) {
	s := newStand(t, "111", "222")
	original := sampleFile(foreignEntry("0", 0x80000001, "Other"))
	s.put(s.vdf("111"), original)
	failing := s.vdf("222")
	s.write = func(path string, data []byte) error {
		if path == failing {
			return errors.New("disk full")
		}
		return storage.WriteAtomic(path, data)
	}

	err := s.manager().add(context.Background(), sampleShortcut(), Art{}, Sync{})
	if err == nil || !strings.Contains(err.Error(), "disk full") {
		t.Fatalf("err = %v, want the write failure", err)
	}
	if !bytes.Equal(s.read(s.vdf("111")), original) {
		t.Fatal("first user keeps the new shortcut after the failure")
	}
	if s.exists(s.vdf("222")) {
		t.Fatal("failed user has a file")
	}
}

func TestRemove(t *testing.T) {
	sc := sampleShortcut()
	id := strconv.FormatUint(uint64(sc.AppID), 10)
	foreign0 := foreignEntry("0", 0x80000001, "Other")
	foreign2 := foreignEntry("2", 0x80000003, "Third")

	t.Run("removes entry and artwork for all users", func(t *testing.T) {
		s := newStand(t, "111", "222")
		s.put(s.vdf("111"), sampleFile(foreign0, newEntry("1", sc), foreign2))
		s.put(s.vdf("222"), sampleFile(newEntry("0", sc)))
		for _, user := range []string{"111", "222"} {
			for _, name := range []string{id + "p.png", id + "_hero.jpg", id + ".jpeg", "keep.png"} {
				s.put(s.grid(user, name), []byte("x"))
			}
		}

		if err := s.manager().remove(context.Background(), sc.AppID, Sync{}); err != nil {
			t.Fatalf("remove: %v", err)
		}
		if got, want := s.read(s.vdf("111")), sampleFile(foreign0, foreignEntry("1", 0x80000003, "Third")); !bytes.Equal(got, want) {
			t.Fatalf("user 111:\n got %x\nwant %x", got, want)
		}
		if got, want := s.read(s.vdf("222")), sampleFile(); !bytes.Equal(got, want) {
			t.Fatalf("user 222:\n got %x\nwant %x", got, want)
		}
		for _, user := range []string{"111", "222"} {
			for _, name := range []string{id + "p.png", id + "_hero.jpg", id + ".jpeg"} {
				if s.exists(s.grid(user, name)) {
					t.Fatalf("user %s still has %s", user, name)
				}
			}
			if !s.exists(s.grid(user, "keep.png")) {
				t.Fatalf("user %s lost an unrelated image", user)
			}
		}
	})

	t.Run("missing entry leaves the file alone", func(t *testing.T) {
		s := newStand(t, "111")
		src := sampleFile(foreign0)
		s.put(s.vdf("111"), src)
		s.running = true
		if err := s.manager().remove(context.Background(), sc.AppID, Sync{}); err != nil {
			t.Fatalf("remove: %v", err)
		}
		if !bytes.Equal(s.read(s.vdf("111")), src) {
			t.Fatal("file changed")
		}
		if s.exists(s.vdf("111") + backupSuffix) {
			t.Fatal("backup written for an unchanged file")
		}
	})

	t.Run("running steam blocks a real change", func(t *testing.T) {
		s := newStand(t, "111")
		src := sampleFile(newEntry("0", sc))
		s.put(s.vdf("111"), src)
		s.running = true
		if err := s.manager().remove(context.Background(), sc.AppID, Sync{}); !errors.Is(err, ErrSteamRunning) {
			t.Fatalf("err = %v, want ErrSteamRunning", err)
		}
		if !bytes.Equal(s.read(s.vdf("111")), src) {
			t.Fatal("file changed")
		}
	})

	t.Run("damaged file is an error and stays", func(t *testing.T) {
		s := newStand(t, "111")
		bad := []byte{0x00, 'x'}
		s.put(s.vdf("111"), bad)
		if err := s.manager().remove(context.Background(), sc.AppID, Sync{}); err == nil {
			t.Fatal("remove succeeded on a damaged file")
		}
		if !bytes.Equal(s.read(s.vdf("111")), bad) {
			t.Fatal("damaged file was rewritten")
		}
	})
}

func TestContains(t *testing.T) {
	sc := sampleShortcut()

	t.Run("present for one of two users", func(t *testing.T) {
		s := newStand(t, "111", "222")
		s.put(s.vdf("222"), sampleFile(newEntry("0", sc)))
		got, err := s.manager().contains(context.Background(), sc.AppID)
		if err != nil || !got {
			t.Fatalf("contains = %v, %v", got, err)
		}
	})

	t.Run("absent", func(t *testing.T) {
		s := newStand(t, "111")
		s.put(s.vdf("111"), sampleFile(foreignEntry("0", 0x80000001, "Other")))
		got, err := s.manager().contains(context.Background(), sc.AppID)
		if err != nil || got {
			t.Fatalf("contains = %v, %v", got, err)
		}
	})

	t.Run("no file at all", func(t *testing.T) {
		s := newStand(t, "111")
		got, err := s.manager().contains(context.Background(), sc.AppID)
		if err != nil || got {
			t.Fatalf("contains = %v, %v", got, err)
		}
	})

	t.Run("no users is not an error", func(t *testing.T) {
		s := newStand(t)
		got, err := s.manager().contains(context.Background(), sc.AppID)
		if err != nil || got {
			t.Fatalf("contains = %v, %v", got, err)
		}
	})

	t.Run("unreadable file is an error", func(t *testing.T) {
		s := newStand(t, "111")
		s.put(s.vdf("111"), []byte{0x00})
		if _, err := s.manager().contains(context.Background(), sc.AppID); err == nil {
			t.Fatal("damaged file reported as a plain answer")
		}
	})

	t.Run("steam not installed propagates", func(t *testing.T) {
		m := manager{root: func() (string, error) { return "", ErrNotInstalled }}
		if _, err := m.contains(context.Background(), sc.AppID); !errors.Is(err, ErrNotInstalled) {
			t.Fatalf("err = %v, want ErrNotInstalled", err)
		}
	})
}

func TestCanceledContext(t *testing.T) {
	s := newStand(t, "111")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.manager().add(ctx, sampleShortcut(), Art{}, Sync{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if s.exists(s.vdf("111")) {
		t.Fatal("file written after cancel")
	}
}

func TestOversizedShortcutsFile(t *testing.T) {
	s := newStand(t, "111")
	s.put(s.vdf("111"), make([]byte, maxVDFBytes+1))
	if err := s.manager().add(context.Background(), sampleShortcut(), Art{}, Sync{}); !errors.Is(err, errVDFTooLarge) {
		t.Fatalf("err = %v, want errVDFTooLarge", err)
	}
}

func TestEmptyShortcutsFileIsAnEmptyList(t *testing.T) {
	s := newStand(t, "111")
	s.put(s.vdf("111"), nil)
	sc := sampleShortcut()
	if err := s.manager().add(context.Background(), sc, Art{}, Sync{}); err != nil {
		t.Fatalf("add: %v", err)
	}
	if got, want := s.read(s.vdf("111")), sampleFile(newEntry("0", sc)); !bytes.Equal(got, want) {
		t.Fatalf("got %x want %x", got, want)
	}
}

func TestRunningFromList(t *testing.T) {
	steam := procs.Process{PID: 10, Path: `C:\Program Files (x86)\Steam\STEAM.EXE`}
	other := procs.Process{PID: 11, Path: `C:\Windows\notepad.exe`}
	hidden := procs.Process{PID: 12, PathUnknown: true}
	listOf := func(list ...procs.Process) func(context.Context) ([]procs.Process, bool, error) {
		return func(context.Context) ([]procs.Process, bool, error) { return list, true, nil }
	}
	cases := []struct {
		name    string
		pid     uint32
		list    func(context.Context) ([]procs.Process, bool, error)
		want    bool
		wantErr bool
	}{
		{"no pid recorded", 0, listOf(steam), false, false},
		{"steam alive", 10, listOf(steam, other), true, false},
		{"pid reused by another program", 11, listOf(steam, other), false, false},
		{"path unreadable counts as running", 12, listOf(hidden), true, false},
		{"pid gone", 99, listOf(steam), false, false},
		{"list failed", 10, func(context.Context) ([]procs.Process, bool, error) {
			return nil, false, errors.New("snapshot")
		}, false, true},
		{"list incomplete", 10, func(context.Context) ([]procs.Process, bool, error) {
			return []procs.Process{steam}, false, nil
		}, false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := runningFromList(context.Background(), tc.pid, tc.list)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if got != tc.want {
				t.Fatalf("running = %v, want %v", got, tc.want)
			}
		})
	}
}
