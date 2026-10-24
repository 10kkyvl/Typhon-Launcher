package crashlog

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func readTestdata(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestParseEvent(t *testing.T) {
	f, err := ParseEvent(readTestdata(t, "app_error_1000.xml"))
	if err != nil {
		t.Fatal(err)
	}
	want := Fault{
		Source: SourceAppError, App: "Game.exe", Module: "MSVCP140.dll", ModuleVersion: "14.38.33135.0",
		ModulePath: `C:\Games\Foo\MSVCP140.dll`, ExceptionCode: 0xC0000005, HasException: true,
		Time: time.Date(2026, 10, 24, 10, 0, 3, 123456700, time.UTC),
	}
	if f != want {
		t.Fatalf("got %+v want %+v", f, want)
	}

	f, err = ParseEvent(readTestdata(t, "wer_1001.xml"))
	if err != nil {
		t.Fatal(err)
	}
	if f.Source != SourceWER || f.App != "Game.exe" || f.Module != "d3dx9_43.dll" ||
		f.ModuleVersion != "9.29.952.3111" || f.ExceptionCode != 0xC0000005 || !f.HasException || f.ModulePath != "" {
		t.Fatalf("wer fault: %+v", f)
	}
}

func TestParseEventRejects(t *testing.T) {
	good := string(readTestdata(t, "app_error_1000.xml"))
	cases := []struct {
		name string
		xml  []byte
		want error
	}{
		{"empty", nil, ErrMalformedEvent},
		{"broken", []byte("<Event><System>"), ErrMalformedEvent},
		{"too large", []byte(strings.Repeat("a", maxEventXML+1)), ErrEventTooLarge},
		{"other provider", []byte(strings.Replace(good, "Application Error", "Other", 1)), ErrNotCrashEvent},
		{"wrong id", []byte(strings.Replace(good, ">1000<", ">1002<", 1)), ErrNotCrashEvent},
		{"few fields", []byte(`<Event><System><Provider Name='Application Error'/><EventID>1000</EventID></System><EventData><Data>a.exe</Data></EventData></Event>`), ErrMalformedEvent},
		{"bad time", []byte(strings.Replace(good, "2026-10-24T10:00:03.1234567Z", "yesterday", 1)), ErrMalformedEvent},
		{"wer other bucket", []byte(strings.Replace(string(readTestdata(t, "wer_1001.xml")), "APPCRASH", "BEX", 1)), ErrNotCrashEvent},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ParseEvent(tc.xml); !errors.Is(err, tc.want) {
				t.Fatalf("got %v want %v", err, tc.want)
			}
		})
	}
}

func TestParseEventCleansNames(t *testing.T) {
	x := `<Event><System><Provider Name='Application Error'/><EventID>1000</EventID></System><EventData>` +
		`<Data>x.exe</Data><Data>1</Data><Data>1</Data><Data>C:\evil\dir\mod` + "\t" + `.dll</Data><Data>1</Data><Data>1</Data><Data>zz</Data></EventData></Event>`
	f, err := ParseEvent([]byte(x))
	if err != nil {
		t.Fatal(err)
	}
	if f.Module != "mod.dll" {
		t.Fatalf("module %q", f.Module)
	}
	if f.HasException {
		t.Fatal("garbage exception code must be reported as unknown")
	}
	unknown := strings.Replace(x, `C:\evil\dir\mod`+"\t"+`.dll`, "unknown", 1)
	if f, err = ParseEvent([]byte(unknown)); err != nil || f.Module != "" {
		t.Fatalf("unknown module: %+v %v", f, err)
	}
}

func TestClassifyModule(t *testing.T) {
	cases := map[string]string{
		"MSVCP140.dll": HintVCRedist, "vcruntime140_1.dll": HintVCRedist, "msvcr100.dll": HintVCRedist,
		"msvcr71.dll": HintVCRedist, "concrt140.dll": HintVCRedist, "vcomp140.dll": HintVCRedist,
		"mfc140u.dll": HintVCRedist, "ucrtbase.dll": HintVCRedist,
		"d3dx9_43.dll": HintDirectX, "D3DX10_43.dll": HintDirectX, "d3dx11_43.dll": HintDirectX,
		"xinput1_3.dll": HintDirectX, "XAudio2_7.dll": HintDirectX, "x3daudio1_7.dll": HintDirectX,
		"d3dcompiler_43.dll": HintDirectX, "xactengine3_7.dll": HintDirectX,
		"d3d8.dll": HintCompat, "d3d9.dll": HintCompat, "ddraw.dll": HintCompat,
		"glide3x.dll": HintCompat, "dinput.dll": HintCompat,
		"msvcrt.dll": "", "d3d11.dll": "", "KERNELBASE.dll": "", "ntdll.dll": "", "": "", "  ": "",
		"dinput8.dll": "", "Game.exe": "",
	}
	for module, want := range cases {
		if got := ClassifyModule(module); got != want {
			t.Errorf("ClassifyModule(%q) = %q want %q", module, got, want)
		}
	}
}

