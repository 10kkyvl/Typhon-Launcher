//go:build darwin && !devmock

package shortcut

import (
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSupportedDarwin(t *testing.T) {
	if !Supported() {
		t.Fatal("Supported() = false on darwin, want true")
	}
}

func TestDesktopDirDarwin(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	if _, err := DesktopDir(); err == nil {
		t.Fatal("DesktopDir() err = nil, want error when Desktop is missing")
	}

	desktop := filepath.Join(tmp, "Desktop")
	if err := os.Mkdir(desktop, 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := DesktopDir()
	if err != nil {
		t.Fatalf("DesktopDir() err = %v", err)
	}
	if got != desktop {
		t.Fatalf("DesktopDir() = %q, want %q", got, desktop)
	}
}

func TestCreateEmptyPath(t *testing.T) {
	if err := Create("", Link{Target: "/bin/true"}); err == nil {
		t.Fatal("Create with empty path: want error, got nil")
	}
}

func TestCreateEmptyTarget(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Game.app")
	if err := Create(path, Link{}); err == nil {
		t.Fatal("Create with empty Target: want error, got nil")
	}
}

func TestCreateMissingParentDir(t *testing.T) {
	path := filepath.Join(t.TempDir(), "no-such-subdir", "Game.app")
	if err := Create(path, Link{Target: "/bin/true"}); err == nil {
		t.Fatal("Create into missing directory: want error, got nil")
	}
}

// TestCreateWritesRealBundle проверяет структуру бандла: Info.plist с
// нужными ключами и исполняемый файл с битом запуска по правильному пути.
func TestCreateWritesRealBundle(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "Half-Life 2.app")
	link := Link{
		Target:      "/usr/bin/true",
		Description: "Half-Life 2 shortcut",
	}
	if err := Create(path, link); err != nil {
		t.Fatalf("Create: %v", err)
	}

	plistPath := filepath.Join(path, "Contents", "Info.plist")
	raw, err := os.ReadFile(plistPath)
	if err != nil {
		t.Fatalf("read Info.plist: %v", err)
	}
	plist := string(raw)
	for _, want := range []string{
		"<key>CFBundleName</key>\n\t<string>Half-Life 2</string>",
		"<key>CFBundleExecutable</key>\n\t<string>Half-Life 2</string>",
		"<key>CFBundlePackageType</key>\n\t<string>APPL</string>",
		"<key>NSHighResolutionCapable</key>\n\t<true/>",
	} {
		if !strings.Contains(plist, want) {
			t.Fatalf("Info.plist missing %q, got:\n%s", want, plist)
		}
	}

	execPath := filepath.Join(path, "Contents", "MacOS", "Half-Life 2")
	info, err := os.Stat(execPath)
	if err != nil {
		t.Fatalf("stat executable: %v", err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("executable mode = %v, want 0755", info.Mode().Perm())
	}

	// Повторный Create с тем же path перезаписывает оба файла на месте, не
	// оставляя старый бандл в промежуточном состоянии.
	if err := Create(path, link); err != nil {
		t.Fatalf("second Create (overwrite): %v", err)
	}
}

// TestCreateEscapesQuotesAndSpacesEndToEnd — ключевой тест на инъекцию:
// Target лежит в каталоге с одинарной кавычкой и пробелом в имени.
// Полученный .app реально запускается, а не только сверяется строкой,
// чтобы доказать, что скрипт не сломан и не открывает shell-инъекцию.
func TestCreateEscapesQuotesAndSpacesEndToEnd(t *testing.T) {
	tmp := t.TempDir()
	targetDir := filepath.Join(tmp, "Alice's Game Files")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatal(err)
	}

	record := filepath.Join(tmp, "record.txt")
	targetScript := filepath.Join(targetDir, "run 'me'.sh")
	recorderBody := "#!/bin/sh\n" +
		"pwd > " + shellQuote(record) + "\n" +
		"for a in \"$@\"; do printf '%s\\n' \"$a\" >> " + shellQuote(record) + "; done\n"
	if err := os.WriteFile(targetScript, []byte(recorderBody), 0o755); err != nil {
		t.Fatal(err)
	}

	link := Link{
		Target:  targetScript,
		Args:    `--play "id with spaces" --flag`,
		WorkDir: targetDir,
	}
	appPath := filepath.Join(tmp, "Game.app")
	if err := Create(appPath, link); err != nil {
		t.Fatalf("Create: %v", err)
	}

	execPath := filepath.Join(appPath, "Contents", "MacOS", "Game")
	//nolint:gosec // G204: execPath собран этим же тестом из t.TempDir(), внешнего ввода нет
	cmd := exec.Command(execPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("running generated bundle failed: %v, output: %s", err, out)
	}

	raw, err := os.ReadFile(record)
	if err != nil {
		t.Fatalf("read record: %v", err)
	}
	lines := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	if len(lines) < 4 {
		t.Fatalf("record has too few lines: %q", lines)
	}

	gotDir, err := filepath.EvalSymlinks(lines[0])
	if err != nil {
		t.Fatalf("eval symlinks of recorded pwd: %v", err)
	}
	wantDir, err := filepath.EvalSymlinks(targetDir)
	if err != nil {
		t.Fatalf("eval symlinks of targetDir: %v", err)
	}
	if gotDir != wantDir {
		t.Fatalf("cwd = %q, want %q (WorkDir not honored)", gotDir, wantDir)
	}

	gotArgs := lines[1:]
	wantArgs := []string{"--play", "id with spaces", "--flag"}
	if len(gotArgs) != len(wantArgs) {
		t.Fatalf("args = %q, want %q", gotArgs, wantArgs)
	}
	for i := range wantArgs {
		if gotArgs[i] != wantArgs[i] {
			t.Fatalf("args[%d] = %q, want %q (full: %q)", i, gotArgs[i], wantArgs[i], gotArgs)
		}
	}
}

// TestShellQuoteRoundTripsThroughRealShell не сверяет экранированную строку
// с руками посчитанным литералом (легко ошибиться), а реально прогоняет её
// через /bin/sh: shellQuote(s), напечатанный echo/printf-ом, должен вернуть
// исходный s байт в байт, включая кавычки, обратные слэши и пробелы.
func TestShellQuoteRoundTripsThroughRealShell(t *testing.T) {
	cases := []string{
		"plain",
		"has space",
		"it's here",
		"''",
		"",
		`a\b`,
		"multiple ' quotes ' in ' a row",
		"leading'",
		"'trailing",
	}
	for _, in := range cases {
		script := "printf '%s' " + shellQuote(in)
		//nolint:gosec // G204: это и есть предмет теста — shellQuote(in) должен обезвредить in для sh -c, in — фиксированные тестовые строки, не внешний ввод
		out, err := exec.Command("/bin/sh", "-c", script).Output()
		if err != nil {
			t.Fatalf("shellQuote(%q): running %q: %v", in, script, err)
		}
		if string(out) != in {
			t.Fatalf("shellQuote(%q) round-tripped to %q", in, string(out))
		}
	}
}

func TestSplitArgsGroupsQuotedSpaces(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"--play abc123", []string{"--play", "abc123"}},
		{`--launch "Test Game"`, []string{"--launch", "Test Game"}},
		{"  extra   spaces  ", []string{"extra", "spaces"}},
		{`"only quoted"`, []string{"only quoted"}},
	}
	for _, tc := range cases {
		got := splitArgs(tc.in)
		if len(got) != len(tc.want) {
			t.Fatalf("splitArgs(%q) = %q, want %q", tc.in, got, tc.want)
		}
		for i := range tc.want {
			if got[i] != tc.want[i] {
				t.Fatalf("splitArgs(%q) = %q, want %q", tc.in, got, tc.want)
			}
		}
	}
}

