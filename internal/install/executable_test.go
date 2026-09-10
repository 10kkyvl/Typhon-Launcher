package install

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestFindExecutablesPrefersGame(t *testing.T) {
	root := t.TempDir()
	mkFile(t, filepath.Join(root, "Game.exe"), 2<<20)
	mkFile(t, filepath.Join(root, "unins000.exe"), 4<<20)
	mkFile(t, filepath.Join(root, "UnityCrashHandler64.exe"), 3<<20)
	mkFile(t, filepath.Join(root, "vc_redist.x64.exe"), 8<<20)
	mkFile(t, filepath.Join(root, "_CommonRedist", "DXSETUP.exe"), 6<<20)
	mkFile(t, filepath.Join(root, "UE4PrereqSetup_x64.exe"), 5<<20)

	got := mustFind(t, root, "Game")
	if len(got) != 1 {
		t.Fatalf("candidates = %+v, want only Game.exe", got)
	}
	if filepath.Base(got[0].Path) != "Game.exe" {
		t.Fatalf("top = %s, want Game.exe", got[0].Path)
	}
	for _, c := range got {
		lower := strings.ToLower(filepath.Base(c.Path))
		for _, bad := range []string{"unins", "crash", "redist", "dxsetup", "prereq"} {
			if strings.Contains(lower, bad) {
				t.Fatalf("excluded exe surfaced: %s", c.Path)
			}
		}
	}
}

func TestFindExecutablesTitleBoost(t *testing.T) {
	root := t.TempDir()
	mkFile(t, filepath.Join(root, "Witcher3.exe"), 1<<20)
	mkFile(t, filepath.Join(root, "tool.exe"), 1<<20)

	got := mustFind(t, root, "The Witcher 3: Wild Hunt")
	if len(got) != 2 {
		t.Fatalf("candidates = %+v", got)
	}
	if filepath.Base(got[0].Path) != "Witcher3.exe" {
		t.Fatalf("top = %s, want Witcher3.exe", got[0].Path)
	}
	if got[0].Score <= got[1].Score {
		t.Fatalf("scores = %v, %v", got[0].Score, got[1].Score)
	}
}

func TestFindExecutablesPrefersShallowAndBinDirs(t *testing.T) {
	root := t.TempDir()
	mkFile(t, filepath.Join(root, "Binaries", "Win64", "Shooter.exe"), 1<<20)
	mkFile(t, filepath.Join(root, "third_party", "tools", "misc", "thing.exe"), 1<<20)

	got := mustFind(t, root, "Shooter")
	if len(got) != 2 || filepath.Base(got[0].Path) != "Shooter.exe" {
		t.Fatalf("candidates = %+v", got)
	}
}

func TestHighConfidence(t *testing.T) {
	if HighConfidence(nil) {
		t.Fatal("empty candidates must not be high confidence")
	}
	if !HighConfidence([]Candidate{{Path: "a", Score: 90}}) {
		t.Fatal("single strong candidate must be high confidence")
	}
	if HighConfidence([]Candidate{{Path: "a", Score: 40}}) {
		t.Fatal("weak candidate must not be high confidence")
	}
	if HighConfidence([]Candidate{{Path: "a", Score: 90}, {Path: "b", Score: 80}}) {
		t.Fatal("close runner-up must not be high confidence")
	}
	if !HighConfidence([]Candidate{{Path: "a", Score: 90}, {Path: "b", Score: 50}}) {
		t.Fatal("clear winner must be high confidence")
	}
}

func TestHighConfidenceOnScanResults(t *testing.T) {
	strong := t.TempDir()
	mkFile(t, filepath.Join(strong, "Game.exe"), 2<<20)
	if !HighConfidence(mustFind(t, strong, "Game")) {
		t.Fatalf("expected high confidence, got %+v", mustFind(t, strong, "Game"))
	}

	weak := t.TempDir()
	mkFile(t, filepath.Join(weak, "alpha.exe"), 2<<20)
	mkFile(t, filepath.Join(weak, "beta.exe"), 2<<20)
	if HighConfidence(mustFind(t, weak, "Something Else")) {
		t.Fatalf("expected low confidence, got %+v", mustFind(t, weak, "Something Else"))
	}
}

