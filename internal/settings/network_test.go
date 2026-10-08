package settings

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"typhon/internal/uierr"
)

func TestNetworkDefaultsAreDirect(t *testing.T) {
	d := Defaults()
	if d.NetworkMode != NetworkDirect {
		t.Fatalf("NetworkMode = %q, want %q", d.NetworkMode, NetworkDirect)
	}
	if d.ProxyType != ProxySOCKS5 {
		t.Fatalf("ProxyType = %q, want %q", d.ProxyType, ProxySOCKS5)
	}
	if _, err := sanitize(d); err != nil {
		t.Fatalf("defaults must pass validation: %v", err)
	}
}

func TestOldConfigWithoutNetworkFieldsLoadsAsDirect(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"theme":"dark","maxActiveDownloads":3}`), 0o600); err != nil {
		t.Fatal(err)
	}
	got := mustServiceAt(t, path).GetSettings()
	if got.NetworkMode != NetworkDirect || got.ProxyType != ProxySOCKS5 || got.ProxyHost != "" || got.ProxyPort != 0 || got.NetworkInterface != "" {
		t.Fatalf("network fields of an old config = %+v", got)
	}
}

func TestNetworkValidationCodes(t *testing.T) {
	proxyBase := func(s *Settings) {
		s.NetworkMode = NetworkProxy
		s.ProxyHost = "127.0.0.1"
		s.ProxyPort = 1080
	}
	cases := []struct {
		name string
		mut  func(*Settings)
		want error
	}{
		{"unknown mode", func(s *Settings) { s.NetworkMode = "vpn" }, ErrNetworkModeInvalid},
		{"empty mode", func(s *Settings) { s.NetworkMode = "" }, ErrNetworkModeInvalid},
		{"mode is case sensitive", func(s *Settings) { s.NetworkMode = "Direct" }, ErrNetworkModeInvalid},
		{"interface mode without adapter", func(s *Settings) { s.NetworkMode = NetworkInterface }, ErrNetworkInterfaceRequired},
		{"interface mode with blank adapter", func(s *Settings) { s.NetworkMode = NetworkInterface; s.NetworkInterface = "   " }, ErrNetworkInterfaceRequired},
		{"adapter with control character", func(s *Settings) { s.NetworkInterface = "eth\x00" }, ErrNetworkInterfaceInvalid},
		{"adapter name too long", func(s *Settings) { s.NetworkInterface = strings.Repeat("a", 257) }, ErrNetworkInterfaceInvalid},
		{"proxy mode without host", func(s *Settings) { s.NetworkMode = NetworkProxy; s.ProxyPort = 1080 }, ErrProxyHostRequired},
		{"proxy mode with blank host", func(s *Settings) { s.NetworkMode = NetworkProxy; s.ProxyHost = "  "; s.ProxyPort = 1080 }, ErrProxyHostRequired},
		{"host with scheme", func(s *Settings) { proxyBase(s); s.ProxyHost = "socks5://proxy.example.com" }, ErrProxyHostInvalid},
		{"host with path", func(s *Settings) { proxyBase(s); s.ProxyHost = "proxy.example.com/x" }, ErrProxyHostInvalid},
		{"host with port", func(s *Settings) { proxyBase(s); s.ProxyHost = "proxy.example.com:1080" }, ErrProxyHostInvalid},
		{"host with inner space", func(s *Settings) { proxyBase(s); s.ProxyHost = "proxy example.com" }, ErrProxyHostInvalid},
		{"host with userinfo", func(s *Settings) { proxyBase(s); s.ProxyHost = "user@proxy.example.com" }, ErrProxyHostInvalid},
		{"host with brackets", func(s *Settings) { proxyBase(s); s.ProxyHost = "[::1]" }, ErrProxyHostInvalid},
		{"host with zone", func(s *Settings) { proxyBase(s); s.ProxyHost = "fe80::1%eth0" }, ErrProxyHostInvalid},
		{"host with empty label", func(s *Settings) { proxyBase(s); s.ProxyHost = "proxy..example.com" }, ErrProxyHostInvalid},
		{"host with trailing dot", func(s *Settings) { proxyBase(s); s.ProxyHost = "proxy.example.com." }, ErrProxyHostInvalid},
		{"host with leading hyphen", func(s *Settings) { proxyBase(s); s.ProxyHost = "-proxy.example.com" }, ErrProxyHostInvalid},
		{"label longer than 63", func(s *Settings) { proxyBase(s); s.ProxyHost = strings.Repeat("a", 64) + ".example.com" }, ErrProxyHostInvalid},
		{"host longer than dns allows", func(s *Settings) { proxyBase(s); s.ProxyHost = strings.Repeat("a.", 127) + "aa" }, ErrProxyHostInvalid},
		{"proxy mode without port", func(s *Settings) { s.NetworkMode = NetworkProxy; s.ProxyHost = "127.0.0.1" }, ErrProxyPortInvalid},
		{"port above range", func(s *Settings) { proxyBase(s); s.ProxyPort = 65536 }, ErrProxyPortInvalid},
		{"negative port", func(s *Settings) { proxyBase(s); s.ProxyPort = -1 }, ErrProxyPortInvalid},
		{"unknown proxy type", func(s *Settings) { proxyBase(s); s.ProxyType = "ftp" }, ErrProxyTypeInvalid},
		{"empty proxy type in proxy mode", func(s *Settings) { proxyBase(s); s.ProxyType = "" }, ErrProxyTypeInvalid},
		{"username with colon", func(s *Settings) { proxyBase(s); s.ProxyUsername = "a:b" }, ErrProxyUsernameInvalid},
		{"username with newline", func(s *Settings) { proxyBase(s); s.ProxyUsername = "a\nb" }, ErrProxyUsernameInvalid},
		{"username too long", func(s *Settings) { proxyBase(s); s.ProxyUsername = strings.Repeat("u", 256) }, ErrProxyUsernameInvalid},
		{"junk host is rejected even when the proxy is off", func(s *Settings) { s.NetworkMode = NetworkDirect; s.ProxyHost = "http://x" }, ErrProxyHostInvalid},
	}
	seen := map[string]string{}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "settings.json")
			svc := mustServiceAt(t, path)
			next := Defaults()
			c.mut(&next)
			err := svc.SaveSettings(next)
			if !errors.Is(err, c.want) {
				t.Fatalf("SaveSettings error = %v, want %v", err, c.want)
			}
			if uierr.Code(err) == "" {
				t.Fatalf("error %v has no ui code", err)
			}
			if _, statErr := os.Stat(path); !errors.Is(statErr, os.ErrNotExist) {
				t.Fatalf("a rejected save must not write the file, stat error = %v", statErr)
			}
			if got := svc.GetSettings(); got != Defaults() {
				t.Fatalf("a rejected save changed the settings: %+v", got)
			}
		})
		seen[uierr.Code(c.want)] = c.name
	}
	if len(seen) != 8 {
		t.Fatalf("network validation must use 8 distinct codes, got %d: %v", len(seen), seen)
	}
}

func TestNetworkValidationAccepts(t *testing.T) {
	cases := []struct {
		name string
		mut  func(*Settings)
	}{
		{"direct", func(s *Settings) {}},
		{"interface", func(s *Settings) { s.NetworkMode = NetworkInterface; s.NetworkInterface = "Ethernet 2 (WireGuard)" }},
		{"interface with cyrillic name", func(s *Settings) {
			s.NetworkMode = NetworkInterface
			s.NetworkInterface = "Подключение по локальной сети"
		}},
		{"socks5 loopback", func(s *Settings) {
			s.NetworkMode, s.ProxyType, s.ProxyHost, s.ProxyPort = NetworkProxy, ProxySOCKS5, "127.0.0.1", 10808
		}},
		{"http hostname with auth", func(s *Settings) {
			s.NetworkMode, s.ProxyType, s.ProxyHost, s.ProxyPort, s.ProxyUsername = NetworkProxy, ProxyHTTP, "proxy.example.com", 8080, "user"
		}},
		{"ipv6 host", func(s *Settings) {
			s.NetworkMode, s.ProxyHost, s.ProxyPort = NetworkProxy, "::1", 1080
		}},
		{"lowest port", func(s *Settings) { s.NetworkMode, s.ProxyHost, s.ProxyPort = NetworkProxy, "localhost", 1 }},
		{"highest port", func(s *Settings) { s.NetworkMode, s.ProxyHost, s.ProxyPort = NetworkProxy, "localhost", 65535 }},
		{"underscore in hostname", func(s *Settings) { s.NetworkMode, s.ProxyHost, s.ProxyPort = NetworkProxy, "my_proxy.lan", 1080 }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "settings.json")
			svc := mustServiceAt(t, path)
			next := Defaults()
			c.mut(&next)
			if err := svc.SaveSettings(next); err != nil {
				t.Fatalf("SaveSettings: %v", err)
			}
			if got := mustServiceAt(t, path).GetSettings(); got != next {
				t.Fatalf("reloaded %+v, want %+v", got, next)
			}
		})
	}
}

func TestNetworkFieldsAreTrimmed(t *testing.T) {
	svc := mustServiceAt(t, filepath.Join(t.TempDir(), "settings.json"))
	next := Defaults()
	next.NetworkMode = NetworkProxy
	next.ProxyHost = "  127.0.0.1\t"
	next.ProxyPort = 1080
	next.ProxyUsername = " user "
	next.NetworkInterface = " Wintun "
	if err := svc.SaveSettings(next); err != nil {
		t.Fatalf("SaveSettings: %v", err)
	}
	got := svc.GetSettings()
	if got.ProxyHost != "127.0.0.1" || got.ProxyUsername != "user" || got.NetworkInterface != "Wintun" {
		t.Fatalf("fields were not trimmed: %+v", got)
	}
}

func TestLoadRefusesUnknownNetworkMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"networkMode":"vpn"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	svc, err := NewServiceAt(path)
	if err == nil {
		t.Fatalf("a file with an unknown network mode must not fall back to direct, got %+v", svc.GetSettings())
	}
	if !errors.Is(err, ErrNetworkModeInvalid) {
		t.Fatalf("error = %v, want ErrNetworkModeInvalid", err)
	}
}

func TestNetworkFieldsAreLocal(t *testing.T) {
	names := jsonNames(Portable{})
	for _, n := range []string{"networkMode", "networkInterface", "proxyType", "proxyHost", "proxyPort", "proxyUsername"} {
		for _, p := range names {
			if p == n {
				t.Fatalf("%q must not travel with the account: the adapter is machine specific and the proxy is private", n)
			}
		}
	}
}

func TestNormalizeProxyUsernameIsTheRuleOfTheSave(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
		err  error
	}{
		{"plain", "bob", "bob", nil},
		{"trimmed", "  bob\t", "bob", nil},
		{"empty", "", "", nil},
		{"colon", "a:b", "", ErrProxyUsernameInvalid},
		{"newline", "a\nb", "", ErrProxyUsernameInvalid},
		{"too long", strings.Repeat("u", 256), "", ErrProxyUsernameInvalid},
		{"invalid utf-8", "a\xffb", "", ErrProxyUsernameInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NormalizeProxyUsername(tc.in)
			if !errors.Is(err, tc.err) || got != tc.want {
				t.Fatalf("NormalizeProxyUsername(%q) = %q, %v, want %q, %v", tc.in, got, err, tc.want, tc.err)
			}
			s := Defaults()
			s.ProxyUsername = tc.in
			saved, saveErr := sanitize(s)
			if !errors.Is(saveErr, tc.err) || (tc.err == nil && saved.ProxyUsername != tc.want) {
				t.Fatalf("sanitize disagrees with NormalizeProxyUsername for %q: %q, %v", tc.in, saved.ProxyUsername, saveErr)
			}
		})
	}
}
