package titles

import (
	"reflect"
	"strings"
	"testing"
)

func hasMatchName(names []string, want string) bool {
	for _, name := range names {
		if Normalize(name) == Normalize(want) {
			return true
		}
	}
	return false
}

// Each row is a game title with version, build, edition or packaging noise.
// The clean title must be offered by MatchNames, after the spellings the exact
// lookup already had.
var fallbackCorpus = []struct{ raw, want string }{
	// text after " +"
	{"Alien Shooter + Expansions v1.2", "Alien Shooter"},
	{"Beholder + Blissful Sleep v1.6.0", "Beholder"},
	{"Hard West + Scars of Freedom v1.5", "Hard West"},
	{"Descent 3 + Mercenary v1.4", "Descent 3"},
	{"Delta Force: Land Warrior + Task Force Dagger", "Delta Force: Land Warrior"},
	{"Leap of Faith + Official Walkthrough", "Leap of Faith"},
	{"Edna & Harvey: The Breakout + Harvey’s New Eyes", "Edna & Harvey: The Breakout"},
	{"Din’s Curse + Demon War", "Din’s Curse"},
	{"Overlord + Raising Hell", "Overlord"},
	{"Torchlight II + Embers", "Torchlight II"},
	{"Dungeon’s Legion +UNRATED", "Dungeon’s Legion"},
	{"Love n Life: Lucky Teacher v3.3.0+Unrated", "Love n Life: Lucky Teacher"},
	{"Apostle: Rebellion +Unrated", "Apostle: Rebellion"},
	{"The Scarlet Demonslayer v5+Unrated", "The Scarlet Demonslayer"},
	{"Samurai Vandalism v4+Unrated", "Samurai Vandalism"},
	// trailing version or build bracket
	{"Resident Evil 2 (1.0-v4)", "Resident Evil 2"},
	{"Dead Space (b192-v2)", "Dead Space"},
	{"Hades (RC2)", "Hades"},
	{"Prey (Patch12)", "Prey"},
	{"Control (release-53)", "Control"},
	{"Alan Wake (1.0.4)", "Alan Wake"},
	{"Metro Exodus (build 12345)", "Metro Exodus"},
	{"Dishonored (v1.4-hotfix)", "Dishonored"},
	{"Inside (GOG)", "Inside"},
	{"Limbo (Steam)", "Limbo"},
	{"Braid (1.0.2 GOG)", "Braid"},
	{"Hotline Miami (2.1-b3)", "Hotline Miami"},
	{"Fez (r42)", "Fez"},
	{"Ori and the Blind Forest (1.0-v2) (GOG)", "Ori and the Blind Forest"},
	{"Subnautica [b192-v2]", "Subnautica"},
	{"Stray (release-12 hf2)", "Stray"},
	{"Cuphead (1.3.4-rc1)", "Cuphead"},
	{"Celeste (build 3)", "Celeste"},
	{"Firewatch (latest)", "Firewatch"},
	{"Oxenfree (4.0.1)", "Oxenfree"},
	{"The Witness (1.0-v7)", "The Witness"},
	{"Dead Cells (patch 4)", "Dead Cells"},
	// labels after a dash
	{"Resident Evil 4 - Release HV", "Resident Evil 4"},
	{"Metro 2033 – Non-HV x64", "Metro 2033"},
	{"Alan Wake – Steam", "Alan Wake"},
	{"Doom Eternal – Complete", "Doom Eternal"},
	{"Sleeping Dogs - Release x86", "Sleeping Dogs"},
	{"Dishonored 2 - HV", "Dishonored 2"},
	{"Mad Max - Release", "Mad Max"},
	{"Hitman 3 – GOG", "Hitman 3"},
	{"Quantum Break – Steam x64", "Quantum Break"},
	{"Prey – Complete Release", "Prey"},
	{"Control - v1.2.3", "Control"},
	{"Tomb Raider - Release HV x64", "Tomb Raider"},
	{"Observer – Non-HV", "Observer"},
	{"Layers of Fear — Steam", "Layers of Fear"},
	{"The Evil Within - Release x64", "The Evil Within"},
	{"Hellblade - Complete", "Hellblade"},
	{"Titanfall 2 – Release HV Non-HV", "Titanfall 2"},
	{"Wolfenstein: The New Order - Steam GOG", "Wolfenstein: The New Order"},
	{"Sniper Elite 4 – Complete x64", "Sniper Elite 4"},
	{"Dirt 5 - Release", "Dirt 5"},
	// episodes and seasons
	{"Tales from the Borderlands Episode 5", "Tales from the Borderlands"},
	{"The Walking Dead (Episodes 1-5)", "The Walking Dead"},
	{"Life is Strange Complete Season", "Life is Strange"},
	{"Deltarune Chapter 2", "Deltarune"},
	{"Batman: The Telltale Series - Complete Season", "Batman: The Telltale Series"},
	{"The Wolf Among Us Episode 3", "The Wolf Among Us"},
	{"Minecraft: Story Mode Episodes 1-8", "Minecraft: Story Mode"},
	{"Game of Thrones Chapter 4", "Game of Thrones"},
	{"Back to the Future Episode 1-5", "Back to the Future"},
	{"Sam & Max Complete Season 2", "Sam & Max"},
	{"The Walking Dead: Season Two Episode 4", "The Walking Dead: Season Two"},
	{"Resident Evil Revelations Episode 2 v1.0", "Resident Evil Revelations"},
	{"Hitman: Blood Money - Chapter 3", "Hitman: Blood Money"},
	{"Strange Brigade: Chapters 1-3", "Strange Brigade"},
	{"Jagged Alliance Complete Season", "Jagged Alliance"},
	// two titles on one line
	{"Bastion / Transistor", "Bastion"},
	{"Bastion / Transistor", "Transistor"},
	{"Hitman Absolution / Hitman Sniper Challenge", "Hitman Absolution"},
	{"Left 4 Dead 2 / L4D2", "Left 4 Dead 2"},
	{"Max Payne 3 / Макс Пейн 3", "Max Payne 3"},
	{"Portal / Portal 2 v1.0", "Portal 2"},
	{"Doom / Doom II", "Doom II"},
	{"Quake / Quake II - Release HV", "Quake"},
	{"Fate / Grand Order", "Grand Order"},
	// a Russian subtitle glued to the title
	{"The Movies Фабрика грёз", "The Movies"},
	{"Prey Добыча", "Prey"},
	{"Max Payne 3 Макс Пейн", "Max Payne 3"},
	{"Metro Exodus Исход", "Metro Exodus"},
	{"Alan Wake Пробуждение v1.0", "Alan Wake"},
	{"Hitman Blood Money Кровавые деньги", "Hitman Blood Money"},
	{"Mirror's Edge Catalyst Зеркальный край", "Mirror's Edge Catalyst"},
	{"Dishonored Бесчестье", "Dishonored"},
	// trailing dictionary words
	{"Dying Light HF2", "Dying Light"},
	{"Dying Light HF", "Dying Light"},
	{"Hades PTBR", "Hades"},
	{"Prey TestKS", "Prey"},
	{"Control Denuvoless", "Control"},
	{"Metro Exodus CrackFix", "Metro Exodus"},
	{"Doom Eternal Release", "Doom Eternal"},
	{"Stronghold Crusader Gold", "Stronghold Crusader"},
	{"Halo Wars Complete", "Halo Wars"},
	{"Prey Deluxe", "Prey"},
	{"Sacred Gold", "Sacred"},
	{"Worms Armageddon Complete v1.0", "Worms Armageddon"},
	{"Cossacks Gold v1.2", "Cossacks"},
	{"Max Payne 3 Complete", "Max Payne 3"},
	{"Painkiller Gold Edition", "Painkiller"},
	{"Painkiller Deluxe Edition", "Painkiller"},
	{"Evil Genius Deluxe v1.1", "Evil Genius"},
	// lookalike letters
	{"NBA 2К25", "NBA 2K25"},
	{"FIFA 2З", "FIFA 2З"},
	{"Left 4 Dеad 2", "Left 4 Dead 2"},
	{"Biοshock Infinite", "Bioshock Infinite"},
	{"ΒioShock 2", "BioShock 2"},
	{"Crysis 2Κ", "Crysis 2Κ"},
	{"Ηalf-Life 2", "Half-Life 2"},
	{"Skyrim Specιal Edition", "Skyrim Special Edition"},
	// combined noise
	{"Fallout 4 + Far Harbor (b192-v2)", "Fallout 4"},
	{"Hitman 2 (v1.0-hotfix) - Release HV", "Hitman 2"},
	{"Witcher 3 – Complete + DLC Pack (1.31)", "Witcher 3"},
	{"Torchlight Episode 2 (1.0-v4)", "Torchlight"},
	{"Prey / Prey 2 (RC2)", "Prey"},
	{"Doom 3 + Resurrection of Evil – Release", "Doom 3"},
	{"Dishonored HF2 - Steam", "Dishonored"},
	{"Hades Gold (b192-v2)", "Hades"},
	{"Control Complete (1.0-v4)", "Control"},
	{"Dead Island Definitive Edition (1.0-v4)", "Dead Island Definitive Edition"},
}