func TestDiagnose(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "Game.exe")
	if err := os.WriteFile(exe, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	present := filepath.Join(dir, "present.dll")
	if err := os.WriteFile(present, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name    string
		in      DiagnoseInput
		hint    string
		missing string
	}{
		{"exe vanished", DiagnoseInput{ExePath: filepath.Join(dir, "Gone.exe"), GameDir: dir, Module: "msvcp140.dll"}, HintAntivirus, "Gone.exe"},
		{"module vanished from game dir", DiagnoseInput{ExePath: exe, GameDir: dir, Module: "helper.dll", ModulePath: filepath.Join(dir, "sub", "helper.dll")}, HintAntivirus, "helper.dll"},
		{"module present in game dir", DiagnoseInput{ExePath: exe, GameDir: dir, Module: "present.dll", ModulePath: present}, HintUnknown, ""},
		{"missing module outside game dir", DiagnoseInput{ExePath: exe, GameDir: dir, Module: "gone.dll", ModulePath: filepath.Join(filepath.Dir(dir), "gone.dll")}, HintUnknown, ""},
		{"vcredist", DiagnoseInput{ExePath: exe, GameDir: dir, Module: "MSVCP140.dll"}, HintVCRedist, ""},
		{"directx", DiagnoseInput{ExePath: exe, Module: "d3dx9_43.dll"}, HintDirectX, ""},
		{"compat", DiagnoseInput{ExePath: exe, Module: "ddraw.dll"}, HintCompat, ""},
		{"dll not found without module", DiagnoseInput{ExePath: exe, ReasonCode: "dll_not_found"}, HintRuntimes, ""},
		{"dll not found with module", DiagnoseInput{ExePath: exe, Module: "x.dll", ReasonCode: "dll_not_found"}, HintUnknown, ""},
		{"empty module", DiagnoseInput{ExePath: exe}, HintUnknown, ""},
		{"no exe path", DiagnoseInput{Module: "vcruntime140.dll"}, HintVCRedist, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Diagnose(tc.in)
			if err != nil {
				t.Fatal(err)
			}
			if got.Hint != tc.hint || got.MissingFile != tc.missing {
				t.Fatalf("got %+v want hint %q missing %q", got, tc.hint, tc.missing)
			}
		})
	}
}

type fakeClock struct {
	now   time.Time
	waits []time.Duration
}

func (c *fakeClock) Now() time.Time { return c.now }
func (c *fakeClock) After(d time.Duration) <-chan time.Time {
	c.waits = append(c.waits, d)
	c.now = c.now.Add(d)
	ch := make(chan time.Time, 1)
	ch <- c.now
	return ch
}

func TestAwaitPollsUntilEventAppears(t *testing.T) {
	clock := &fakeClock{now: time.Unix(0, 0)}
	calls := 0
	src := func(context.Context, Query) ([]Fault, error) {
		calls++
		if calls < 3 {
			return nil, nil
		}
		return []Fault{{Source: SourceWER, App: "game.exe", Module: "a.dll"}}, nil
	}
	f, ok, err := Await(t.Context(), src, clock, Query{Exe: "Game.exe"}, DefaultOptions)
	if err != nil || !ok || f.Module != "a.dll" {
		t.Fatalf("got %+v %v %v", f, ok, err)
	}
	if calls != 3 || len(clock.waits) != 2 {
		t.Fatalf("calls %d waits %v", calls, clock.waits)
	}
}

