// Package emulators is the single source of truth about supported consoles and
// the emulators that run them. The launcher never downloads an emulator, a BIOS
// or firmware and never edits an emulator's configuration: it only remembers
// where the user keeps the program and hands it the game image on launch.
package emulators

import (
	"fmt"
	"net/url"
	"path/filepath"
	"slices"
	"strings"
)

const (
	PS1 = "ps1"
	PS2 = "ps2"
	PSP = "psp"
	PS3 = "ps3"
	// Vita is the PlayStation Vita.
	Vita = "vita"
	PS4  = "ps4"

	// ImageArg marks the place of the game image in an argument template.
	ImageArg = "<image>"
	// TitleArg marks the place of the title id in an argument template.
	TitleArg = "<title>"

	maxCoverURL = 2048
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

	// game describes the unpacked form of a console, on top of Extensions.
	game gameFile
	// manualForms are files of this console the launcher cannot start: the
	// player installs them in the emulator by hand.
	manualForms []string
	// updateCategories are the values of CATEGORY that mean an update, not a
	// game. Case matters: on the PS3 "GD" is game data, on the PS4 "gd" a game.
	updateCategories []string
	// addonCategories are the values of CATEGORY that mean an add-on.
	addonCategories []string
	// skippedFolders never hold a game, whatever lies in them.
	skippedFolders []string
	// installedArgs start a game the emulator keeps for itself. Empty means the
	// emulator is given the image every time.
	installedArgs []string
	// installedGone is the exit code of installedArgs for an unknown title.
	installedGone int
	// bios is where the emulator looks for the BIOS image; zero for consoles
	// whose emulators need none.
	bios biosLayout
	// singleInstance means a second copy refuses to start while one runs.
	singleInstance bool
	// packages are files the emulator installs for itself; the launcher hands
	// them over and later starts the game from hddRoots.
	packages []string
	hddRoots func(program, goos string, env BIOSEnv) []string
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

		bios: biosLayout{
			portableMarks: []string{"portable.txt", "settings.ini"},
			roots:         duckStationRoots,
			config:        []string{"settings.ini"},
			section:       "BIOS", key: "SearchDirectory", dir: "bios",
			// A PS1 BIOS is 512 KB; DuckStation also takes a PS2 one.
			minSize: 512 << 10, maxSize: 16 << 20,
		},
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

		bios: biosLayout{
			portableMarks: []string{"portable.ini", "portable.txt"},
			roots:         pcsx2Roots,
			config:        []string{"inis", "PCSX2.ini"},
			section:       "Folders", key: "Bios", dir: "bios",
			// A PS2 BIOS is 4 MB; some dumps carry the extra ROMs.
			minSize: 4 << 20, maxSize: 16 << 20,
		},
	},
	{
		ID:         PSP,
		Name:       "PlayStation Portable",
		Emulator:   "PPSSPP",
		Homepage:   "https://www.ppsspp.org/",
		Extensions: []string{".iso", ".cso", ".chd", ".pbp"},
		// The pause menu then offers "Exit" instead of "Exit to menu": a
		// fullscreen game has no other way out without the launcher.
		Args: []string{"--pause-menu-exit", "--fullscreen", ImageArg},

		CatalogPlatforms: []string{"PlayStation Portable"},

		dirs:        []string{"PPSSPP"},
		windowsExes: []string{"PPSSPPWindows64.exe", "PPSSPPWindowsARM64.exe", "PPSSPPWindows.exe"},
		macBundles:  []string{"PPSSPP.app", "PPSSPPSDL.app"},
		macBinaries: []string{"PPSSPP", "PPSSPPSDL"},
	},
	{
		ID:         PS3,
		Name:       "PlayStation 3",
		Emulator:   "RPCS3",
		Homepage:   "https://rpcs3.net/",
		Extensions: []string{},
		Args:       []string{"--no-gui", "--fullscreen", ImageArg},

		CatalogPlatforms: []string{"PlayStation 3"},

		dirs:        []string{"RPCS3"},
		windowsExes: []string{"rpcs3.exe"},
		macBundles:  []string{"RPCS3.app"},
		macBinaries: []string{"rpcs3"},

		game: gameFile{
			name: "EBOOT.BIN", parent: "USRDIR", proof: []string{"PARAM.SFO"},
			discFolder: "PS3_GAME", discMark: "PS3_DISC.SFB",
		},
		manualForms:      []string{".iso", ".pkg"},
		updateCategories: []string{"GD"},
		skippedFolders:   []string{"PS3_UPDATE"},
		singleInstance:   true,
		packages:         []string{pkgExt},
		hddRoots:         rpcs3Roots,
	},
	{
		ID:         Vita,
		Name:       "PlayStation Vita",
		Emulator:   "Vita3K",
		Homepage:   "https://vita3k.org/",
		Extensions: []string{packageExt},
		// The Qt frontend stops on a first-run window that nothing would answer.
		Args: []string{"--frontend", "sdl", "-F", ImageArg},

		CatalogPlatforms: []string{"PlayStation Vita"},

		dirs:        []string{"Vita3K"},
		windowsExes: []string{"Vita3K.exe"},
		macBundles:  []string{"Vita3K.app"},
		macBinaries: []string{"Vita3K"},

		game:             gameFile{name: "eboot.bin", proof: []string{"sce_sys", "param.sfo"}, startsFolder: true},
		manualForms:      []string{".iso", ".pkg"},
		updateCategories: []string{"gp"},
		addonCategories:  []string{"ac"},
		installedArgs:    []string{"--frontend", "sdl", "-F", "-r", TitleArg},
		installedGone:    4,
	},
	{
		ID:         PS4,
		Name:       "PlayStation 4",
		Emulator:   "shadPS4",
		Homepage:   "https://shadps4.net/",
		Extensions: []string{},
		Args:       []string{"-f", "true", "-g", ImageArg},

		CatalogPlatforms: []string{"PlayStation 4"},

		dirs:        []string{"shadPS4"},
		windowsExes: []string{"shadPS4.exe"},

		game:             gameFile{name: "eboot.bin", proof: []string{"sce_sys", "param.sfo"}},
		manualForms:      []string{".iso", ".pkg"},
		updateCategories: []string{"gp"},
		addonCategories:  []string{"ac"},
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
		for _, ext := range slices.Concat(p.Extensions, p.packages) {
			if pattern := "*" + ext; !slices.Contains(patterns, pattern) {
				patterns = append(patterns, pattern)
			}
		}
		for _, name := range p.ImageNames() {
			if !slices.ContainsFunc(patterns, func(seen string) bool { return strings.EqualFold(seen, name) }) {
				patterns = append(patterns, name)
			}
		}
	}
	return strings.Join(patterns, ";"), nil
}

