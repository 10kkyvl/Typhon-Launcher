package overlay

import (
	"errors"
	"testing"

	"typhon/internal/settings"
)

func TestParseHotkey(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want hotkey
		err  error
	}{
		{"alt backtick", "Alt+`", hotkey{mods: modAlt, vk: 0xC0}, nil},
		{"shift f1", "Shift+F1", hotkey{mods: modShift, vk: 0x70}, nil},
		{"shift f2", "Shift+F2", hotkey{mods: modShift, vk: 0x71}, nil},
		{"ctrl shift o", "Ctrl+Shift+O", hotkey{mods: modCtrl | modShift, vk: 0x4F}, nil},
		{"empty", "", hotkey{}, ErrUnknownHotkey},
		{"unknown key", "Alt+Q", hotkey{}, ErrUnknownHotkey},
		{"lower case", "alt+`", hotkey{}, ErrUnknownHotkey},
		{"padded", " Alt+`", hotkey{}, ErrUnknownHotkey},
		{"reordered modifiers", "Shift+Ctrl+O", hotkey{}, ErrUnknownHotkey},
		{"bare key", "F1", hotkey{}, ErrUnknownHotkey},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := parseHotkey(c.in)
			if !errors.Is(err, c.err) {
				t.Fatalf("error = %v, want %v", err, c.err)
			}
			if got != c.want {
				t.Fatalf("hotkey = %+v, want %+v", got, c.want)
			}
		})
	}
}

func TestEverySettingsHotkeyParses(t *testing.T) {
	for _, name := range settings.OverlayHotkeys() {
		if _, err := parseHotkey(name); err != nil {
			t.Errorf("%q is accepted by settings but not by the overlay: %v", name, err)
		}
	}
	if len(hotkeys) != len(settings.OverlayHotkeys()) {
		t.Errorf("overlay knows %d hotkeys, settings allows %d", len(hotkeys), len(settings.OverlayHotkeys()))
	}
}

func TestNewServiceRejectsUnknownHotkey(t *testing.T) {
	_, err := newService(&fakePlatform{}, func(f func()) { f() }, func(string, any) {}, true, "Alt+Q")
	if !errors.Is(err, ErrUnknownHotkey) {
		t.Fatalf("error = %v, want ErrUnknownHotkey", err)
	}
}
