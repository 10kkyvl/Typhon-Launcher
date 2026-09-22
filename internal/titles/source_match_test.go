package titles

import (
	"reflect"
	"strings"
	"testing"
)

func TestSourceReleaseMatchNames(t *testing.T) {
	cases := []struct {
		raw   string
		names []string
	}{
		{"Grand Theft Auto V Enhanced – Build 1013.20/Online 1.72", []string{"Grand Theft Auto V Enhanced"}},
		{"Grand Theft Auto V Enhanced [v. 1.0.814.9] (2025) RePack от Decepticon", []string{"Grand Theft Auto V Enhanced"}},
		{"Grand Theft Auto V Legacy v.3570.0 + 3586.0 [Папка игры (Steam)] (2015)", []string{"Grand Theft Auto V Legacy"}},
		{"Grand Theft Auto IV: The Complete Edition v.1.2.0.59 [Папка игры] (2008-2010)", []string{"Grand Theft Auto IV The Complete Edition", "Grand Theft Auto IV"}},
		{"GTA: The Trilogy – The Definitive Edition — RePack от Igruha", []string{"Grand Theft Auto: The Trilogy The Definitive Edition"}},
		{"ГТА 4 (GTA 4) — RePack от xatab", []string{"Grand Theft Auto IV"}},
		{"Grand Theft Auto V / GTA 5 (v1.0.2802/1.64 Online, MULTi13) [FitGirl Repack]", []string{"Grand Theft Auto V"}},
		{"Grand Theft Auto V (v1.0.3095/1.68 + NVE Platinum Modpack + Bonus OSTs, MULTi13) [FitGirl Repack, Selective Download - from 49.2 GB]", []string{"Grand Theft Auto V"}},
		{"9-Bit Armies: A Bit Too Far — v864547 (Build 19702616) | Архив", []string{"9 Bit Armies: A Bit Too Far"}},
		{"Half-Life 2: Episode Two v1.0.4", []string{"Half Life 2: Episode Two"}},
		{"Persona 3 Portable", []string{"Persona 3 Portable"}},
		{"Kingdom Come: Deliverance (All Stars)", []string{"Kingdom Come: Deliverance (All Stars)"}},
		{"GTA 4 / Grand Theft Auto IV - Complete Edition [v 1070-1120] (2010) PC | RePack от xatab", []string{"Grand Theft Auto IV Complete Edition", "Grand Theft Auto IV"}},
		{"Grand Theft Auto IV: The Complete Edition [v 1.2.0.43] (2010-2020) RePack от xatab", []string{"Grand Theft Auto IV The Complete Edition", "Grand Theft Auto IV"}},
		{"GTA 4 / Grand Theft Auto IV (2010) RePack от xatab", []string{"Grand Theft Auto IV"}},
		{"Grand Theft Auto III / GTA 4", []string{"Grand Theft Auto III / GTA 4"}},
		{"Little Nightmares III (2025/11/19) [Папка игры] (2025)", []string{"Little Nightmares III"}},
		{"Mortal Sin (2025/10/27)", []string{"Mortal Sin"}},
		{"TerraScape: Deluxe Edition – v2.1.0.3 + 2 DLCs/Bonuses", []string{"TerraScape Deluxe Edition", "TerraScape"}},
		{"Prince of Persia: The Lost Crown – Complete Edition, v1.4.3 + 5 DLCs + 2 OSTs", []string{"Prince of Persia: The Lost Crown Complete Edition", "Prince of Persia: The Lost Crown"}},
		{"Dark Souls Remastered, v1.0 + All DLCs + Bonus OST", []string{"Dark Souls Remastered"}},
		{"Game + Another Game II", []string{"Game + Another Game II"}},
		{"V Rising – v1.0.10.1-r82675-b2 + 7 DLCs/Bonuses + Dedicated Server + Windows 7 Fix", []string{"V Rising"}},
		{"Life is Strange: Double Exposure, v1.0.3 + Sudachi/Torzu Switch Emulators", []string{"Life is Strange: Double Exposure"}},
		{"Postal 3 – v1.3 (ZOOM Platform) + Fart Gun DLC + Bonus Content", []string{"Postal 3"}},
		{"SD Gundam: G Generation – Cross Rays + Update 1 + 7/32 DLCs", []string{"SD Gundam: G Generation – Cross Rays"}},
		{"Ara: History Untold v.2.0.2.528 [Папка игры] (2024)", []string{"Ara: History Untold"}},
		{"Unravel (2016) PC | Репак от xatab", []string{"Unravel"}},
		{"Cyberpunk 2077 Ultimate Edition v.2.12 [GOG] (2020) Лицензия", []string{"Cyberpunk 2077 Ultimate Edition", "Cyberpunk 2077"}},
		{"Ultimate Fishing Simulator 2 v.1.25.05.16:4006 [Архив] (2025)", []string{"Ultimate Fishing Simulator 2"}},
		{"Aragami: Nightfall (RUS|ENG|MULTI) [RePack] by xatab", []string{"Aragami: Nightfall"}},
		{"Frigato: Shadows of the Caribbean (+ Bonus Soundtrack, MULTi27) [FitGirl Repack]", []string{"Frigato: Shadows of the Caribbean"}},
		{"My Memory of Us – Build 16287132 (Secret Update HotFix)", []string{"My Memory of Us"}},
		{"The Plucky Squire (Отважный Паж) v.1.50.15 [Папка игры] (2024)", []string{"The Plucky Squire (Отважный Паж)", "The Plucky Squire"}},
		{"The Planet Crafter: Deluxe Bundle, v1.608 + 3 DLCs/Bonuses", []string{"The Planet Crafter: Deluxe Bundle", "The Planet Crafter"}},
		{"Space Prison: Supporter Edition + 2 DLCs", []string{"Space Prison: Supporter Edition", "Space Prison"}},
		{"GTA 4 / Grand Theft Auto IV: The Complete Edition – v1.2.0.43 + Radio Downgrader + Vanilla Fixes Modpack v1.6.2 + Wrappers", []string{"Grand Theft Auto IV The Complete Edition", "Grand Theft Auto IV"}},
		{"GTA 4: Complete Edition", []string{"Grand Theft Auto IV Complete Edition", "Grand Theft Auto IV"}},
		{"Grand Theft Auto V / GTA 5 (Legacy) – v1.0.3725.0/1.72 + Bonus Content", []string{"Grand Theft Auto V Legacy"}},
		{"Grand Theft Auto V / GTA 5 – v1.0.3411/1.70 + NVE Platinum Modpack + Bonus Content", []string{"Grand Theft Auto V"}},
		{"Grand Theft Auto V / GTA 5 Redux", []string{"Grand Theft Auto V / GTA 5 Redux"}},
		{"Age of Empires 2 (II) Definitive Edition — (Build 24094652) | P2P", []string{"Age of Empires II Definitive Edition"}},
		{"Age of Empires 3 (III) Definitive Edition — RePack от Igruha", []string{"Age of Empires III Definitive Edition"}},
		{"Age of Empires 4 (IV): Anniversary Edition — RePack от Igruha", []string{"Age of Empires IV Anniversary Edition"}},
		{"Game 2 (III) Definitive Edition", []string{"Game 2 (III) Definitive Edition"}},
		{"Дарк Соулс (III)", []string{"Дарк Соулс (III)"}},
		{"Дарк Соулс 2 (III)", []string{"Дарк Соулс 2 (III)"}},
		{"Tomb Raider IV-VI Remastered (19/09/2025) [Папка игры] (2025)", []string{"Tomb Raider IV VI Remastered"}},
		{"Warhammer 40,000: Dawn of War Definitive Edition (19.08.2025)", []string{"Warhammer 40,000: Dawn of War Definitive Edition"}},
		{"Dаys Gone Remastered — RePack от Igruha", []string{"Dаys Gone Remastered", "Days Gone Remastered"}},
		{"Niоh 2 The Complete Edition — RePack от Igruha", []string{"Niоh 2 The Complete Edition", "Nioh 2 The Complete Edition", "Niоh 2", "Nioh 2"}},
		{"BioShоck 2 Remastered", []string{"BioShоck 2 Remastered", "BioShock 2 Remastered"}},
		{"Kingdom of Velvet Сhains", []string{"Kingdom of Velvet Сhains", "Kingdom of Velvet Chains"}},
		{"ГТА Сан Андреас (GTA San Andreas) — RePack от Igruha", []string{"Grand Theft Auto San Andreas"}},
		{"Жизнь и Смерть", []string{"Жизнь и Смерть"}},
		{"GoЯ", []string{"GoЯ"}},
	}
	for _, tc := range cases {
		t.Run(tc.raw, func(t *testing.T) {
			if got := MatchNames(tc.raw); !reflect.DeepEqual(got, tc.names) {
				t.Fatalf("names=%q, want %q (parsed=%+v)", got, tc.names, Parse(tc.raw))
			}
		})
	}
}

