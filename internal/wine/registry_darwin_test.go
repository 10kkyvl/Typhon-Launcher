package wine

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseUninstallReadsBothHives(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "system.reg"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	got := parseUninstall(string(data))
	if len(got) != 2 {
		t.Fatalf("parseUninstall = %d entries, want 2: %+v", len(got), got)
	}

	byKey := map[string]UninstallEntry{}
	for _, e := range got {
		byKey[e.Key] = e
	}
	inno, ok := byKey["Inno Setup 6_is1"]
	if !ok {
		t.Fatalf("Wow6432Node entry missing: %+v", byKey)
	}
	if inno.DisplayName != "Inno Setup version 6.7.3" {
		t.Fatalf("DisplayName = %q", inno.DisplayName)
	}
	if inno.Command != `"T:\DemoGame\InnoSetup\unins000.exe"` {
		t.Fatalf("Command = %q", inno.Command)
	}
	if inno.QuietCommand != `"T:\DemoGame\InnoSetup\unins000.exe" /SILENT` {
		t.Fatalf("QuietCommand = %q", inno.QuietCommand)
	}
	if inno.InstallLocation != `T:\DemoGame\InnoSetup\` {
		t.Fatalf("InstallLocation = %q", inno.InstallLocation)
	}
	if inno.SystemComponent {
		t.Fatal("SystemComponent = true for a normal program")
	}

	cx, ok := byKey["CXHTML"]
	if !ok {
		t.Fatal("64-bit hive entry missing")
	}
	if !cx.SystemComponent {
		t.Fatal("SystemComponent = false, want true")
	}
}

func TestParseUninstallIgnoresOtherKeys(t *testing.T) {
	got := parseUninstall("[Software\\\\Classes\\\\.iss] 1\n@=\"InnoSetupScriptFile\"\n")
	if len(got) != 0 {
		t.Fatalf("parseUninstall = %+v, want none", got)
	}
}

func TestUninstallEntriesReadsBottle(t *testing.T) {
	path := fakeBottle(t)
	data, err := os.ReadFile(filepath.Join("testdata", "system.reg"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	//nolint:gosec // G703: путь целиком из t.TempDir(), внешнего ввода в нём нет
	if err := os.WriteFile(filepath.Join(path, "system.reg"), data, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got, err := Bottle{Path: path}.UninstallEntries()
	if err != nil {
		t.Fatalf("UninstallEntries: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("UninstallEntries = %d, want 2", len(got))
	}
}

func TestUninstallEntriesMissingFile(t *testing.T) {
	got, err := Bottle{Path: fakeBottle(t)}.UninstallEntries()
	if err != nil {
		t.Fatalf("UninstallEntries without system.reg: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %+v, want none", got)
	}
}