// TestBundleInfoPlistEscapesXML проверяет, что символы, запрещённые в XML
// (& < > " '), попадают в Info.plist только в виде сущностей: имя бандла
// берётся из названия игры, которое может быть чем угодно, в отличие от
// FileName, здесь ничего не отсекается.
func TestBundleInfoPlistEscapesXML(t *testing.T) {
	name := `A&B<C>"D'`
	plist := bundleInfoPlist(name)
	want := "&amp;B&lt;C&gt;&quot;D&apos;"
	if !strings.Contains(plist, want) {
		t.Fatalf("Info.plist = %s, want it to contain %q", plist, want)
	}
	if strings.Contains(plist, `<string>A&B`) {
		t.Fatalf("Info.plist contains unescaped ampersand: %s", plist)
	}
}

func TestCreateNameWithXMLSpecialCharsOnDisk(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, `A&B<C>"D'.app`)
	if err := Create(path, Link{Target: "/usr/bin/true"}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := os.Stat(filepath.Join(path, "Contents", "MacOS", `A&B<C>"D'`)); err != nil {
		t.Fatalf("stat executable with special chars in name: %v", err)
	}
}

func TestRemoveThenStatBundle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Game.app")
	if err := Create(path, Link{Target: "/usr/bin/true"}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := Remove(path); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("bundle still present after Remove: %v", err)
	}
}