func TestReleaseMetadataBracketsAndTrailingEdition(t *testing.T) {
	for _, marker := range []string{"v", "V"} {
		p := Parse("Example Game: Complete Edition [" + marker + " 1.2.0.43] (2025/10/27) (2010)")
		if p.Base != "Example Game" || p.Edition != "Complete Edition" || p.Version != "1.2.0.43" || p.Year != 2010 {
			t.Fatalf("metadata polluted title: %+v", p)
		}
	}
	for _, name := range []string{"Game (2025/13/01)", "Game (31/02/2025)", "Game (05.05.26)", "Game (Act 1/2)", "Game (All Stars)"} {
		if p := Parse(name); p.Base != name {
			t.Fatalf("non-date title removed: %q => %+v", name, p)
		}
	}
}

func TestReleaseBracketKeepsVersionLanguageAndDLCCount(t *testing.T) {
	p := Parse("Example Game (v1.2 + 12 DLCs + Bonus OST, MULTi13) [FitGirl Repack]")
	if p.Base != "Example Game" || p.Version != "1.2" || p.DLCCount != 12 || len(p.Languages) != 1 {
		t.Fatalf("release fields lost: %+v", p)
	}
}

func TestPackageBracketRequiresClosedContentMarker(t *testing.T) {
	for _, raw := range []string{
		"Example (Patch Quest)",
		"Example (Bonus Round)",
		"Example (+ Patchwork)",
		"Example (+ Other Game)",
		"Example (DLC Quest)",
	} {
		if got := Parse(raw); got.Base != raw {
			t.Fatalf("ordinary bracketed subtitle was removed: %q => %+v", raw, got)
		}
	}
	for raw := range map[string]struct{}{
		"Example (+ Bonus Content, MULTi8)":      {},
		"Example (+ 3 DLCs + Bonus OST, MULTi8)": {},
		"Example (+ Patch 3, MULTi8)":            {},
		"Example (Patch 3) [Папка игры]":         {},
		"Example (Build 12345) [Архив]":          {},
	} {
		if got := Parse(raw); got.Base != "Example" {
			t.Fatalf("release package bracket remained in title: %q => %+v", raw, got)
		}
	}
}

