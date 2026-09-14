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
