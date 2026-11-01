// Package emulators is the single source of truth about supported consoles and
// the emulators that run them. The launcher never downloads an emulator, a BIOS
// or firmware and never edits an emulator's configuration: it only remembers
// where the user keeps the program and hands it the game image on launch.
package emulators

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
)

const (
	PS1 = "ps1"
	PS2 = "ps2"
	PSP = "psp"

	// ImageArg marks the place of the game image in an argument template.
	ImageArg = "<image>"
)

// Platform is one console. Args are checked against the real emulators by hand,
// so they stay data here and not code in the launch path.
type Platform struct {
	ID         string
	Name       string
	Emulator   string
	Homepage   string
	Extensions []string
	Args       []string
	// CatalogPlatforms are the platform names of the game catalog (IGDB) that
	// mean this console.
	CatalogPlatforms []string

	dirs        []string
	windowsExes []string
	macBundles  []string
	macBinaries []string
}

var platforms = []Platform{
	{
		ID:         PS1,
		Name:       "PlayStation",
		Emulator:   "DuckStation",
		Homepage:   "https://www.duckstation.org/",
		Extensions: []string{".cue", ".chd", ".pbp", ".iso", ".img", ".bin", ".ecm", ".mds", ".m3u"},
		Args:       []string{"-batch", "-fullscreen", "--", ImageArg},

		CatalogPlatforms: []string{"PlayStation"},

		dirs:        []string{"DuckStation"},
		windowsExes: []string{"duckstation-qt-x64-ReleaseLTCG.exe", "duckstation-qt-ARM64-ReleaseLTCG.exe", "duckstation-qt.exe"},
		macBundles:  []string{"DuckStation.app"},
		macBinaries: []string{"DuckStation"},
	},
	{
		ID:         PS2,
		Name:       "PlayStation 2",
		Emulator:   "PCSX2",
		Homepage:   "https://pcsx2.net/",
		Extensions: []string{".iso", ".chd", ".cso", ".zso", ".gz", ".bin", ".cue", ".mdf", ".nrg", ".img"},
		Args:       []string{"-batch", "-fullscreen", "--", ImageArg},

		CatalogPlatforms: []string{"PlayStation 2"},

		dirs:        []string{"PCSX2"},
		windowsExes: []string{"pcsx2-qt.exe", "pcsx2-qtx64-avx2.exe", "pcsx2-qtx64.exe"},
		macBundles:  []string{"PCSX2.app"},
		macBinaries: []string{"PCSX2"},
	},
	{
		ID:         PSP,
		Name:       "PlayStation Portable",
		Emulator:   "PPSSPP",
		Homepage:   "https://www.ppsspp.org/",
		Extensions: []string{".iso", ".cso", ".chd", ".pbp"},
		Args:       []string{"--fullscreen", ImageArg},

		CatalogPlatforms: []string{"PlayStation Portable"},

		dirs:        []string{"PPSSPP"},
		windowsExes: []string{"PPSSPPWindows64.exe", "PPSSPPWindowsARM64.exe", "PPSSPPWindows.exe"},
		macBundles:  []string{"PPSSPP.app", "PPSSPPSDL.app"},
		macBinaries: []string{"PPSSPP", "PPSSPPSDL"},
	},
}

// Platforms returns the table in display order. The result is a copy.
func Platforms() []Platform {
	return slices.Clone(platforms)
}

func Lookup(id string) (Platform, bool) {
	for _, p := range platforms {
		if p.ID == id {
			return p, true
		}
	}
	return Platform{}, false
}

// ImagePatterns is the file dialog filter for the given consoles; no ids means
// all of them.
func ImagePatterns(ids []string) (string, error) {
	selected := platforms
	if len(ids) > 0 {
		selected = make([]Platform, 0, len(ids))
		for _, id := range ids {
			p, ok := Lookup(id)
			if !ok {
				return "", fmt.Errorf("%w: %q", ErrUnknownPlatform, id)
			}
			selected = append(selected, p)
		}
	}
	var patterns []string
	for _, p := range selected {
		for _, ext := range p.Extensions {
			if pattern := "*" + ext; !slices.Contains(patterns, pattern) {
				patterns = append(patterns, pattern)
			}
		}
	}
	return strings.Join(patterns, ";"), nil
}

// AcceptsImage says whether the file extension belongs to this console.
func (p Platform) AcceptsImage(path string) bool {
	return slices.Contains(p.Extensions, strings.ToLower(filepath.Ext(path)))
}

// OnCatalog says whether a catalog entry was released for this console. Names
// are compared whole: "PlayStation" must not match "PlayStation 2".
func (p Platform) OnCatalog(platforms []string) bool {
	return slices.ContainsFunc(platforms, func(name string) bool {
		return slices.ContainsFunc(p.CatalogPlatforms, func(own string) bool {
			return strings.EqualFold(strings.TrimSpace(name), own)
		})
	})
}

// Command fills the argument template with the game image.
func (p Platform) Command(image string) []string {
	args := slices.Clone(p.Args)
	for i, arg := range args {
		if arg == ImageArg {
			args[i] = image
		}
	}
	return args
}
