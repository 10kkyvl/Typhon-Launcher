package platform

import (
	"os/exec"
	"runtime"
	"slices"
	"strings"
	"testing"
)

// What the Mac Steam handed to the launcher when it started a library entry,
// recorded on macOS 27 with Steam of November 2026.
var steamLaunchEnviron = []string{
	"HOME=/Users/u",
	"PATH=/usr/bin:/bin",
	"STEAM_APP_BUNDLE_PATH=/Applications/Steam.app",
	"SteamClientLaunch=1",
	"ENABLE_VK_LAYER_VALVE_steam_overlay_1=1",
	"SDL_GAMECONTROLLER_ALLOW_STEAM_VIRTUAL_GAMEPAD=1",
	"SDL_GAMECONTROLLER_IGNORE_DEVICES=0x28de/0x1002",
	"SDL_JOYSTICK_HIDAPI_STEAMXBOX=0",
	"SteamGameId=13769565928675082240",
	"SteamAppId=3205976898",
	"SteamOverlayGameId=13769565928675082240",
	"SteamAppUser=user",
	"SteamUser=user",
	"Steam3Master=127.0.0.1:57343",
	"SteamVirtualGamepadInfo=/tmp/SteamVirtualGamepadInfo.txt",
	"SteamEnv=1",
	"DYLD_FALLBACK_LIBRARY_PATH=/Applications/Typhon.app/Contents/MacOS:/usr/lib",
	"DYLD_INSERT_LIBRARIES=/Users/u/Library/Application Support/Steam/Steam.AppBundle/Steam/Contents/MacOS/gameoverlayrenderer.dylib",
	"STEAM_DYLD_INSERT_LIBRARIES=/Users/u/Library/Application Support/Steam/Steam.AppBundle/Steam/Contents/MacOS/gameoverlayrenderer.dylib",
	"STEAM_CLIENT_CONFIG_FILE=/Users/u/Library/Application Support/Steam/steam.cfg",
	"LANG=ru_RU.UTF-8",
}

func TestSteamLaunchVarsTakesEverythingSteamAdded(t *testing.T) {
	got := steamLaunchVars(steamLaunchEnviron)
	for _, kv := range steamLaunchEnviron {
		name, _, _ := strings.Cut(kv, "=")
		keep := name == "HOME" || name == "PATH" || name == "LANG"
		if dropped := slices.Contains(got, name); dropped == keep {
			t.Errorf("%s: dropped = %v", name, dropped)
		}
	}
}

func TestSteamLaunchVarsLeavesAProgramSteamDidNotStart(t *testing.T) {
	env := []string{"HOME=/Users/u", "DYLD_INSERT_LIBRARIES=/opt/mine.dylib", "STEAM_COMPAT_DATA_PATH=/x", "SteamEnv=0"}
	if got := steamLaunchVars(env); len(got) != 0 {
		t.Fatalf("dropped %v without a Steam launch", got)
	}
}

// The variables are gone for the programs the launcher starts, which is what
// matters: a child built the usual way sees none of them.
func TestDropSteamLaunchEnvCleansWhatChildrenInherit(t *testing.T) {
	if runtime.GOOS != "darwin" {
		if names, err := DropSteamLaunchEnv(); err != nil || names != nil {
			t.Fatalf("DropSteamLaunchEnv off macOS = %v, %v", names, err)
		}
		t.Skip("only the Mac Steam is handled")
	}
	t.Setenv("SteamClientLaunch", "1")
	t.Setenv("SteamAppId", "3205976898")
	t.Setenv("DYLD_INSERT_LIBRARIES", "/nonexistent/gameoverlayrenderer.dylib")
	t.Setenv("TYPHON_KEEP_ME", "yes")

	names, err := DropSteamLaunchEnv()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"SteamClientLaunch", "SteamAppId", "DYLD_INSERT_LIBRARIES"} {
		if !slices.Contains(names, want) {
			t.Errorf("%s not reported as dropped: %v", want, names)
		}
	}
	out, err := exec.Command("/usr/bin/env").Output()
	if err != nil {
		t.Fatal(err)
	}
	env := string(out)
	for _, gone := range []string{"SteamClientLaunch=", "SteamAppId=", "DYLD_INSERT_LIBRARIES="} {
		if strings.Contains(env, gone) {
			t.Errorf("child still sees %s", gone)
		}
	}
	if !strings.Contains(env, "TYPHON_KEEP_ME=yes") {
		t.Error("an unrelated variable was dropped")
	}
}