func TestSourceMatchKeepsRemasterIdentityAndRepacker(t *testing.T) {
	if got := MatchNames("Dark Souls Remastered v1.0"); len(got) != 1 || got[0] != "Dark Souls Remastered" {
		t.Fatalf("remaster collapsed: %q", got)
	}
	p := Parse("Grand Theft Auto V v1.0 RePack от xatab")
	if p.Base != "Grand Theft Auto V" || Repacker(p.Tags) != "xatab" {
		t.Fatalf("repacker lost: %+v", p)
	}
}

func TestSourceMatchStripsMixedReleaseMetadata(t *testing.T) {
	cases := []struct {
		raw  string
		want []string
	}{
		{"Sacred 2 Remaster v.CL16601 [RePack Decepticon] (2025)", []string{"Sacred 2 Remaster"}},
		{"Marvel’s Midnight Suns: Legendary Edition – Build CL-930465 + 8 DLCs/Bonuses", []string{"Marvel’s Midnight Suns Legendary Edition", "Marvel’s Midnight Suns"}},
		{"Schedule I v.0.4.2f9 [Архив] (Early Access)", []string{"Schedule I"}},
		{"AI Olympius v.0.9.15f2 [Архив] (Early Access)", []string{"AI Olympius"}},
		{"Intravenous 2 v.1.4.6HF2 [Папка игры] (2024)", []string{"Intravenous 2"}},
		{"Forts v.1.34.0r20112 [Архив] (2017)", []string{"Forts"}},
		{"Vivat Slovakia v.1.0.1b16 [Архив] (2025)", []string{"Vivat Slovakia"}},
		{"Cities: Skylines II v.1.3.5f1 [Папка игры] (2023)", []string{"Cities: Skylines II"}},
		{"Keep Driving v.1.3.1.0с [Папка игры] (2025)", []string{"Keep Driving"}},
		{"Risk of Rain 2 v.1.4.0#840 [Архив] (2020)", []string{"Risk of Rain 2"}},
		{"Rue Valley v.1.0.0v2 [Папка игры] (2025)", []string{"Rue Valley"}},
		{"Chernobylite v.48723s03dx12 [GOG] (2021) Лицензия", []string{"Chernobylite"}},
		{"Quarterstaff v1.0.0-5db267 [Папка игры] (2025)", []string{"Quarterstaff"}},
		{"Galacticare v1.1.0+e0d30dc159 [Папка игры] (2024)", []string{"Galacticare"}},
		{"Unknown 9: Awakening [v Build 16687288 + DLCs] (2024) RePack от Decepticon", []string{"Unknown 9: Awakening"}},
		{"Black Mesa: Definitive Edition [v Necro Patch.build.14113817] (2020) RePack от Decepticon", []string{"Black Mesa Definitive Edition"}},
		{"Dragon Age: The Veilguard [v Build 16179329] (2024) RePack от Decepticon", []string{"Dragon Age: The Veilguard"}},
		{"Immortal: Unchained [v Update.17 + DLCs] (2018) PC | RePack by xatab", []string{"Immortal: Unchained"}},
		{"Warhammer: Chaosbane [v.Build 28.05.2020 ] (2019) PC | RePack by xatab", []string{"Warhammer: Chaosbane"}},
		{"Bellwright (0.0.46961) [Папка игры] (Early Access)", []string{"Bellwright"}},
		{"Monster Train 2 (14193) [Папка игры] (2025)", []string{"Monster Train 2"}},
		{"Drive Beyond Horizons (20609719) [Папка игры] (Early Access)", []string{"Drive Beyond Horizons"}},
		{"Skin Deep (2025.10.02.1017) [Архив] (2025)", []string{"Skin Deep"}},
		{"Project Silverfish (Beta 0.3.8) [Папка игры] (Early Access)", []string{"Project Silverfish"}},
		{"Revenge of the Savage Planet (2025-10-27-111237 Net6) [Папка игры] (2025)", []string{"Revenge of the Savage Planet"}},
		{"Stygian: Outer Gods (32e14d5f) [Папка игры] (Early Access)", []string{"Stygian: Outer Gods"}},
		{"StarRupture (0.1.1.112941-S) [Архив] (Early Access)", []string{"StarRupture"}},
		{"SimCity 4 Deluxe Edition v.1.1.641 hotfix (25621) [GOG] (2003)", []string{"SimCity 4 Deluxe Edition", "SimCity 4"}},
	}
	for _, tc := range cases {
		t.Run(tc.raw, func(t *testing.T) {
			if got := MatchNames(tc.raw); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("names=%q, want %q (parsed=%+v)", got, tc.want, Parse(tc.raw))
			}
		})
	}
}

