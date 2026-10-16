package steamlink

import (
	"bytes"
	"context"
	"errors"
	"strconv"
	"sync"
	"testing"
)

func shortcutN(n int) Shortcut {
	name := "Game " + strconv.Itoa(n)
	return Shortcut{
		AppID:    AppID(name),
		Name:     name,
		Exe:      `C:\Games\` + name + `\game.exe`,
		StartDir: `C:\Games\` + name,
		Icon:     `C:\Games\` + name + `\game.exe`,
	}
}

func hasEntry(t *testing.T, s *stand, user string, appID uint32) bool {
	t.Helper()
	doc, err := parseShortcuts(s.read(s.vdf(user)))
	if err != nil {
		t.Fatal(err)
	}
	return doc.has(appID)
}

func TestConcurrentAddKeepsEveryEntry(t *testing.T) {
	const n = 12
	s := newStand(t, "111", "222")
	m := s.manager()
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- m.add(context.Background(), shortcutN(i), Art{}, Sync{})
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("add: %v", err)
		}
	}
	for _, user := range []string{"111", "222"} {
		for i := range n {
			if !hasEntry(t, s, user, shortcutN(i).AppID) {
				t.Fatalf("user %s lost the entry of game %d", user, i)
			}
		}
	}
}

func TestShortcutWithNULIsRefused(t *testing.T) {
	fields := map[string]func(*Shortcut){
		"name":     func(sc *Shortcut) { sc.Name += "\x00x" },
		"exe":      func(sc *Shortcut) { sc.Exe += "\x00x" },
		"startdir": func(sc *Shortcut) { sc.StartDir += "\x00x" },
		"icon":     func(sc *Shortcut) { sc.Icon += "\x00x" },
		"launch":   func(sc *Shortcut) { sc.LaunchOptions += "\x00x" },
	}
	for name, corrupt := range fields {
		t.Run(name, func(t *testing.T) {
			s := newStand(t, "111")
			bad := sampleShortcut()
			corrupt(&bad)
			if err := s.manager().add(context.Background(), bad, Art{}, Sync{}); !errors.Is(err, errBadShortcut) {
				t.Fatalf("add err = %v, want errBadShortcut", err)
			}
			if err := s.manager().add(context.Background(), sampleShortcut(), Art{}, Sync{Update: []Shortcut{bad}}); !errors.Is(err, errBadShortcut) {
				t.Fatalf("add with bad update err = %v, want errBadShortcut", err)
			}
			if err := s.manager().reconcile(context.Background(), Sync{Update: []Shortcut{bad}}); !errors.Is(err, errBadShortcut) {
				t.Fatalf("reconcile err = %v, want errBadShortcut", err)
			}
			if s.exists(s.vdf("111")) {
				t.Fatal("shortcuts.vdf written")
			}
		})
	}
}

func TestRemoveDeletesDuplicates(t *testing.T) {
	sc := sampleShortcut()
	other := foreignEntry("1", 0x80000001, "Other")

	doc, err := parseShortcuts(sampleFile(newEntry("0", sc), other, newEntry("2", sc)))
	if err != nil {
		t.Fatal(err)
	}
	if !doc.remove(sc.AppID) {
		t.Fatal("remove reported no change")
	}
	if got, want := doc.bytes(), sampleFile(foreignEntry("0", 0x80000001, "Other")); !bytes.Equal(got, want) {
		t.Fatalf("got  %x\nwant %x", got, want)
	}

	s := newStand(t, "111")
	s.put(s.vdf("111"), sampleFile(newEntry("0", sc), other, newEntry("2", sc)))
	if err := s.manager().remove(context.Background(), sc.AppID, Sync{}); err != nil {
		t.Fatal(err)
	}
	if hasEntry(t, s, "111", sc.AppID) {
		t.Fatal("a duplicate survived")
	}
}

func TestUpdateRefreshesEveryDuplicate(t *testing.T) {
	sc := sampleShortcut()
	doc, err := parseShortcuts(sampleFile(newEntry("0", Shortcut{AppID: sc.AppID, Name: "A", Exe: "a"}), newEntry("1", Shortcut{AppID: sc.AppID, Name: "B", Exe: "b"})))
	if err != nil {
		t.Fatal(err)
	}
	if !doc.update(sc) {
		t.Fatal("update reported no change")
	}
	if got, want := doc.bytes(), sampleFile(newEntry("0", sc), newEntry("1", sc)); !bytes.Equal(got, want) {
		t.Fatalf("got  %x\nwant %x", got, want)
	}
}

func TestReconcile(t *testing.T) {
	gone := shortcutN(1)
	live := shortcutN(2)
	absent := shortcutN(3)
	goneID := strconv.FormatUint(uint64(gone.AppID), 10)

	moved := live
	moved.Exe = `D:\Moved\game.exe`
	moved.StartDir = `D:\Moved`

	t.Run("drops, refreshes present entries and never adds", func(t *testing.T) {
		s := newStand(t, "111", "222")
		s.put(s.vdf("111"), sampleFile(newEntry("0", gone), newEntry("1", live)))
		for _, name := range []string{goneID + "p.png", goneID + "_hero.JPG", goneID + ".jpeg", "9.png"} {
			s.put(s.grid("111", name), []byte("x"))
		}
		s.put(s.grid("222", goneID+"p.png"), []byte("x"))

		if err := s.manager().reconcile(context.Background(), Sync{Update: []Shortcut{moved, absent}, Drop: []uint32{gone.AppID}}); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if got, want := s.read(s.vdf("111")), sampleFile(newEntry("0", moved)); !bytes.Equal(got, want) {
			t.Fatalf("user 111:\n got %x\nwant %x", got, want)
		}
		if s.exists(s.vdf("222")) {
			t.Fatal("a user without a shortcuts file got one")
		}
		for _, p := range []string{s.grid("111", goneID+"p.png"), s.grid("111", goneID+"_hero.JPG"), s.grid("111", goneID+".jpeg"), s.grid("222", goneID+"p.png")} {
			if s.exists(p) {
				t.Fatalf("%s survived", p)
			}
		}
		if !s.exists(s.grid("111", "9.png")) {
			t.Fatal("an unrelated image was removed")
		}
	})

	t.Run("nothing to change with Steam running is fine", func(t *testing.T) {
		s := newStand(t, "111")
		src := sampleFile(newEntry("0", live))
		s.put(s.vdf("111"), src)
		s.running = true
		if err := s.manager().reconcile(context.Background(), Sync{Update: []Shortcut{live, absent}, Drop: []uint32{gone.AppID}}); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !bytes.Equal(s.read(s.vdf("111")), src) {
			t.Fatal("file touched")
		}
		if s.exists(s.vdf("111") + backupSuffix) {
			t.Fatal("backup written for an unchanged file")
		}
	})

	t.Run("a change with Steam running is refused and the entry stays", func(t *testing.T) {
		s := newStand(t, "111")
		src := sampleFile(newEntry("0", gone))
		s.put(s.vdf("111"), src)
		s.running = true
		if err := s.manager().reconcile(context.Background(), Sync{Drop: []uint32{gone.AppID}}); !errors.Is(err, ErrSteamRunning) {
			t.Fatalf("err = %v, want ErrSteamRunning", err)
		}
		if !bytes.Equal(s.read(s.vdf("111")), src) {
			t.Fatal("file touched")
		}

		s.running = false
		if err := s.manager().add(context.Background(), live, Art{}, Sync{Drop: []uint32{gone.AppID}}); err != nil {
			t.Fatalf("add: %v", err)
		}
		if hasEntry(t, s, "111", gone.AppID) || !hasEntry(t, s, "111", live.AppID) {
			t.Fatal("the next add did not clean up the orphan")
		}
	})

	t.Run("no users is not an error", func(t *testing.T) {
		s := newStand(t)
		if err := s.manager().reconcile(context.Background(), Sync{Drop: []uint32{gone.AppID}}); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
	})

	t.Run("damaged file is an error", func(t *testing.T) {
		s := newStand(t, "111")
		s.put(s.vdf("111"), []byte{0x00})
		if err := s.manager().reconcile(context.Background(), Sync{Drop: []uint32{gone.AppID}}); err == nil {
			t.Fatal("damaged file ignored")
		}
	})

	t.Run("remove carries the sync in one write", func(t *testing.T) {
		s := newStand(t, "111")
		s.put(s.vdf("111"), sampleFile(newEntry("0", gone), newEntry("1", live), newEntry("2", absent)))
		if err := s.manager().remove(context.Background(), absent.AppID, Sync{Update: []Shortcut{moved}, Drop: []uint32{gone.AppID}}); err != nil {
			t.Fatal(err)
		}
		if got, want := s.read(s.vdf("111")), sampleFile(newEntry("0", moved)); !bytes.Equal(got, want) {
			t.Fatalf("got  %x\nwant %x", got, want)
		}
	})
}
