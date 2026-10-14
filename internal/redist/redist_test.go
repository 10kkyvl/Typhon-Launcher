package redist

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

func touch(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("stub"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func scanNames(t *testing.T, root string) []string {
	t.Helper()
	pkgs, err := Scan(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(pkgs))
	for _, p := range pkgs {
		rel, err := filepath.Rel(root, p.Path)
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, filepath.ToSlash(rel))
	}
	sort.Strings(names)
	return names
}

func TestScanFindsRedistByName(t *testing.T) {
	root := t.TempDir()
	for _, rel := range []string{
		"_CommonRedist/vcredist/2013/vcredist_x64.exe",
		"redist/VC_redist.x86.exe",
		"DirectX/DXSETUP.exe",
		"prerequisites/dxwebsetup.exe",
		"dotnet/ndp48-web.exe",
		"support/windowsdesktop-runtime-8.0.1-win-x64.exe",
		"support/dotnet-runtime-6.0.0-win-x64.exe",
		"Redistributables/oalinst.exe",
		"redist/PhysX_9.21.msi",
		"redist/readme.txt",
		"redist/game.exe",
		"vcredist_x64.exe",
		"bin/vcredist_x64.exe",
		"game.exe",
	} {
		touch(t, filepath.Join(root, filepath.FromSlash(rel)))
	}
	want := []string{
		"DirectX/DXSETUP.exe",
		"Redistributables/oalinst.exe",
		"_CommonRedist/vcredist/2013/vcredist_x64.exe",
		"dotnet/ndp48-web.exe",
		"prerequisites/dxwebsetup.exe",
		"redist/PhysX_9.21.msi",
		"redist/VC_redist.x86.exe",
		"support/dotnet-runtime-6.0.0-win-x64.exe",
		"support/windowsdesktop-runtime-8.0.1-win-x64.exe",
	}
	sort.Strings(want)
	if got := scanNames(t, root); !reflect.DeepEqual(got, want) {
		t.Fatalf("scan = %v, want %v", got, want)
	}
}

func TestScanFillsKindArchVersionSize(t *testing.T) {
	root := t.TempDir()
	touch(t, filepath.Join(root, "redist", "vcredist2010_x86.exe"))
	pkgs, err := Scan(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if len(pkgs) != 1 {
		t.Fatalf("scan = %v", pkgs)
	}
	p := pkgs[0]
	if p.Kind != KindVCRedist || p.Arch != "x86" || p.Version != "2010" || p.SizeBytes != 4 || p.Name != "vcredist2010_x86.exe" {
		t.Fatalf("package = %+v", p)
	}
}

func TestScanDepthLimit(t *testing.T) {
	root := t.TempDir()
	touch(t, filepath.Join(root, "a", "b", "c", "redist", "vcredist_x64.exe"))
	touch(t, filepath.Join(root, "a", "b", "c", "d", "redist", "vcredist_x86.exe"))
	want := []string{"a/b/c/redist/vcredist_x64.exe"}
	if got := scanNames(t, root); !reflect.DeepEqual(got, want) {
		t.Fatalf("scan = %v, want %v", got, want)
	}
}

func TestScanRejectsBadInput(t *testing.T) {
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	root := t.TempDir()
	touch(t, filepath.Join(root, "redist", "vcredist_x64.exe"))
	cases := []struct {
		name string
		ctx  context.Context
		dir  string
		want error
	}{
		{"empty dir", context.Background(), "", ErrEmptyInstallDir},
		{"blank dir", context.Background(), "  ", ErrEmptyInstallDir},
		{"canceled", canceled, root, context.Canceled},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Scan(tc.ctx, tc.dir); !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}
	t.Run("missing root", func(t *testing.T) {
		if _, err := Scan(context.Background(), filepath.Join(root, "nope")); err == nil {
			t.Fatal("a missing install dir must be an error")
		}
	})
}

func TestSilentArgs(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		name string
		pkg  Package
		want []string
		err  error
	}{
		{"vc 2015 no year", Package{Kind: KindVCRedist, Path: filepath.Join(dir, "vcredist_x64.exe")}, []string{"/install", "/quiet", "/norestart"}, nil},
		{"vc_redist", Package{Kind: KindVCRedist, Path: filepath.Join(dir, "VC_redist.x64.exe")}, []string{"/install", "/quiet", "/norestart"}, nil},
		{"vc 2022", Package{Kind: KindVCRedist, Path: filepath.Join(dir, "vcredist2022_x64.exe")}, []string{"/install", "/quiet", "/norestart"}, nil},
		{"vc 2013", Package{Kind: KindVCRedist, Path: filepath.Join(dir, "vcredist2013_x86.exe")}, []string{"/q", "/norestart"}, nil},
		{"vc 2012", Package{Kind: KindVCRedist, Path: filepath.Join(dir, "vcredist2012_x86.exe")}, []string{"/q", "/norestart"}, nil},
		{"vc 2010", Package{Kind: KindVCRedist, Path: filepath.Join(dir, "vcredist2010_x86.exe")}, []string{"/q", "/norestart"}, nil},
		{"vc 2008", Package{Kind: KindVCRedist, Path: filepath.Join(dir, "vcredist2008_x86.exe")}, []string{"/q"}, nil},
		{"vc 2005", Package{Kind: KindVCRedist, Path: filepath.Join(dir, "vcredist2005_x86.exe")}, []string{"/q"}, nil},
		{"vc year in dir", Package{Kind: KindVCRedist, Path: filepath.Join(dir, "2010", "vcredist_x86.exe")}, []string{"/q", "/norestart"}, nil},
		{"dxsetup", Package{Kind: KindDirectX, Path: filepath.Join(dir, "DXSETUP.exe")}, []string{"/silent"}, nil},
		{"dotnet", Package{Kind: KindDotNet, Path: filepath.Join(dir, "ndp48-web.exe")}, []string{"/q", "/norestart"}, nil},
		{"openal", Package{Kind: KindOther, Path: filepath.Join(dir, "oalinst.exe")}, []string{"/s"}, nil},
		{"msi", Package{Kind: KindOther, Path: filepath.Join(dir, "PhysX.msi")}, []string{"/i", filepath.Join(dir, "PhysX.msi"), "/qn", "/norestart"}, nil},
		{"unknown other", Package{Kind: KindOther, Path: filepath.Join(dir, "PhysX_setup.exe")}, nil, ErrNoSilentArgs},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := silentArgs(tc.pkg)
			if !errors.Is(err, tc.err) {
				t.Fatalf("err = %v, want %v", err, tc.err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("args = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestInterpretExit(t *testing.T) {
	cases := []struct {
		code    int
		reboot  bool
		already bool
		fails   bool
	}{
		{0, false, false, false},
		{3010, true, false, false},
		{1638, false, true, false},
		{1603, false, false, true},
		{-1073741515, false, false, true},
	}
	for _, tc := range cases {
		res, err := interpretExit(tc.code)
		var exitErr *ExitError
		if tc.fails != errors.As(err, &exitErr) {
			t.Fatalf("code %d: err = %v", tc.code, err)
		}
		if res.RebootRequired != tc.reboot || res.AlreadyInstalled != tc.already || res.ExitCode != tc.code {
			t.Fatalf("code %d: result = %+v", tc.code, res)
		}
	}
}

func TestInstallRefusesWithoutLaunching(t *testing.T) {
	launched := 0
	prev := run
	run = func(context.Context, string, []string, string) (int, error) {
		launched++
		return 0, nil
	}
	t.Cleanup(func() { run = prev })

	dir := t.TempDir()
	dirPath := filepath.Join(dir, "vcredist_x64.exe")
	if err := os.MkdirAll(dirPath, 0o755); err != nil {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	cases := []struct {
		name string
		ctx  context.Context
		pkg  Package
		want error
	}{
		{"relative path", context.Background(), Package{Kind: KindVCRedist, Path: "vcredist_x64.exe"}, ErrNotAbsolute},
		{"missing file", context.Background(), Package{Kind: KindVCRedist, Path: filepath.Join(dir, "missing.exe")}, ErrNotAFile},
		{"directory", context.Background(), Package{Kind: KindVCRedist, Path: dirPath}, ErrNotAFile},
		{"canceled", canceled, Package{Kind: KindVCRedist, Path: dirPath}, context.Canceled},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Install(tc.ctx, tc.pkg); !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}
	unknown := filepath.Join(dir, "setup.exe")
	touch(t, unknown)
	if _, err := Install(context.Background(), Package{Kind: KindOther, Path: unknown}); !errors.Is(err, ErrNoSilentArgs) {
		t.Fatalf("err = %v, want ErrNoSilentArgs", err)
	}
	if launched != 0 {
		t.Fatalf("installer launched %d times for refused input", launched)
	}
}

func TestInstallRunsKnownPackage(t *testing.T) {
	var gotArgs []string
	prev := run
	run = func(_ context.Context, exe string, args []string, workDir string) (int, error) {
		gotArgs = append([]string{exe}, args...)
		return 3010, nil
	}
	t.Cleanup(func() { run = prev })

	path := filepath.Join(t.TempDir(), "vcredist2010_x64.exe")
	touch(t, path)
	res, err := Install(context.Background(), Package{Kind: KindVCRedist, Path: path})
	if err != nil {
		t.Fatal(err)
	}
	if !res.RebootRequired {
		t.Fatalf("result = %+v", res)
	}
	if want := []string{path, "/q", "/norestart"}; !reflect.DeepEqual(gotArgs, want) {
		t.Fatalf("launch = %v, want %v", gotArgs, want)
	}
}

func TestParseVCDisplayName(t *testing.T) {
	cases := []struct {
		display string
		want    Runtime
		ok      bool
	}{
		{"Microsoft Visual C++ 2010  x64 Redistributable - 10.0.40219", Runtime{Name: "Visual C++ 2010", Version: "10.0.40219", Arch: "x64"}, true},
		{"Microsoft Visual C++ 2008 Redistributable - x86 9.0.30729.6161", Runtime{Name: "Visual C++ 2008", Version: "10.0.40219", Arch: "x86"}, true},
		{"Microsoft Visual C++ 2013 Redistributable (x64) - 12.0.40664", Runtime{Name: "Visual C++ 2013", Version: "10.0.40219", Arch: "x64"}, true},
		{"Microsoft Visual C++ 2015-2022 Redistributable (x64) - 14.38.33135", Runtime{}, false},
		{"Microsoft Visual Studio Code", Runtime{}, false},
		{"", Runtime{}, false},
	}
	for _, tc := range cases {
		got, ok := parseVCDisplayName(tc.display, "10.0.40219")
		if ok != tc.ok || got != tc.want {
			t.Errorf("parse(%q) = %+v, %v; want %+v, %v", tc.display, got, ok, tc.want, tc.ok)
		}
	}
}

func TestOfficialLinksAreHTTPSMicrosoft(t *testing.T) {
	links := OfficialLinks()
	if len(links) != 4 {
		t.Fatalf("links = %v", links)
	}
	for _, l := range links {
		if len(l.URL) < 8 || l.URL[:8] != "https://" {
			t.Errorf("link %q is not https", l.URL)
		}
	}
}