func TestSourceMatchPreservesSequelNumbersAndRealBrackets(t *testing.T) {
	cases := map[string][]string{
		"Game 2 (III) Definitive Edition": {"Game 2 (III) Definitive Edition"},
		"Hades 2":                         {"Hades 2"},
		"Hades II":                        {"Hades II"},
		"Monster Train 2 (2025) [Папка игры]": {"Monster Train 2"},
		"Game (14193)":                          {"Game (14193)"},
		"Kingdom Come: Deliverance (All Stars)": {"Kingdom Come: Deliverance (All Stars)"},
		"Persona 3 Portable":                    {"Persona 3 Portable"},
		"VA-11 Hall-A":                          {"VA 11 Hall A"},
		"O.V.N.I. Abduction":                    {"O V N I Abduction"},
		"V1RUZ":                                 {"V1RUZ"},
		"Grisaia Phantom Trigger Vol.8":         {"Grisaia Phantom Trigger Vol 8"},
		"NieR Replicant ver.1.22474487139":      {"NieR Replicant ver 1.22474487139"},
		"SteamWorld Build v.1.0.4 [GOG] (2023)": {"SteamWorld Build"},
	}
	for raw, want := range cases {
		t.Run(raw, func(t *testing.T) {
			if got := MatchNames(raw); !reflect.DeepEqual(got, want) {
				t.Fatalf("names=%q, want %q", got, want)
			}
		})
	}
}