func TestFallbackCorpusOffersCleanTitle(t *testing.T) {
	for _, tc := range fallbackCorpus {
		t.Run(tc.raw+"=>"+tc.want, func(t *testing.T) {
			got := MatchNames(tc.raw)
			if !hasMatchName(got, tc.want) {
				t.Fatalf("MatchNames(%q) = %q, want %q among them", tc.raw, got, tc.want)
			}
			if len(got) > maxMatchNames {
				t.Fatalf("MatchNames(%q) returned %d names", tc.raw, len(got))
			}
		})
	}
}

func TestFallbackKeepsExactSpellingFirst(t *testing.T) {
	cases := []struct{ raw, first string }{
		{"PAC-MAN MUSEUM+", "PAC-MAN MUSEUM+"},
		{"Mario + Rabbids Kingdom Battle", "Mario + Rabbids Kingdom Battle"},
		{"Alien Shooter + Expansions v1.2", "Alien Shooter + Expansions"},
		{"The Binding of Isaac: Rebirth - Repentance", "The Binding of Isaac: Rebirth Repentance"},
		{"Knights and Merchants: Remake", "Knights and Merchants: Remake"},
		{"Campaign/Zombies", "Campaign/Zombies"},
		{"Resident Evil 4 - Release HV", "Resident Evil 4 Release HV"},
		{"Agony Unrated", "Agony Unrated"},
		{"Shatter Remastered Deluxe", "Shatter Remastered Deluxe"},
	}
	for _, tc := range cases {
		t.Run(tc.raw, func(t *testing.T) {
			got := MatchNames(tc.raw)
			if len(got) == 0 || Normalize(got[0]) != Normalize(tc.first) {
				t.Fatalf("MatchNames(%q) = %q, want %q first", tc.raw, got, tc.first)
			}
		})
	}
}

