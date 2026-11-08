package emulators

import (
	"context"
	"encoding/binary"
	"errors"
	"path/filepath"
	"testing"

	"typhon/internal/emulators/sfotest"
)

// pbp lays out a PSP EBOOT.PBP: a header with eight offsets, the param.sfo first
// and the rest of the parts after it.
func pbp(sfo []byte) []byte {
	out := []byte("\x00PBP\x00\x00\x01\x00")
	start := uint32(pbpHeaderSize)
	end := start + uint32(len(sfo)&0xFFFFFFF)
	out = binary.LittleEndian.AppendUint32(out, start)
	for range 7 {
		out = binary.LittleEndian.AppendUint32(out, end)
	}
	return append(append(out, sfo...), "PSP-DATA"...)
}

func TestReadTitleFromEveryForm(t *testing.T) {
	root := t.TempDir()
	titled := sfotest.Bytes(map[string]string{"TITLE_ID": "NP0APOLLO", "CATEGORY": "HG", "TITLE": "Apollo\nSave  Tool"})

	ps3 := plant(t, filepath.Join(root, "ps3", "NP0APOLLO", "USRDIR", "EBOOT.BIN"), []byte("x"))
	plant(t, filepath.Join(root, "ps3", "NP0APOLLO", "PARAM.SFO"), titled)
	vpk := writeZip(t, filepath.Join(root, "VitaShell.vpk"), map[string][]byte{
		"eboot.bin":         []byte("x"),
		"sce_sys/param.sfo": sfotest.Bytes(map[string]string{"TITLE_ID": "VITASHELL", "CATEGORY": "gd", "TITLE": "VitaShell"}),
	})
	psp := plant(t, filepath.Join(root, "psp", "TrigWars", "EBOOT.PBP"), pbp(
		sfotest.Bytes(map[string]string{"DISC_ID": "UCJS10041", "CATEGORY": "MG", "TITLE": "TrigWars PSP"})))
	iso := plant(t, filepath.Join(root, "game.iso"), []byte("disc"))

	cases := []struct {
		platform, image, want string
	}{
		{PS3, ps3, "Apollo Save Tool"},
		{Vita, vpk, "VitaShell"},
		{PSP, psp, "TrigWars PSP"},
		{PSP, iso, ""},
		{PS2, iso, ""},
	}
	for _, c := range cases {
		got, err := mustPlatform(t, c.platform).ReadTitle(t.Context(), c.image)
		if err != nil || got != c.want {
			t.Errorf("%s %s: ReadTitle = %q, %v; want %q", c.platform, filepath.Base(c.image), got, err, c.want)
		}
	}
}

// A name that is not text must not cost the game its id: the emulator starts
// it by the id, the name is only offered to the user.
func TestATitleThatIsNotTextLeavesTheGameReadable(t *testing.T) {
	data := sfotest.Bytes(map[string]string{"TITLE_ID": "PCSE00001", "CATEGORY": "gd", "TITLE": "Игра"})
	// Entries are sorted by key: CATEGORY, TITLE, TITLE_ID. Mark TITLE as an integer.
	binary.LittleEndian.PutUint16(data[sfoHeaderSize+sfoEntrySize+2:], 0x0404)
	info, err := parseSFO(data)
	if err != nil || info.TitleID != "PCSE00001" || info.Title != "" {
		t.Fatalf("parseSFO = %+v, %v", info, err)
	}
	bad := sfotest.Bytes(map[string]string{"TITLE_ID": "PCSE00001", "TITLE": "\xff\xfe"})
	if info, err := parseSFO(bad); err != nil || info.Title != "" {
		t.Fatalf("invalid utf-8 title: %+v, %v", info, err)
	}
}

func TestReadTitleRefusesABrokenPBP(t *testing.T) {
	root := t.TempDir()
	psp := mustPlatform(t, PSP)
	good := pbp(sfotest.Bytes(map[string]string{"TITLE": "Game"}))

	// Homebrew ships a bare ELF under the PBP name: no name, and no error.
	elf := plant(t, filepath.Join(root, "a", "EBOOT.PBP"), []byte("\x7fELF\x01\x01\x01\x00 and the rest of the program........"))
	if got, err := psp.ReadTitle(t.Context(), elf); err != nil || got != "" {
		t.Errorf("bare ELF: %q, %v", got, err)
	}
	short := plant(t, filepath.Join(root, "b", "EBOOT.PBP"), good[:12])
	if got, err := psp.ReadTitle(t.Context(), short); err != nil || got != "" {
		t.Errorf("short header: %q, %v", got, err)
	}
	backwards := append([]byte(nil), good...)
	binary.LittleEndian.PutUint32(backwards[12:], 4)
	if _, err := psp.ReadTitle(t.Context(), plant(t, filepath.Join(root, "c", "EBOOT.PBP"), backwards)); !errors.Is(err, ErrSFOInvalid) {
		t.Errorf("end before start: %v", err)
	}
	huge := append([]byte(nil), good...)
	binary.LittleEndian.PutUint32(huge[12:], maxSFOSize+pbpHeaderSize+1)
	if _, err := psp.ReadTitle(t.Context(), plant(t, filepath.Join(root, "d", "EBOOT.PBP"), huge)); !errors.Is(err, ErrSFOInvalid) {
		t.Errorf("oversized: %v", err)
	}
	cut := good[:pbpHeaderSize+10]
	cutFile := plant(t, filepath.Join(root, "e", "EBOOT.PBP"), cut)
	binary.LittleEndian.PutUint32(cut[12:], uint32(len(good)&0xFFFFFFF))
	if _, err := psp.ReadTitle(t.Context(), cutFile); !errors.Is(err, ErrSFOInvalid) {
		t.Errorf("truncated: %v", err)
	}
	if _, err := psp.ReadTitle(t.Context(), filepath.Join(root, "missing", "EBOOT.PBP")); !errors.Is(err, ErrImageMissing) {
		t.Errorf("missing: %v", err)
	}
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := psp.ReadTitle(cancelled, plant(t, filepath.Join(root, "f", "EBOOT.PBP"), good)); !errors.Is(err, context.Canceled) {
		t.Errorf("cancelled: %v", err)
	}
}

func TestImageTitleChecksItsInput(t *testing.T) {
	s := &Service{}
	if _, err := s.ImageTitle(t.Context(), "/game.iso", "gameboy"); !errors.Is(err, ErrUnknownPlatform) {
		t.Errorf("unknown console: %v", err)
	}
	if _, err := s.ImageTitle(t.Context(), "relative/EBOOT.PBP", PSP); !errors.Is(err, ErrPathInvalid) {
		t.Errorf("relative path: %v", err)
	}
	root := t.TempDir()
	image := plant(t, filepath.Join(root, "g", "EBOOT.PBP"), pbp(sfotest.Bytes(map[string]string{"TITLE": "Game"})))
	if got, err := s.ImageTitle(t.Context(), image, PSP); err != nil || got != "Game" {
		t.Errorf("ImageTitle = %q, %v", got, err)
	}
}