func TestVersionSpaceKeepsStandaloneYear(t *testing.T) {
	if got := Parse("Game v1.0 2023"); got.Base != "Game 2023" || got.Version != "1.0" {
		t.Fatalf("standalone year was consumed as a build number: %+v", got)
	}
	if got := Parse("Game v1.0 185 531"); got.Base != "Game" || got.Version != "1.0" {
		t.Fatalf("spaced build chain was not consumed: %+v", got)
	}
}

func TestSourceMatchPreservesExecutableNamesAndClosedNumberPairs(t *testing.T) {
	for raw, want := range map[string][]string{
		"Hero.EXE":                        {"Hero EXE"},
		"Dave.EXE":                        {"Dave EXE"},
		"Hero.exe":                        {"Hero"},
		"game.torrent":                    {"game"},
		"Europa Universalis V (5) v1.0":   {"Europa Universalis V"},
		"FATAL FRAME II (2) v1.0":         {"FATAL FRAME II"},
		"A.I.L.A (AILA) v1.0":             {"A I L A"},
		"Game 2 (III) Definitive Edition": {"Game 2 (III) Definitive Edition"},
	} {
		t.Run(raw, func(t *testing.T) {
			if got := MatchNames(raw); !reflect.DeepEqual(got, want) {
				t.Fatalf("names=%q, want %q (parsed=%+v)", got, want, Parse(raw))
			}
		})
	}
}

