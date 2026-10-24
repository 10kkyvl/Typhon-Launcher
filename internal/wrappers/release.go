package wrappers

import (
	"fmt"
	"net/url"
	"strings"
)

const (
	WrapperDgVoodoo = "dgvoodoo"
	WrapperDXVK     = "dxvk"

	dgVoodooVersion = "2.87.5"
	dgVoodooURL     = "https://github.com/dege-diosg/dgVoodoo2/releases/download/v2.87.5/dgVoodoo2_87_5.zip"
	dgVoodooSHA256  = "5ffde6927f7355ca3fdd5d785b581256a8e6539fa13e395a891ade6ba1040850"

	dxvkVersion = "3.1.1"
	dxvkURL     = "https://github.com/doitsujin/dxvk/releases/download/v3.1.1/dxvk-3.1.1.tar.gz"
	dxvkSHA256  = "40565b4a724aadc4433fa4e010b4b23916d9b1f1baeee64e17186db94f54e608"

	maxArchiveBytes = 64 << 20
	maxEntryBytes   = 32 << 20
	confName        = "dgVoodoo.conf"
)

type archiveFile struct {
	From string
	To   string
}

type release struct {
	Wrapper string
	Version string
	URL     string
	SHA256  string
	Kind    string
	Files   []archiveFile
	Presets []string
}

func (r release) hasPreset(p string) bool {
	for _, v := range r.Presets {
		if v == p {
			return true
		}
	}
	return false
}

func productionReleases() map[string]release {
	dgv := []archiveFile{{"dgVoodoo.conf", confName}}
	for _, n := range []string{"D3D8", "D3D9", "D3DImm", "DDraw"} {
		dgv = append(dgv, archiveFile{"MS/x86/" + n + ".dll", "x86/" + strings.ToLower(n) + ".dll"})
	}
	dgv = append(dgv, archiveFile{"MS/x64/D3D9.dll", "x64/d3d9.dll"})
	for _, n := range []string{"Glide", "Glide2x", "Glide3x"} {
		dgv = append(dgv, archiveFile{"3Dfx/x86/" + n + ".dll", "x86/" + strings.ToLower(n) + ".dll"})
	}

	var dx []archiveFile
	for _, a := range [][2]string{{"x32", "x86"}, {"x64", "x64"}} {
		for _, n := range []string{"d3d9", "d3d10core", "d3d11", "dxgi"} {
			dx = append(dx, archiveFile{
				"dxvk-" + dxvkVersion + "/" + a[0] + "/" + n + ".dll",
				a[1] + "/" + n + ".dll",
			})
		}
	}

	return map[string]release{
		WrapperDgVoodoo: {
			Wrapper: WrapperDgVoodoo, Version: dgVoodooVersion, URL: dgVoodooURL, SHA256: dgVoodooSHA256,
			Kind: "zip", Files: dgv, Presets: []string{"default", "1080p", "native"},
		},
		WrapperDXVK: {
			Wrapper: WrapperDXVK, Version: dxvkVersion, URL: dxvkURL, SHA256: dxvkSHA256,
			Kind: "tgz", Files: dx, Presets: []string{"default"},
		},
	}
}

func allowedURL(u *url.URL) error {
	if u.Scheme != "https" {
		return fmt.Errorf("%w: scheme %q", ErrForeignHost, u.Scheme)
	}
	switch u.Hostname() {
	case "github.com", "objects.githubusercontent.com", "release-assets.githubusercontent.com":
		return nil
	}
	return fmt.Errorf("%w: host %q", ErrForeignHost, u.Hostname())
}
