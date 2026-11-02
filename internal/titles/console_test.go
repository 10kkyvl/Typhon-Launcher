package titles

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

// The rules: a console counts as a whole bracket part without the PC beside it,
// as a word behind a separator, or first before a separator. Everything else is
// the name of a game.
func TestParseConsoleTokens(t *testing.T) {
	cases := []struct {
		raw, platform, base string
	}{
		{"God of War [PS2]", ConsolePS2, "God of War"},
		{"God of War (ps2)", ConsolePS2, "God of War"},
		{"God of War {PlayStation 2}", ConsolePS2, "God of War"},
		{"Crash Bandicoot [PS1]", ConsolePS1, "Crash Bandicoot"},
		{"Crash Bandicoot (PSX)", ConsolePS1, "Crash Bandicoot"},
		{"Crash Bandicoot [PS One]", ConsolePS1, "Crash Bandicoot"},
		{"Crash Bandicoot [psone]", ConsolePS1, "Crash Bandicoot"},
		{"Crash Bandicoot [PlayStation 1]", ConsolePS1, "Crash Bandicoot"},
		{"Daxter (PSP)", ConsolePSP, "Daxter"},
		{"Daxter [PlayStation Portable]", ConsolePSP, "Daxter"},
		{"Persona 3 Portable [psp]", ConsolePSP, "Persona 3 Portable"},

		{"God of War - PS2", ConsolePS2, "God of War"},
		{"God of War / PS2", ConsolePS2, "God of War"},
		{"PS2 / God of War", ConsolePS2, "God of War"},
		{"Crash Bandicoot - Sony PlayStation 2", ConsolePS2, "Crash Bandicoot"},
		{"God of War - PS2 ISO", ConsolePS2, "God of War"},
		{"God of War - PS2 (2005)", ConsolePS2, "God of War"},
		{"God of War / PS2 [RUS]", ConsolePS2, "God of War"},
		{"God of War — PSX", ConsolePS1, "God of War"},
		{"God of War | PS2 | RePack", ConsolePS2, "God of War"},
		{"God of War - PS2 - Rus", ConsolePS2, "God of War"},
		{"PS2 - God of War", ConsolePS2, "God of War"},
		{"PS2: God of War", ConsolePS2, "God of War"},
		{"[PS2] God of War", ConsolePS2, "God of War"},

		{"Crash Bandicoot (USA) [PS1]", ConsolePS1, "Crash Bandicoot"},
		{"Crash Bandicoot [PlayStation]", ConsolePS1, "Crash Bandicoot"},
		{"Crash Bandicoot (Sony PlayStation)", ConsolePS1, "Crash Bandicoot"},
		{"Crash Bandicoot [Sony PlayStation 2]", ConsolePS2, "Crash Bandicoot"},
		{"God of War [PSX-PSP]", ConsolePS1, "God of War"},
		{"God of War [PS-2]", ConsolePS2, "God of War"},
		{"God of War [PS2, RUS]", ConsolePS2, "God of War"},
		{"God of War [PS2 NTSC]", ConsolePS2, "God of War"},
		{"God of War [PS2/PSP]", ConsolePS2, "God of War"},

		{"God of War [PS3]", ConsolePS3, "God of War"},
		{"God of War (PlayStation 3)", ConsolePS3, "God of War"},
		{"God of War [PS3 PKG]", ConsolePS3, "God of War"},
		{"God of War [PS2 ISO]", ConsolePS2, "God of War"},
		{"God of War [PS2 NTSC RUS]", ConsolePS2, "God of War"},
		{"Daxter [PSP CSO]", ConsolePSP, "Daxter"},
		{"Crash Bandicoot [PS1 CHD]", ConsolePS1, "Crash Bandicoot"},
		{"Crash Bandicoot [PSX PBP]", ConsolePS1, "Crash Bandicoot"},
		{"God of War [PS-3, RUS]", ConsolePS3, "God of War"},
		{"God of War - PS3", ConsolePS3, "God of War"},
		{"PS3: God of War", ConsolePS3, "God of War"},
		{"Persona 4 Golden [PSVita]", ConsoleVita, "Persona 4 Golden"},
		{"Persona 4 Golden [PS Vita]", ConsoleVita, "Persona 4 Golden"},
		{"Persona 4 Golden [PS-Vita]", ConsoleVita, "Persona 4 Golden"},
		{"Persona 4 Golden (PlayStation Vita)", ConsoleVita, "Persona 4 Golden"},
		{"Persona 4 Golden [PSV]", ConsoleVita, "Persona 4 Golden"},
		{"Persona 4 Golden [PSV, RUS]", ConsoleVita, "Persona 4 Golden"},
		{"Persona 4 Golden [PSV VPK]", ConsoleVita, "Persona 4 Golden"},
		{"Persona 4 Golden [Sony PSV]", ConsoleVita, "Persona 4 Golden"},
		{"Persona 4 Golden - PSVita", ConsoleVita, "Persona 4 Golden"},
		{"Persona 4 Golden - PS Vita", ConsoleVita, "Persona 4 Golden"},
		{"Bloodborne [PS4]", ConsolePS4, "Bloodborne"},
		{"Bloodborne (PlayStation 4)", ConsolePS4, "Bloodborne"},
		{"Bloodborne [PS4 PKG]", ConsolePS4, "Bloodborne"},
		{"Bloodborne [PS4 FPKG]", ConsolePS4, "Bloodborne"},
		{"Uncharted - PS4", ConsolePS4, "Uncharted"},
		{"Uncharted - PS4 PKG", ConsolePS4, "Uncharted"},
		{"PS4 / Uncharted", ConsolePS4, "Uncharted"},
		{"God of War [PS3/PS4]", ConsolePS3, "God of War"},

		{"Half-Life 2 [PC, PS3]", "", "Half Life 2 [PC, PS3]"},
		{"Bloodborne [PC/PS4]", "", "Bloodborne [PC/PS4]"},
		{"Gravity Rush [Windows, PSVita]", "", "Gravity Rush [Windows, PSVita]"},
		{"Uncharted - PC / PS4", "", "Uncharted PC / PS4"},

		{"Bloodborne PS4", "", "Bloodborne PS4"},
		{"God of War PS3", "", "God of War PS3"},
		{"Gravity Rush PS Vita", "", "Gravity Rush PS Vita"},
		{"PS4 Remaster", "", "PS4 Remaster"},
		{"Final Fantasy X PS4 Remaster", "", "Final Fantasy X PS4 Remaster"},
		{"Game (PS4 Remaster)", "", "Game (PS4 Remaster)"},
		{"Game - PS4 Remaster", "", "Game PS4 Remaster"},
		{"PSVR Game", "", "PSVR Game"},
		{"Game [PSVR]", "", "Game [PSVR]"},
		{"Game [PSVRAM]", "", "Game [PSVRAM]"},
		{"Game - PSV", "", "Game PSV"},
		{"PSV - Game", "", "PSV Game"},
		{"PSV Game", "", "PSV Game"},
		{"Game PSV", "", "Game PSV"},
		{"PS4", "", "PS4"},
		{"[PSV]", "", "[PSV]"},

		// PC comes first: a bracket that names it keeps the release as it was.
		{"God of War [PC, PS2]", "", "God of War [PC, PS2]"},
		{"God of War (PC + PS2)", "", "God of War (PC + PS2)"},
		{"God of War [PC/PS2/Xbox]", "", "God of War [PC/PS2/Xbox]"},
		{"God of War [Windows, PSP]", "", "God of War [Windows, PSP]"},
		{"God of War [Win64/PS2]", "", "God of War [Win64/PS2]"},
		{"God of War - PC / PS2", "", "God of War PC / PS2"},

		// Not a console: a word at the end with no separator before it.
		{"Bloodborne PSX", "", "Bloodborne PSX"},
		{"God of War PS2", "", "God of War PS2"},
		{"God of War PlayStation 2", "", "God of War PlayStation 2"},
		{"Crash Bandicoot Sony PlayStation 2", "", "Crash Bandicoot Sony PlayStation 2"},
		{"God of War for PS2", "", "God of War for PS2"},
		{"God of War PS2 (2005)", "", "God of War PS2"},
		{"Half-PSX", "", "Half PSX"},
		// Not a console: a scene name.
		{"God.of.War.PS2-GROUP", "", "God of War PS2 GROUP"},
		// Not a console: part of a word.
		{"PSPice Simulator", "", "PSPice Simulator"},
		{"Epsilon", "", "Epsilon"},
		{"Epsilon PSPice", "", "Epsilon PSPice"},
		{"Lapsx", "", "Lapsx"},
		{"PS2я", "", "PS2я"},
		{"Game [PSPice]", "", "Game [PSPice]"},
		// Not a console: inside the words of a title.
		{"Haunted PS1 Demo Disc", "", "Haunted PS1 Demo Disc"},
		{"Cyber PS2 Remake", "", "Cyber PS2 Remake"},
		{"PS2 Remake", "", "PS2 Remake"},
		{"Game (PS2 Remake)", "", "Game (PS2 Remake)"},
		{"My PlayStation 2 Story", "", "My PlayStation 2 Story"},
		// Not a console: a number after a dot.
		{"Kingdom Hearts PS2.5", "", "Kingdom Hearts PS2.5"},
		// Not a console: nothing would be left of the name.
		{"PlayStation 2", "", "PlayStation 2"},
		{"PS2", "", "PS2"},
		{"[PS2]", "", "[PS2]"},
		// Not listed: the bare brand name is not a token.
		{"The History of PlayStation", "", "The History of PlayStation"},
		{"", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.raw, func(t *testing.T) {
			got := Parse(tc.raw)
			if got.Platform != tc.platform {
				t.Fatalf("Parse(%q).Platform = %q, want %q", tc.raw, got.Platform, tc.platform)
			}
			if got.Base != tc.base {
				t.Fatalf("Parse(%q).Base = %q, want %q", tc.raw, got.Base, tc.base)
			}
		})
	}
}