func TestExcludedExe(t *testing.T) {
	excluded := []string{
		"unins000", "uninstall", "UnityCrashHandler64", "CrashReportClient",
		"vc_redist.x64", "vcredist_x86", "DXSETUP", "dxwebsetup", "oalinst",
		"dotNetFx45_Full_setup", "setup", "Install", "Updater", "cleanup",
		"UE4PrereqSetup_x64", "launcher-helper", "activation",
		"crashpad_handler", "UnrealCEFSubProcess", "EasyAntiCheat_Setup", "BEService",
		"vcruntime140", "GameSetup",
	}
	for _, name := range excluded {
		if !excludedExe(name) {
			t.Fatalf("%s should be excluded", name)
		}
	}
	allowed := []string{"Game", "Witcher3", "eldenring", "GTA5", "Cyberpunk2077", "bin_win64"}
	for _, name := range allowed {
		if excludedExe(name) {
			t.Fatalf("%s should not be excluded", name)
		}
	}
}

func TestFindExecutablesPairsWithEngineData(t *testing.T) {
	root := t.TempDir()
	mkFile(t, filepath.Join(root, "runme.exe"), 1<<20)
	mkFile(t, filepath.Join(root, "runme_Data", "level0"), 1<<20)
	mkFile(t, filepath.Join(root, "tools.exe"), 8<<20)

	got := mustFind(t, root, "Совсем другое название")
	if filepath.Base(got[0].Path) != "runme.exe" {
		t.Fatalf("top = %s, want the exe paired with runme_Data", got[0].Path)
	}
	if !HighConfidence(got) {
		t.Fatalf("engine data next to the exe must give high confidence: %+v", got)
	}
}

func TestFindExecutablesPairsWithGodotPack(t *testing.T) {
	root := t.TempDir()
	mkFile(t, filepath.Join(root, "runme.exe"), 1<<20)
	mkFile(t, filepath.Join(root, "runme.pck"), 40<<20)
	mkFile(t, filepath.Join(root, "tools.exe"), 8<<20)

	got := mustFind(t, root, "Совсем другое название")
	if filepath.Base(got[0].Path) != "runme.exe" {
		t.Fatalf("top = %s, want the exe paired with runme.pck", got[0].Path)
	}
}

func TestFindExecutablesPrefers64Bit(t *testing.T) {
	root := t.TempDir()
	// win32 сортируется раньше x64: без разведения по разрядности победил бы
	// он, а не 64-битная сборка.
	mkFile(t, filepath.Join(root, "bin", "win32", "Game.exe"), 2<<20)
	mkFile(t, filepath.Join(root, "bin", "x64", "Game.exe"), 2<<20)

	got := mustFind(t, root, "Game")
	if len(got) != 2 {
		t.Fatalf("candidates = %+v", got)
	}
	if !strings.Contains(filepath.ToSlash(got[0].Path), "/x64/") {
		t.Fatalf("top = %s, want the 64-bit build", got[0].Path)
	}
}

func TestFindExecutablesScoresShippingBuild(t *testing.T) {
	root := t.TempDir()
	mkFile(t, filepath.Join(root, "Shooter", "Binaries", "Win64", "Shooter-Win64-Shipping.exe"), 2<<20)
	mkFile(t, filepath.Join(root, "startme.exe"), 2<<20)

	got := mustFind(t, root, "Shooter")
	if filepath.Base(got[0].Path) != "Shooter-Win64-Shipping.exe" {
		t.Fatalf("top = %s, want the shipping build", got[0].Path)
	}
	if !HighConfidence(got) {
		t.Fatalf("shipping build must win clearly: %+v", got)
	}
}

func TestFindExecutablesSkipsRedistDirs(t *testing.T) {
	root := t.TempDir()
	mkFile(t, filepath.Join(root, "Game.exe"), 2<<20)
	mkFile(t, filepath.Join(root, "_CommonRedist", "runtime.exe"), 9<<20)
	mkFile(t, filepath.Join(root, "EasyAntiCheat", "start.exe"), 9<<20)
	mkFile(t, filepath.Join(root, "$PLUGINSDIR", "app.exe"), 9<<20)

	got := mustFind(t, root, "Game")
	if len(got) != 1 || filepath.Base(got[0].Path) != "Game.exe" {
		t.Fatalf("candidates = %+v, want only Game.exe", got)
	}
}

func TestFindExecutablesLooksInsideDataDirOnlyForPairs(t *testing.T) {
	root := t.TempDir()
	mkFile(t, filepath.Join(root, "runme.exe"), 1<<20)
	mkFile(t, filepath.Join(root, "runme_Data", "Plugins", "tool.exe"), 9<<20)

	got := mustFind(t, root, "runme")
	if len(got) != 1 || filepath.Base(got[0].Path) != "runme.exe" {
		t.Fatalf("candidates = %+v, want only runme.exe", got)
	}
}

