package library

import "testing"

func TestRebaseUninstallCommand(t *testing.T) {
	const (
		oldRoot = `C:\Games\Foo`
		newRoot = `D:\Library\Foo`
	)
	tests := []struct {
		name    string
		command string
		oldRoot string
		want    string
	}{
		{"quoted path with arguments", `"C:\Games\Foo\unins000.exe" /SILENT`, oldRoot, `"D:\Library\Foo\unins000.exe" /SILENT`},
		{"unquoted path at the start", `C:\Games\Foo\unins000.exe /VERYSILENT`, oldRoot, `D:\Library\Foo\unins000.exe /VERYSILENT`},
		{"match ignores case", `"c:\games\FOO\unins000.exe"`, oldRoot, `"D:\Library\Foo\unins000.exe"`},
		{"path in an argument after an equals sign", `uninstall.exe /LOG=C:\Games\Foo\log.txt`, oldRoot, `uninstall.exe /LOG=D:\Library\Foo\log.txt`},
		{"every occurrence is rebased", `"C:\Games\Foo\a.exe" /log "C:\Games\Foo\log.txt"`, oldRoot, `"D:\Library\Foo\a.exe" /log "D:\Library\Foo\log.txt"`},
		{"trailing separator on the old root", `"C:\Games\Foo\unins000.exe"`, oldRoot + `\`, `"D:\Library\Foo\unins000.exe"`},
		{"sibling directory with the same prefix is left alone", `"C:\Games\Foo2\unins000.exe"`, oldRoot, `"C:\Games\Foo2\unins000.exe"`},
		{"same name nested deeper is left alone", `"E:\Backup\C:\Games\Foo\unins000.exe"`, oldRoot, `"E:\Backup\C:\Games\Foo\unins000.exe"`},
		{"msi product code has no path", `MsiExec.exe /X{8F1E2A3B-0000-0000-0000-000000000000}`, oldRoot, `MsiExec.exe /X{8F1E2A3B-0000-0000-0000-000000000000}`},
		{"empty old root changes nothing", `"C:\Games\Foo\unins000.exe"`, "", `"C:\Games\Foo\unins000.exe"`},
		{"empty command stays empty", "", oldRoot, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := rebaseUninstallCommand(tc.command, tc.oldRoot, newRoot); got != tc.want {
				t.Fatalf("rebaseUninstallCommand(%q) = %q, want %q", tc.command, got, tc.want)
			}
		})
	}
}
