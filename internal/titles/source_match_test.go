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
		{"Cuphead v1.3.9+DLC", []string{"Cuphead"}},
		{"Hollow Knight v1.5.78.11833a+DLCs", []string{"Hollow Knight"}},
		{"RimWorld v1.5.4409+allDLCs", []string{"RimWorld"}},
		{"Bridge Constructor Portal v1.4-fix+DLC", []string{"Bridge Constructor Portal"}},
		{"Kerbal Space Program v1.12.5.03190+DLCs", []string{"Kerbal Space Program"}},
		{"Streets of Rogue v99i2+allDLC", []string{"Streets of Rogue"}},
		{"Dead Age 2 v1.118-fix", []string{"Dead Age 2"}},
		{"Eldest Souls v1.1.23f2", []string{"Eldest Souls"}},
		{"Farlanders v1.2.1f2", []string{"Farlanders"}},
		{"Combat Mission: Beyond Overlord v1.12RDNAfix", []string{"Combat Mission: Beyond Overlord"}},
		{"Cooking Simulator 2: Better Together – v1.4.6717bcc", []string{"Cooking Simulator 2: Better Together"}},
		{"Neon Abyss v1.5.0.0src+3DLC", []string{"Neon Abyss"}},
		{"Clive Barker’s Undying v1.1hotfix", []string{"Clive Barker’s Undying"}},
		{"Kingdoms and Castles v122r2a", []string{"Kingdoms and Castles"}},
		{"SpongeBob SquarePants: Battle for Bikini Bottom – Rehydrated – Rev. 603296 (Build 1)", []string{"SpongeBob SquarePants: Battle for Bikini Bottom – Rehydrated"}},
		{"Sugar Shack – v1.0.3-rev6153 + Windows 7 Fix", []string{"Sugar Shack"}},
		{"Tank Squad – v1.0 Rev 12985", []string{"Tank Squad"}},
		{"Police Chief Simulator – Rev.3871", []string{"Police Chief Simulator"}},
		{"Resident Evil 2 v1.0 hotfix5", []string{"Resident Evil 2"}},
		{"Blood West v4.6.2 rc3+DLC", []string{"Blood West"}},
		{"NINJA GAIDEN: Ragebound (cs37823)", []string{"NINJA GAIDEN: Ragebound"}},
		{"Cloudpunk – BuildID 6754726 + City of Ghosts DLC", []string{"Cloudpunk"}},
		{
			"Azur Lane Crosswave: Complete Deluxe Edition + All DLCs",
			[]string{"Azur Lane Crosswave Complete Deluxe Edition", "Azur Lane Crosswave"},
		},
		{
			"Lords of the Fallen GotY Edition",
			[]string{"Lords of the Fallen GotY Edition", "Lords of the Fallen"},
		},
		{
			"Kingdoms of Amalur: Re-Reckoning – FATE Edition – Version CS:13925/Update 11 + DLC + Bonus",
			[]string{"Kingdoms of Amalur: Re Reckoning – FATE Edition", "Kingdoms of Amalur: Re Reckoning"},
		},
		// "Rev N" with a small number and only a space is the game's own
		// subtitle, not a build revision: catalog names these games exactly
		// this way.
		{"Guilty Gear Xrd REV 2 + All DLC", []string{"Guilty Gear Xrd REV 2"}},
		{"GUILTY GEAR Xrd REV 2 — v1.02 | Portable", []string{"GUILTY GEAR Xrd REV 2"}},
		{"PayDay 2 – v1.102.954/Update 204.1 Hotfix + 106 DLCs", []string{"PayDay 2"}},
		{
			"A Total War Saga: Thrones of Britannia – v1.2.3 Build 13348.2970280 + DLC",
			[]string{"A Total War Saga: Thrones of Britannia"},
		},
		{
			"Deus Ex: Mankind Divided – Digital Deluxe Edition – v1.19 build 801.0 + All DLCs + Bonus Content (Re-repack)",
			[]string{"Deus Ex: Mankind Divided Digital Deluxe Edition", "Deus Ex: Mankind Divided"},
		},
		{
			"Men of War: Assault Squad GOTY Edition v2.05.15",
			[]string{"Men of War: Assault Squad GOTY Edition", "Men of War: Assault Squad"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.raw, func(t *testing.T) {
			if got := MatchNames(tc.raw); !reflect.DeepEqual(got, tc.names) {
				t.Fatalf("names=%q, want %q (parsed=%+v)", got, tc.names, Parse(tc.raw))
			}
		})
	}
}

// The catalog match-releases endpoint rejects a whole batch of 100 queries if
// any one of them carries more than 6 titles, so MatchNames must never grow
// past that regardless of how many edition/fallback variants it can produce.
func TestSourceMatchNamesNeverExceedsSix(t *testing.T) {
	for _, raw := range []string{
		"Triangle Strategy: Digital Deluxe Edition – BuildID 9842040 (Denuvoless) + Bonus ArtBook",
		"Kingdoms of Amalur: Re-Reckoning – FATE Edition – Version CS:13925/Update 11 + DLC + Bonus",
		"GTA 4 / Grand Theft Auto IV: The Complete Edition [v 1.2.0.43] (2010-2020) RePack от xatab",
		"Azur Lane Crosswave: Complete Deluxe Edition + All DLCs",
		"Niоh 2 The Complete Edition — RePack от Igruha",
		"The Plucky Squire (Отважный Паж) v.1.50.15 [Папка игры] (2024)",
	} {
		if got := MatchNames(raw); len(got) > maxMatchNames {
			t.Fatalf("MatchNames(%q) returned %d names, want at most %d: %q", raw, len(got), maxMatchNames, got)
		}
	}
}

// The version/build cleanup added for packaging junk must never eat a number
// or Roman numeral that is actually part of the title.
func TestSourceMatchKeepsRealTitleNumbers(t *testing.T) {
	for _, raw := range []string{
		"Construction Simulator 4",
		"Yakuza 0",
		"Cyberpunk 2077",
		"The Order: 1886",
		"Rise Eterna 2",
		"Grand Tactician: The Civil War (1861-1865)",
		"7 Days to Die",
		"Ara: History Untold",
		"Hades II",
		"STAR FLEET II - Krellan Commander Version 2.0",
		"V1RUZ",
		"V1rus Killer",
	} {
		t.Run(raw, func(t *testing.T) {
			got := MatchNames(raw)
			if len(got) != 1 || Normalize(got[0]) != Normalize(raw) {
				t.Fatalf("MatchNames(%q) = %q, want the title unchanged", raw, got)
			}
		})
	}
}

func TestSourceMatchDropsWordsButNeverCutsOne(t *testing.T) {
	for _, raw := range []string{
		"Crusader Kings v1.0 Revolution",
		"Game v1.2 Fixed Edition",
		"Hitman v1.0 Updated Edition",
		"Builder v2.1 Buildings Pack",
		"Racer v1.0 RCT Edition",
		"Icewind Dale 2 Complete v2.01_fixes",
		"Dead Age 2 v1.118-fix",
		"Sugar Shack – v1.0.3-rev6153 + Windows 7 Fix",
	} {
		t.Run(raw, func(t *testing.T) {
			words := map[string]bool{}
			for _, w := range strings.Fields(Normalize(raw)) {
				words[w] = true
			}
			for _, name := range MatchNames(raw) {
				for _, w := range strings.Fields(Normalize(name)) {
					if !words[w] {
						t.Fatalf("MatchNames(%q) = %q: %q is a cut word", raw, MatchNames(raw), w)
					}
				}
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
