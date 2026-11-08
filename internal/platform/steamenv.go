package platform

import (
	"fmt"
	"os"
	"runtime"
	"slices"
	"strings"
)

// steamOverlayVars are the variables without a Steam prefix that the Mac Steam
// sets for a program it starts: the overlay it injects into the process and
// the controller setup of its virtual gamepad.
var steamOverlayVars = map[string]bool{
	"DYLD_INSERT_LIBRARIES":                          true,
	"DYLD_FALLBACK_LIBRARY_PATH":                     true,
	"SDL_GAMECONTROLLER_ALLOW_STEAM_VIRTUAL_GAMEPAD": true,
	"SDL_GAMECONTROLLER_IGNORE_DEVICES":              true,
	"SDL_JOYSTICK_HIDAPI_STEAMXBOX":                  true,
}

// DropSteamLaunchEnv removes from the launcher's environment what the Mac Steam
// added when it started the launcher from a library entry, and returns the
// names it removed. Every game, emulator and CrossOver command inherits this
// environment: with Steam's overlay library forced into them and the shortcut's
// app id set, a game in a bottle never comes up. A launcher that was already
// running when Steam asked for the game starts it with a clean environment,
// and the result must not depend on which of the two happened.
func DropSteamLaunchEnv() ([]string, error) {
	if runtime.GOOS != "darwin" {
		return nil, nil
	}
	names := steamLaunchVars(os.Environ())
	for _, name := range names {
		if err := os.Unsetenv(name); err != nil {
			return nil, fmt.Errorf("unset %s: %w", name, err)
		}
	}
	return names, nil
}

// WithoutSteamLaunchEnv is environ without the variables DropSteamLaunchEnv
// removes, for a program started with an explicit environment.
func WithoutSteamLaunchEnv(environ []string) []string {
	drop := steamLaunchVars(environ)
	out := make([]string, 0, len(environ))
	for _, kv := range environ {
		name, _, _ := strings.Cut(kv, "=")
		if !slices.Contains(drop, name) {
			out = append(out, kv)
		}
	}
	return out
}

// LaunchedBySteam says whether the environment carries the markers the Steam
// client puts on every program it starts from its library.
func LaunchedBySteam(environ []string) bool {
	for _, kv := range environ {
		if kv == "SteamClientLaunch=1" || kv == "SteamEnv=1" {
			return true
		}
	}
	return false
}

// steamLaunchVars lists the variables to drop. Without the markers Steam puts
// on every program it starts nothing is touched: a DYLD variable set by the
// user is not Steam's.
func steamLaunchVars(environ []string) []string {
	if !LaunchedBySteam(environ) {
		return nil
	}
	var names []string
	for _, kv := range environ {
		name, _, ok := strings.Cut(kv, "=")
		if !ok || name == "" {
			continue
		}
		if strings.HasPrefix(name, "Steam") || strings.HasPrefix(name, "STEAM_") ||
			strings.HasPrefix(name, "ENABLE_VK_LAYER_VALVE_steam_overlay") || steamOverlayVars[name] {
			names = append(names, name)
		}
	}
	return names
}
