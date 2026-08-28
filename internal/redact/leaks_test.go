package redact

import (
	"strings"
	"testing"
)

// Each case here is a value that reached the wire intact before the rules were
// tightened. leaks lists the substrings that must not survive; keeps lists the
// diagnostic text that must, so a rule cannot pass by deleting the message.
func TestTextRedactsPreviouslyLeakedValues(t *testing.T) {
	cases := []struct {
		name  string
		in    string
		leaks []string
		keeps []string
	}{
		{
			name:  "windows path with a space in a directory name",
			in:    `CreateFile D:\Games\Cyberpunk 2077\bin\x64\game.exe: Access is denied.`,
			leaks: []string{"Cyberpunk", "2077", "game.exe", `D:\`, "Games"},
			keeps: []string{"CreateFile", "Access is denied."},
		},
		{
			name:  "windows path written with forward slashes",
			in:    "open E:/TyphonLauncher/internal/download/torrent.go: no such file",
			leaks: []string{"TyphonLauncher", "torrent.go", "E:/"},
			keeps: []string{"open", "no such file"},
		},
		{
			name: "go stack with forward slash build paths",
			in: "goroutine 1 [running]:\n" +
				"typhon/internal/install.(*Service).Run(0xc0001)\n" +
				"\tE:/TyphonLauncher/internal/install/service.go:214 +0x25",
			leaks: []string{"TyphonLauncher", "service.go", "E:/"},
			keeps: []string{"install.(*Service).Run", ":214"},
		},
		{
			name:  "signed url keeps neither signature nor credential",
			in:    `Get "https://cdn.example.com/builds/game.zip?X-Amz-Signature=deadbeefcafe&X-Amz-Credential=AKIA123%2F20260828": i/o timeout`,
			leaks: []string{"X-Amz-Credential", "AKIA123", "deadbeefcafe", "game.zip", "builds"},
			keeps: []string{"https://cdn.example.com", "i/o timeout"},
		},
		{
			name:  "windows default hostname",
			in:    "host DESKTOP-9F3KQ1 unreachable",
			leaks: []string{"DESKTOP-9F3KQ1", "9F3KQ1"},
			keeps: []string{"unreachable"},
		},
		{
			name:  "ipv4 peer address",
			in:    "dial tcp 192.168.1.77:6881: connectex: no route to host",
			leaks: []string{"192.168.1.77"},
			keeps: []string{"dial tcp", "no route to host"},
		},
		{
			name:  "ipv6 peer address",
			in:    "peer 2001:db8::1 disconnected",
			leaks: []string{"2001:db8", "db8::1"},
			keeps: []string{"disconnected"},
		},
		{
			name:  "mac address",
			in:    "adapter 00:1A:2B:3C:4D:5E down",
			leaks: []string{"00:1A:2B:3C:4D:5E", "1A:2B"},
			keeps: []string{"adapter", "down"},
		},
		{
			name:  "posix library path outside the old whitelist",
			in:    "/data/steamlibrary/steamapps/common/Game/run.sh: exec format error",
			leaks: []string{"steamlibrary", "steamapps", "run.sh", "/data/"},
			keeps: []string{"exec format error"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Text(c.in)
			for _, leak := range c.leaks {
				if strings.Contains(got, leak) {
					t.Fatalf("Text(%q) leaked %q:\n  got %q", c.in, leak, got)
				}
			}
			for _, keep := range c.keeps {
				if !strings.Contains(got, keep) {
					t.Fatalf("Text(%q) dropped diagnostic text %q:\n  got %q", c.in, keep, got)
				}
			}
		})
	}
}

// Sanitize is the function callers actually use, and its fail-closed re-scan
// covers a narrower set of patterns than Text substitutes unless every rule is
// listed in unsafeText. Running the same corpus through Sanitize proves the
// two lists agree: a value Text scrubs must come back clean, not refused.
func TestSanitizeAcceptsAndScrubsTheSameCorpus(t *testing.T) {
	corpus := []string{
		`CreateFile D:\Games\Cyberpunk 2077\bin\x64\game.exe: Access is denied.`,
		"open E:/TyphonLauncher/internal/download/torrent.go: no such file",
		`Get "https://cdn.example.com/b.zip?X-Amz-Signature=deadbeefcafe": i/o timeout`,
		"host DESKTOP-9F3KQ1 unreachable",
		"dial tcp 192.168.1.77:6881: connectex: no route to host",
		"peer 2001:db8::1 disconnected",
		"adapter 00:1A:2B:3C:4D:5E down",
		"/data/steamlibrary/steamapps/common/Game/run.sh: exec format error",
	}
	forbidden := []string{
		"Cyberpunk", "TyphonLauncher", "AKIA123", "deadbeefcafe",
		"DESKTOP-9F3KQ1", "192.168.1.77", "2001:db8", "00:1A:2B",
		"steamlibrary",
	}
	for _, in := range corpus {
		t.Run(in[:minInt(28, len(in))], func(t *testing.T) {
			out, err := Sanitize(in)
			if err != nil {
				t.Fatalf("Sanitize(%q) refused a scrubbable value: %v", in, err)
			}
			for _, leak := range forbidden {
				if strings.Contains(out, leak) {
					t.Fatalf("Sanitize(%q) leaked %q: %q", in, leak, out)
				}
			}
		})
	}
}

func TestSetLocalRemovesMachineIdentityOutsidePaths(t *testing.T) {
	t.Cleanup(func() { SetLocal("", "") })
	SetLocal("EGOR-PC-01", "Egor")

	got := Text("account Egor on EGOR-PC-01 is not authorized")
	for _, leak := range []string{"Egor", "EGOR-PC-01"} {
		if strings.Contains(got, leak) {
			t.Fatalf("Text leaked local identity %q: %q", leak, got)
		}
	}
	if !strings.Contains(got, "is not authorized") {
		t.Fatalf("Text dropped the diagnostic text: %q", got)
	}
	if _, err := Sanitize("account Egor logged out"); err != nil {
		t.Fatalf("Sanitize refused a value it can scrub: %v", err)
	}
}

// A generic account name must be ignored: replacing every "user" would shred
// unrelated text while hiding nobody.
func TestSetLocalIgnoresGenericAndShortNames(t *testing.T) {
	t.Cleanup(func() { SetLocal("", "") })
	SetLocal("", "user")
	if got := Text("the user cancelled the download"); got != "the user cancelled the download" {
		t.Fatalf("Text rewrote a generic account name: %q", got)
	}
	SetLocal("", "ab")
	if got := Text("ab is a fragment of absolute"); !strings.Contains(got, "absolute") {
		t.Fatalf("Text matched a two-character account name: %q", got)
	}
}

// The JS side substitutes its own placeholders before calling in. The Go pass
// must not read "<path>:10:5" as an IPv6 address or a drive path.
func TestPlaceholdersSurviveASecondPass(t *testing.T) {
	in := "at run (<path>:10:5)\n at boot (<path>:2:1)"
	got := Text(in)
	if got != in {
		t.Fatalf("Text rewrote already-scrubbed placeholders:\n  in  %q\n  got %q", in, got)
	}
	if strings.Contains(got, IP) {
		t.Fatalf("Text read a stack location as an address: %q", got)
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