func TestSourceMatchKeepsPackageGuardsAndCommercialIdentity(t *testing.T) {
	for raw, want := range map[string][]string{
		"Supermarket Simulator Complete Pack v1.0":                                                        {"Supermarket Simulator Complete Pack", "Supermarket Simulator"},
		"Some Game Ultimate Pack v1.0":                                                                    {"Some Game Ultimate Pack", "Some Game"},
		"Some Game Collection v1.0":                                                                       {"Some Game Collection"},
		"Game Remastered Collection v1.0":                                                                 {"Game Remastered Collection"},
		"Need for Speed: The Run Limited Edition v1.1 + DLC":                                              {"Need for Speed: The Run Limited Edition", "Need for Speed: The Run"},
		"Need for Speed: Most Wanted – Limited Edition – v1.0":                                            {"Need for Speed: Most Wanted – Limited Edition", "Need for Speed: Most Wanted"},
		"Need for Speed: Most Wanted - Limited Edition v1.0":                                              {"Need for Speed: Most Wanted Limited Edition", "Need for Speed: Most Wanted"},
		"Homura Hime: Soundtrack Bundle, v1.0.8 + Bonus OST":                                              {"Homura Hime: Soundtrack Bundle", "Homura Hime"},
		"CarX Street: Year One Edition, v1.15.0 + 7 DLCs":                                                 {"CarX Street: Year One Edition", "CarX Street"},
		"Succubus: The Worshipper Bundle [v 1.15.18327 + DLCs] (2021) RePack от Decepticon":               {"Succubus: The Worshipper Bundle", "Succubus"},
		"X4: Foundations - Community of Planets Edition [v 7.50 + DLCs] (2018) RePack от Decepticon":      {"X4: Foundations Community of Planets Edition", "X4: Foundations"},
		"Hitman: The Complete First Season - GOTY Edition [v 1.14.2 + DLC's] (2016) PC | RePack от xatab": {"Hitman: The Complete First Season GOTY Edition", "Hitman"},
		"The Sims 3: The Complete Collection [1.67.2.024017] (2009-2013) PC | RePack by xatab":            {"The Sims 3: The Complete Collection", "The Sims 3"},
		"Watchmen: The End is Nigh - Complete Collection — RePack от R.G. Механики":                       {"Watchmen: The End is Nigh Complete Collection", "Watchmen"},
		"Cult of the Lamb: The One Who Waits Edition v1.0":                                                {"Cult of the Lamb: The One Who Waits Edition", "Cult of the Lamb"},
		"Stand By v1.0":                                {"Stand By"},
		"TMNT: The Cowabunga Collection v1.0":          {"TMNT: The Cowabunga Collection"},
		"Beat 'Em Up Collection v1.0":                  {"Beat 'Em Up Collection"},
		"Aliens Versus Predator 2 (+Primal Hunt) v1.0": {"Aliens Versus Predator 2 (+Primal Hunt)"},
		"Example Game (Online)":                        {"Example Game (Online)"},
		"Example Game (Multiplayer)":                   {"Example Game (Multiplayer)"},
		"Crimson Tactics (v1.0 (Build 12345)":          {"Crimson Tactics"},
		"Crimson Tactics: The Rise of The White Banner (v1.0.0b + Bonus Content, Selective Download - from 2.7 GB]": {"Crimson Tactics: The Rise of The White Banner"},
		"Sackboy: A Big Adventure (+ 3 DLCs + Bonus OST + Online/LAN CoOp, MULTi8) [FitGirl Repack]":                {"Sackboy: A Big Adventure"},
		"Klonoa: Phantasy Reverie Series (+ Special Bundle DLC + Bonus Content, MULTi10) [FitGirl Repack]":          {"Klonoa: Phantasy Reverie Series"},
		"Снайпер Элит 2 (Sniper Elite V2) — RePack от xatab":                                                        {"Sniper Elite V2"},
		"Ghostrunner 2 - Deluxe Edition [v 42294_40 + DLCs] (2023) RePack от Decepticon":                            {"Ghostrunner 2 Deluxe Edition", "Ghostrunner 2"},
	} {
		t.Run(raw, func(t *testing.T) {
			if got := MatchNames(raw); !reflect.DeepEqual(got, want) {
				t.Fatalf("names=%q, want %q (parsed=%+v)", got, want, Parse(raw))
			}
		})
	}
	p := Parse("Hob [Steam + GOG] (2017)")
	if !reflect.DeepEqual(p.Tags, []string{"gog", "steam-rip"}) {
		t.Fatalf("source labels lost from package bracket: %+v", p)
	}
}

