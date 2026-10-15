package overlay

import (
	"errors"
	"fmt"

	"typhon/internal/settings"
)

const (
	modAlt   = 0x0001
	modCtrl  = 0x0002
	modShift = 0x0004

	vkOem3 = 0xC0
	vkF1   = 0x70
	vkF2   = 0x71
	vkO    = 0x4F
	vkF11  = 0x7A
	vkF12  = 0x7B
)

var ErrUnknownHotkey = errors.New("unknown overlay hotkey")

type hotkey struct {
	mods uint32
	vk   uint32
}

var hotkeys = map[string]hotkey{
	settings.OverlayHotkeyAltBacktick: {mods: modAlt, vk: vkOem3},
	settings.OverlayHotkeyShiftF1:     {mods: modShift, vk: vkF1},
	settings.OverlayHotkeyShiftF2:     {mods: modShift, vk: vkF2},
	settings.OverlayHotkeyCtrlShiftO:  {mods: modCtrl | modShift, vk: vkO},
}

var screenshotHotkeys = map[string]hotkey{
	settings.ScreenshotHotkeyF12:      {vk: vkF12},
	settings.ScreenshotHotkeyF11:      {vk: vkF11},
	settings.ScreenshotHotkeyShiftF12: {mods: modShift, vk: vkF12},
	settings.ScreenshotHotkeyCtrlF12:  {mods: modCtrl, vk: vkF12},
}

func parseScreenshotHotkey(name string) (hotkey, error) {
	key, ok := screenshotHotkeys[name]
	if !ok {
		return hotkey{}, fmt.Errorf("%w: %q", ErrUnknownHotkey, name)
	}
	return key, nil
}

func parseHotkey(name string) (hotkey, error) {
	key, ok := hotkeys[name]
	if !ok {
		return hotkey{}, fmt.Errorf("%w: %q", ErrUnknownHotkey, name)
	}
	return key, nil
}
