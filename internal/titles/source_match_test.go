package titles

import (
	"reflect"
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