func TestFallbackNeverCutsRealTitleWords(t *testing.T) {
	cases := []struct {
		raw    string
		absent []string
	}{
		{"Campaign/Zombies", []string{"Zombies", "Campaign"}},
		{"PAC-MAN MUSEUM+", []string{"PAC MAN"}},
		{"The Binding of Isaac: Rebirth - Repentance", []string{"The Binding of Isaac", "The Binding of Isaac: Rebirth"}},
		{"Knights and Merchants: Remake", []string{"Knights and Merchants"}},
		{"Hades II", []string{"Hades"}},
		{"Game (3D)", []string{"Game"}},
		{"Sniper Elite V2", []string{"Sniper Elite"}},
	}
	for _, tc := range cases {
		t.Run(tc.raw, func(t *testing.T) {
			got := MatchNames(tc.raw)
			for _, name := range tc.absent {
				if hasMatchName(got, name) {
					t.Fatalf("MatchNames(%q) = %q, must not offer %q", tc.raw, got, name)
				}
			}
		})
	}
}

func TestFallbackEmptyAndJunkOnly(t *testing.T) {
	for _, raw := range []string{"", "   ", "(1.0-v4)", "v1.2.3", "- Release HV", "HV", "+", " + Bonus", "[b192-v2]", "Episode 3", "(RC2) - Release"} {
		t.Run(raw, func(t *testing.T) {
			got := MatchNames(raw)
			if len(got) > maxMatchNames {
				t.Fatalf("MatchNames(%q) = %q", raw, got)
			}
			for _, name := range got {
				if strings.TrimSpace(name) == "" {
					t.Fatalf("MatchNames(%q) = %q has an empty name", raw, got)
				}
			}
		})
	}
	if got := MatchNames(""); len(got) != 0 {
		t.Fatalf("MatchNames(\"\") = %q", got)
	}
}