func TestSourceMatchAddsSafePublisherFallback(t *testing.T) {
	for raw, want := range map[string][]string{
		"Cuphead (Studio MDHR) (RUS|ENG) RePack от xatab": {
			"Cuphead (Studio MDHR)", "Cuphead",
		},
		"Assassin's Creed: Brotherhood (Ubisoft Entertainment) (RUS|RUS) [RePack] от xatab": {
			"Assassin's Creed: Brotherhood (Ubisoft Entertainment)", "Assassin's Creed: Brotherhood",
		},
		"Call of Duty: WWII - Digital Deluxe Edition (Activision) (RUS|ENG) [RePack] от xatab": {
			"Call of Duty: WWII Digital Deluxe Edition (Activision)", "Call of Duty: WWII Digital Deluxe Edition", "Call of Duty: WWII",
		},
		"Kingdom Come: Deliverance (All Stars)": {"Kingdom Come: Deliverance (All Stars)"},
	} {
		t.Run(raw, func(t *testing.T) {
			if got := MatchNames(raw); !reflect.DeepEqual(got, want) {
				t.Fatalf("names=%q, want %q (parsed=%+v)", got, want, Parse(raw))
			}
		})
	}
}

func TestSourceMatchKeepsRemasterInsideCommercialEdition(t *testing.T) {
	raw := "Neverwinter Nights: Enhanced Edition Digital Deluxe Edition"
	want := []string{"Neverwinter Nights Enhanced Edition Digital Deluxe Edition", "Neverwinter Nights Enhanced Edition"}
	if got := MatchNames(raw); !reflect.DeepEqual(got, want) {
		t.Fatalf("enhanced release fell back to original: %q", got)
	}
	if p := Parse(raw); p.Edition != "Digital Deluxe Edition" {
		t.Fatalf("matching cleanup changed stored release edition: %+v", p)
	}
	for _, identity := range []string{"Remastered", "Definitive Edition", "Director's Cut"} {
		got := MatchNames("Example " + identity + " Deluxe Edition v1.0")
		want := []string{"Example " + identity + " Deluxe Edition", "Example " + identity}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("%s identity lost: %q", identity, got)
		}
	}
	if got := MatchNames("Seven: Enhanced Collector’s Edition – v1.3.2 + Bonus Content"); !reflect.DeepEqual(got, []string{"Seven: Enhanced Collector’s Edition", "Seven: Enhanced"}) {
		t.Fatalf("identity already in base title duplicated: %q", got)
	}
}

func TestSourceMatchKeepsVersionedEditionMetadata(t *testing.T) {
	lastOfUs := Parse("The Last of Us Part II Remastered v.1.0.10402.1014 + 1.0.10407.0714 [Папка игры] (2020-2025)")
	if lastOfUs.Base != "The Last of Us Part II" || lastOfUs.Edition != "Remastered" || lastOfUs.Version != "1.0.10402.1014" || lastOfUs.Year != 2025 {
		t.Fatalf("version continuation polluted edition metadata: %+v", lastOfUs)
	}
	grandma := Parse("Grandma, No! Deluxe Edition – v20250522R + Bonus Content")
	if grandma.Base != "Grandma, No!" || grandma.Edition != "Deluxe Edition" || grandma.Version != "20250522R" {
		t.Fatalf("compact letter-suffixed version lost edition metadata: %+v", grandma)
	}
	if got := MatchNames("Grandma, No! Deluxe Edition – v20250522R + Bonus Content"); !reflect.DeepEqual(got, []string{"Grandma, No! Deluxe Edition", "Grandma, No!"}) {
		t.Fatalf("names=%q, want edition and base", got)
	}
}