// ValidCover says whether a cover can be stored: covers are catalog addresses,
// never a local path a card would make the window load.
func ValidCover(raw string) bool {
	if len(raw) > maxCoverURL {
		return false
	}
	address, err := url.Parse(raw)
	if err != nil {
		return false
	}
	return (address.Scheme == "http" || address.Scheme == "https") && address.Hostname() != ""
}

// AcceptsImage says whether the file name belongs to this console. The name alone
// does not make an unpacked game: GameFolder checks what lies around it.
func (p Platform) AcceptsImage(path string) bool {
	return slices.Contains(p.Extensions, strings.ToLower(filepath.Ext(path))) || p.AcceptsPackage(path) || p.IsGameFile(path)
}

// ManualOnly says whether the file is a form of this console's games the launcher
// cannot start.
func (p Platform) ManualOnly(path string) bool {
	return slices.Contains(p.manualForms, strings.ToLower(filepath.Ext(path)))
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
	if p.game.startsFolder && p.IsGameFile(image) {
		image = filepath.Dir(image)
	}
	args := slices.Clone(p.Args)
	for i, arg := range args {
		if arg == ImageArg {
			args[i] = image
		}
	}
	return args
}

// RemembersInstall says whether the emulator copies a game into its own storage
// on the first launch and starts it by title id afterwards.
func (p Platform) RemembersInstall() bool {
	return len(p.installedArgs) > 0
}

// InstalledCommand fills the template of a game the emulator keeps. The id comes
// from a file on disk, which can be edited by hand, so it is checked on every use.
func (p Platform) InstalledCommand(titleID string) ([]string, error) {
	if !p.RemembersInstall() {
		return nil, fmt.Errorf("%w: %q", ErrUnknownPlatform, p.ID)
	}
	if !validTitleID.MatchString(titleID) {
		return nil, fmt.Errorf("%w: %q", ErrTitleIDInvalid, titleID)
	}
	args := slices.Clone(p.installedArgs)
	for i, arg := range args {
		if arg == TitleArg {
			args[i] = titleID
		}
	}
	return args, nil
}

// InstallGone says whether an exit code means the emulator no longer has the game.
func (p Platform) InstallGone(code int) bool {
	return p.RemembersInstall() && code == p.installedGone
}