// The token must not reach the server in any spelling, the fallbacks that read
// the raw line among them.
func TestMatchNamesDropsConsoleToken(t *testing.T) {
	cases := []struct {
		raw  string
		want []string
	}{
		{"God of War [PS2]", []string{"God of War"}},
		{"God of War [PS2] (2005)", []string{"God of War"}},
		{"God of War - PS2", []string{"God of War"}},
		{"God of War / PS2", []string{"God of War"}},
		{"Daxter - Release HV [PSP]", []string{"Daxter Release HV", "Daxter"}},
		{"God of War [PS3]", []string{"God of War"}},
		{"God of War [PS3 PKG]", []string{"God of War"}},
		{"God of War [PS2 ISO]", []string{"God of War"}},
		{"God of War [PS2 NTSC RUS]", []string{"God of War"}},
		{"Daxter [PSP CSO]", []string{"Daxter"}},
		{"Crash Bandicoot [PS1 CHD]", []string{"Crash Bandicoot"}},
		{"Persona 4 Golden [PSVita]", []string{"Persona 4 Golden"}},
		{"Persona 4 Golden [PSV]", []string{"Persona 4 Golden"}},
		{"Persona 4 Golden [PSV VPK]", []string{"Persona 4 Golden"}},
		{"Bloodborne [PS4 PKG]", []string{"Bloodborne"}},
		{"Bloodborne [PS4 FPKG, RUS]", []string{"Bloodborne"}},
		{"Uncharted - PS4", []string{"Uncharted"}},
	}
	for _, tc := range cases {
		t.Run(tc.raw, func(t *testing.T) {
			got := MatchNames(tc.raw)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("MatchNames(%q) = %q, want %q", tc.raw, got, tc.want)
			}
			for _, name := range got {
				lower := strings.ToLower(name)
				for _, word := range []string{"ps2", "psp", "psx", "ps3", "psvita", "psv", "ps4", "pkg", "vpk", "iso", "cso", "chd"} {
					if strings.Contains(lower, word) {
						t.Fatalf("MatchNames(%q) leaks %q in %q", tc.raw, word, name)
					}
				}
			}
		})
	}
}

// Every string in this package's tests is a title some test already pins down.
// None of them may lose a byte to the console rules.
func TestConsoleRulesLeavePCFixturesAlone(t *testing.T) {
	paths, err := filepath.Glob("*_test.go")
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	fset := token.NewFileSet()
	for _, path := range paths {
		if path == "console_test.go" {
			continue
		}
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			raw, err := strconv.Unquote(lit.Value)
			if err != nil {
				t.Fatalf("%s: %v", lit.Value, err)
			}
			checked++
			if got, id := extractConsole(raw); got != raw || id != "" {
				t.Errorf("%s: %q became %q on console %q", fset.Position(lit.Pos()), raw, got, id)
			}
			return true
		})
	}
	if checked < 500 {
		t.Fatalf("only %d fixtures found", checked)
	}
}