func TestSourceMatchBilingualAndVersionedTitles(t *testing.T) {
	for raw, want := range map[string][]string{
		"Sniper Elite V2 Remastered — RePack от Igruha":                          {"Sniper Elite V2 Remastered"},
		"Demolish & Build 2018 — RePack от Other's":                              {"Demolish & Build 2018"},
		"Unbroken: The Awakening v.0.9.4.2.A [Архив] (Early Access)":             {"Unbroken: The Awakening"},
		"Millennia v.1.0.26357.F [Папка игры] (2024)":                            {"Millennia"},
		"UBOAT (2024.1 Patch 22) [Папка игры (Steam)] (2024)":                    {"UBOAT"},
		"UBOAT (2025.1.1 Patch 4) [Папка игры (Steam)] (2024)":                   {"UBOAT"},
		"Persona 4 Golden / Персона 4: Золотое издание [Папка игры] (2008-2020)": {"Persona 4 Golden"},
		"Персона 4: Золотое издание / Persona 4 Golden [Папка игры] (2008-2020)": {"Persona 4 Golden"},
		"HITMAN III / HITMAN World of Assassination":                             {"HITMAN III / HITMAN World of Assassination"},
		"Game One / Game Two": {"Game One / Game Two"},
	} {
		t.Run(raw, func(t *testing.T) {
			if got := MatchNames(raw); !reflect.DeepEqual(got, want) {
				t.Fatalf("names=%q, want %q", got, want)
			}
		})
	}
	// A patch-shaped bracket without a feed marker is not enough evidence
	// for this source-only cleanup to remove the complete bracket.
	const title = "Example (1.2 Patch 3)"
	if got := withoutMatchBuildBrackets(title); got != title {
		t.Fatalf("bracket without feed context removed: %q", got)
	}
}

func TestSourcePackagesNeverReplaceAnotherGameOrStandaloneAddon(t *testing.T) {
	for raw, forbidden := range map[string]string{
		"Age of Empires 2 - HD Edition Bundle — RePack от xatab":                                                        "Age of Empires 2",
		"Sleeping Dogs: Definitive + Limited Editions Pack (24/30 DLCs)":                                                "Sleeping Dogs",
		"Fallout 4: High Resolution Texture Pack (for v1.10.980.0+) [FitGirl Repack]":                                   "Fallout 4",
		"Age of Empires IV: 4K HDR Video Pack (MULTi8) [FitGirl Repack]":                                                "Age of Empires IV",
		"Middle-earth: Shadow of War - Definitive Edition - 4K Cinematics Pack Add-on for v1.21 GOG [FitGirl Repack]":   "Middle-earth: Shadow of War - Definitive Edition",
		"Warhammer 40,000: Chaos Gate – Daemonhunters: Grand Master Edition, Build a7bc905 (20865149) + 5 DLCs/Bonuses": "Warhammer 40,000: Chaos Gate",
		"Mafia II Enhanced Edition (2K Games) (ENG/RUS) [RePack] от xatab":                                              "Mafia II",
		"Metro: Exodus - Gold & Enhanced Edition's [v 1.0.8.39|3.0.8.39 + DLCs] (2019-2021) RePack от Decepticon":       "Metro: Exodus",
	} {
		for _, name := range MatchNames(raw) {
			if Normalize(name) == Normalize(forbidden) {
				t.Errorf("%q offered unrelated/incomplete identity %q: %q", raw, forbidden, MatchNames(raw))
			}
		}
	}
	for _, raw := range []string{"Wild West Legacy: Digital Supporter Edition, v1.0", "Mafia 3: Digital Deluxe Edition – v1.09 GOG"} {
		names := MatchNames(raw)
		if len(names) != 2 || strings.HasSuffix(names[1], "Digital") {
			t.Errorf("digital package left in game identity: %q -> %q", raw, names)
		}
	}
	if got := Parse("Example Build a7bc905"); got.Base != "Example" || got.Version != "a7bc905" {
		t.Fatalf("build hash left in title: %+v", got)
	}
	if got := Parse("Build Defaced"); got.Base != "Build Defaced" {
		t.Fatalf("title word read as build hash: %+v", got)
	}
}