// Older spellings keep their places; newcomers are the ones the limit cuts.
func TestFallbackDoesNotDisplaceExistingSpellings(t *testing.T) {
	const raw = "Niоh 2 The Complete Edition + Extra Mission (b192-v2) / Nioh 2 Gold"
	got := MatchNames(raw)
	if len(got) > maxMatchNames {
		t.Fatalf("MatchNames(%q) returned %d names: %q", raw, len(got), got)
	}
	const exact = "Niоh 2 The Complete Edition — RePack от Igruha"
	want := []string{"Niоh 2 The Complete Edition", "Nioh 2 The Complete Edition", "Niоh 2", "Nioh 2"}
	if got := MatchNames(exact); !reflect.DeepEqual(got, want) {
		t.Fatalf("MatchNames(%q) = %q, want %q", exact, got, want)
	}
}

func TestFallbackManyVariantsStayWithinLimit(t *testing.T) {
	for _, raw := range []string{
		"Triangle Strategy: Digital Deluxe Edition – BuildID 9842040 (Denuvoless) + Bonus ArtBook (b192-v2) - Release HV",
		"Dаys Gone Remastered / Days Gone Episode 3 + Extra (1.0-v4)",
		"Niоh 2 The Complete Edition + Extra / Nioh 2 Gold Edition Complete (RC2) - Steam",
		"A / B / C / D / E / F / G / H Gold Complete Deluxe (b192-v2)",
	} {
		got := MatchNames(raw)
		if len(got) > maxMatchNames {
			t.Fatalf("MatchNames(%q) returned %d names: %q", raw, len(got), got)
		}
		seen := map[string]bool{}
		for _, name := range got {
			if seen[Normalize(name)] {
				t.Fatalf("MatchNames(%q) repeats %q: %q", raw, name, got)
			}
			seen[Normalize(name)] = true
		}
	}
}

func TestRepairLookalikeGlyphs(t *testing.T) {
	cases := []struct{ in, want string }{
		{"NBA 2К25", "NBA 2K25"},
		{"2К25", "2K25"},
		{"Fifa 2Н3", "Fifa 2H3"},
		{"Biοshock", "Bioshock"},
		{"ΒioShock", "BioShock"},
		{"Ηalf-Life", "Half-Life"},
		{"Жизнь и Смерть", "Жизнь и Смерть"},
		{"Ελλάδα", "Ελλάδα"},
		{"К25", "К25"},
		{"2К", "2К"},
		{"", ""},
	}
	for _, tc := range cases {
		if got := repairLookalikeGlyphs(tc.in); got != tc.want {
			t.Errorf("repairLookalikeGlyphs(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestCutAtPlus(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Game + Bonus", "Game"},
		{"PAC-MAN MUSEUM+", "PAC-MAN MUSEUM+"},
		{"Game (A + B)", "Game (A + B)"},
		{"Game +", "Game +"},
		{"+ Bonus", "+ Bonus"},
		{"A + B", "A + B"},
		{"", ""},
	}
	for _, tc := range cases {
		if got := cutAtPlus(tc.in); got != tc.want {
			t.Errorf("cutAtPlus(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