func TestAwaitGivesUpAfterMaxWait(t *testing.T) {
	clock := &fakeClock{now: time.Unix(0, 0)}
	calls := 0
	src := func(context.Context, Query) ([]Fault, error) { calls++; return nil, nil }
	_, ok, err := Await(t.Context(), src, clock, Query{Exe: "g.exe"}, DefaultOptions)
	if ok || err != nil {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if calls != 6 {
		t.Fatalf("calls %d, clock advanced %v", calls, clock.now.Sub(time.Unix(0, 0)))
	}
}

func TestAwaitZeroWaitQueriesOnce(t *testing.T) {
	clock := &fakeClock{now: time.Unix(0, 0)}
	calls := 0
	src := func(context.Context, Query) ([]Fault, error) { calls++; return nil, nil }
	if _, ok, err := Await(t.Context(), src, clock, Query{Exe: "g.exe"}, Options{Interval: time.Second}); ok || err != nil || calls != 1 {
		t.Fatalf("ok=%v err=%v calls=%d", ok, err, calls)
	}
}

func TestAwaitErrors(t *testing.T) {
	clock := &fakeClock{now: time.Unix(0, 0)}
	boom := errors.New("access denied")
	calls := 0
	failing := func(context.Context, Query) ([]Fault, error) { calls++; return nil, boom }
	if _, ok, err := Await(t.Context(), failing, clock, Query{Exe: "g.exe"}, DefaultOptions); ok || !errors.Is(err, boom) {
		t.Fatalf("read failure must surface: ok=%v err=%v", ok, err)
	}
	unsupported := func(context.Context, Query) ([]Fault, error) { return nil, ErrUnsupported }
	clock = &fakeClock{now: time.Unix(0, 0)}
	if _, _, err := Await(t.Context(), unsupported, clock, Query{Exe: "g.exe"}, DefaultOptions); !errors.Is(err, ErrUnsupported) || len(clock.waits) != 0 {
		t.Fatalf("unsupported must not poll: %v %v", err, clock.waits)
	}
	if _, _, err := Await(t.Context(), failing, clock, Query{Exe: " "}, DefaultOptions); !errors.Is(err, ErrNoExe) {
		t.Fatalf("empty exe: %v", err)
	}
}

func TestAwaitCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	calls := 0
	src := func(context.Context, Query) ([]Fault, error) { calls++; return nil, nil }
	_, ok, err := Await(ctx, src, &fakeClock{now: time.Unix(0, 0)}, Query{Exe: "g.exe"}, DefaultOptions)
	if ok || !errors.Is(err, context.Canceled) || calls != 0 {
		t.Fatalf("ok=%v err=%v calls=%d", ok, err, calls)
	}

	ctx, cancel = context.WithCancel(t.Context())
	blocked := &blockingClock{fakeClock: fakeClock{now: time.Unix(0, 0)}}
	go cancel()
	_, ok, err = Await(ctx, src, blocked, Query{Exe: "g.exe"}, DefaultOptions)
	if ok || !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel while waiting: ok=%v err=%v", ok, err)
	}
}

type blockingClock struct{ fakeClock }

func (c *blockingClock) After(time.Duration) <-chan time.Time { return nil }

func TestPickPrefersApplicationError(t *testing.T) {
	t0 := time.Unix(100, 0)
	faults := []Fault{
		{Source: SourceWER, App: "game.exe", Module: "wer", Time: t0.Add(time.Minute)},
		{Source: SourceAppError, App: "GAME.EXE", Module: "old", Time: t0},
		{Source: SourceAppError, App: "game.exe", Module: "new", Time: t0.Add(time.Second)},
		{Source: SourceAppError, App: "other.exe", Module: "other", Time: t0.Add(time.Hour)},
	}
	f, ok := Pick(faults, "Game.exe")
	if !ok || f.Module != "new" {
		t.Fatalf("got %+v %v", f, ok)
	}
	if _, ok := Pick(faults, "none.exe"); ok {
		t.Fatal("unrelated exe matched")
	}
}

func TestFindDump(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	write := func(rel string, mod time.Time) {
		t.Helper()
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(p, mod, mod); err != nil {
			t.Fatal(err)
		}
	}
	write("Game.exe.1234.dmp", now)
	write("other.exe.99.dmp", now)
	write("a/b/deep.dmp", now)
	write("x/y/z/q/toodeep.dmp", now)
	write("old/stale.dmp", now.Add(-time.Hour))
	write("note.txt", now)

	from, to := now.Add(-time.Minute), now.Add(time.Minute)
	cases := []struct {
		name string
		dir  DumpDir
		exe  string
		want bool
	}{
		{"match exe", DumpDir{Path: root, MatchExe: true, MaxDepth: 1}, "game.exe", true},
		{"other exe", DumpDir{Path: root, MatchExe: true, MaxDepth: 0}, "missing.exe", false},
		{"any dump at depth 2", DumpDir{Path: filepath.Join(root, "a"), MaxDepth: 2}, "x.exe", true},
		{"too deep", DumpDir{Path: filepath.Join(root, "x"), MaxDepth: 2}, "x.exe", false},
		{"stale", DumpDir{Path: filepath.Join(root, "old"), MaxDepth: 2}, "x.exe", false},
		{"missing dir", DumpDir{Path: filepath.Join(root, "nope"), MaxDepth: 2}, "x.exe", false},
		{"empty path", DumpDir{}, "x.exe", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := FindDump(t.Context(), []DumpDir{tc.dir}, tc.exe, from, to)
			if err != nil || got != tc.want {
				t.Fatalf("got %v err %v want %v", got, err, tc.want)
			}
		})
	}

	if _, err := FindDump(t.Context(), []DumpDir{{Path: root, MaxDepth: 2}}, "game.exe", now.Add(time.Hour), now.Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := FindDump(ctx, []DumpDir{{Path: root, MaxDepth: 2}}, "game.exe", from, to); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled ctx: %v", err)
	}
}