func TestFindExecutablesSurvivesAnUnreadableSubdir(t *testing.T) {
	root := t.TempDir()
	mkFile(t, filepath.Join(root, "Game.exe"), 2<<20)
	mkFile(t, filepath.Join(root, "locked", "hidden.exe"), 1<<20)
	requireUnreadableDir(t, filepath.Join(root, "locked"))

	got := mustFind(t, root, "Game")
	if len(got) != 1 || filepath.Base(got[0].Path) != "Game.exe" {
		t.Fatalf("candidates = %+v, want Game.exe found despite the locked directory", got)
	}
}

func TestFindExecutablesFailsWhenEverythingIsUnreadable(t *testing.T) {
	root := t.TempDir()
	mkFile(t, filepath.Join(root, "locked", "Game.exe"), 2<<20)
	requireUnreadableDir(t, filepath.Join(root, "locked"))

	got, err := FindExecutables(context.Background(), root, "Game")
	if err == nil {
		t.Fatalf("FindExecutables = %+v, want an error rather than an empty answer", got)
	}
	if got != nil {
		t.Fatalf("candidates = %+v, want none alongside the error", got)
	}
}

// Раскладка Retro Gadgets: игра в корне, рядом служебный агент на том же
// движке и с тем же именем файла, плюс вспомогательное приложение, чьё имя
// похоже на название игры. До разведения одинаковых имён верх списка
// занимало RetroPlayground.exe, лаунчер подставлял его по умолчанию, и
// пользователь запускал не игру.
func TestFindExecutablesPicksTheGameNotItsAgent(t *testing.T) {
	root := t.TempDir()
	mkFile(t, filepath.Join(root, "RG.exe"), 640<<10)
	mkFile(t, filepath.Join(root, "RG_Data", "data.unity3d"), 8<<20)
	mkFile(t, filepath.Join(root, "UnityCrashHandler64.exe"), 1<<20)
	mkFile(t, filepath.Join(root, "PlaygroundAgent", "RG.exe"), 640<<10)
	mkFile(t, filepath.Join(root, "PlaygroundAgent", "RG_Data", "data.unity3d"), 8<<20)
	mkFile(t, filepath.Join(root, "RetroPlayground", "RetroPlayground.exe"), 3<<20)
	mkFile(t, filepath.Join(root, "RetroPlayground", "CefSharp.BrowserSubprocess.exe"), 1<<20)
	mkFile(t, filepath.Join(root, "RetroPlayground", "steamcmd", "steamcmd.exe"), 4<<20)
	mkFile(t, filepath.Join(root, "RetroProtocolHandler", "RetroProtocolHandler.exe"), 1<<20)

	got := mustFind(t, root, "Retro Gadgets")
	if filepath.Base(got[0].Path) != "RG.exe" || filepath.Dir(got[0].Path) != root {
		t.Fatalf("top = %s, want the RG.exe next to RG_Data in the root", got[0].Path)
	}
	if !HighConfidence(got) {
		t.Fatalf("выбор должен быть уверенным, иначе лаунчер снова спросит: %+v", got)
	}
}

func TestTrimArch(t *testing.T) {
	cases := map[string]string{
		"shooterwin64shipping": "shooter",
		"gamex64":              "game",
		"game":                 "game",
		"gta5":                 "gta5",
	}
	for in, want := range cases {
		if got := trimArch(in); got != want {
			t.Fatalf("trimArch(%q) = %q, want %q", in, got, want)
		}
	}
}

func mustFind(t *testing.T, root, title string) []Candidate {
	t.Helper()
	got, err := FindExecutables(context.Background(), root, title)
	if err != nil {
		t.Fatalf("FindExecutables(%s): %v", root, err)
	}
	return got
}

func TestClientOutranksServerAndEditor(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"ClientG64.exe", "InstanceServerG64.exe", "Public_PGTerrainEditor64.exe", "ModTools/GUIEditor.exe"} {
		mkFile(t, filepath.Join(root, name), 20<<20)
	}
	got := mustFind(t, root, "9-Bit Armies")
	if filepath.Base(got[0].Path) != "ClientG64.exe" || !HighConfidence(got) {
		t.Fatalf("ambiguous tools: %+v", got)
	}
}
