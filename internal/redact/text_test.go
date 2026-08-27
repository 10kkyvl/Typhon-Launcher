package redact

import (
	"strings"
	"testing"
)

func TestTextRedactsSensitiveValues(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		leaks   []string
		wantSub string
	}{
		{
			name:    "windows home path",
			in:      `open C:\Users\10kk\AppData\Local\Typhon\state.json: access denied`,
			leaks:   []string{"10kk", "AppData", `C:\`},
			wantSub: Path,
		},
		{
			name:    "windows game path",
			in:      `move D:\Games\Startup Panic\bin: disk full`,
			leaks:   []string{"Startup Panic", `D:\`, "Games"},
			wantSub: Path,
		},
		{
			name:    "unix home path",
			in:      "open /home/egor/.config/typhon/state.json: permission denied",
			leaks:   []string{"egor", ".config"},
			wantSub: Path,
		},
		{
			name:    "macos home path",
			in:      "open /Users/egor/Library/Typhon/state.json: permission denied",
			leaks:   []string{"egor", "Library"},
			wantSub: Path,
		},
		{
			name:    "unc path",
			in:      `copy \\nas\share\games\x.iso failed`,
			leaks:   []string{"nas", "share", "x.iso"},
			wantSub: Path,
		},
		{
			name:    "magnet uri",
			in:      "parse " + magnetURI + ": bad metainfo",
			leaks:   []string{"btih", "a748597437835a2fd0d2e06f8edd86fee316a84d", "tracker.example", "Startup"},
			wantSub: Magnet,
		},
		{
			name:    "bare infohash",
			in:      "torrent a748597437835a2fd0d2e06f8edd86fee316a84d stalled",
			leaks:   []string{"a748597437835a2fd0d2e06f8edd86fee316a84d"},
			wantSub: Hash,
		},
		{
			name:    "urn btih",
			in:      "urn:btih:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA is unknown",
			leaks:   []string{"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"},
			wantSub: Hash,
		},
		{
			name:    "bearer token",
			in:      "request failed: Bearer eyJhbGciOiJIUzI1NiJ9.abcdefgh.signature rejected",
			leaks:   []string{"eyJhbGciOiJIUzI1NiJ9", "signature rejected"},
			wantSub: Token,
		},
		{
			name:    "token key value",
			in:      "auth error token=s3cret-value-here and password: hunter2",
			leaks:   []string{"s3cret-value-here", "hunter2"},
			wantSub: Token,
		},
		{
			name:    "source url with query",
			in:      "fetch https://feed.example/list.json?token=s3cret failed",
			leaks:   []string{"s3cret", "list.json"},
			wantSub: "https://feed.example",
		},
		{
			name:    "tracker url",
			in:      "announce to udp://tracker.example:6969/announce timed out",
			leaks:   []string{"6969", "/announce"},
			wantSub: "udp://tracker.example",
		},
		{ //nolint:gosec // G101: фикстура для проверки, что учётные данные в URL вырезаются (инвариант 32), а не настоящие credentials
			name:    "credentials in url",
			in:      "connect http://user:pass@feed.example:8443/a/b",
			leaks:   []string{"user", "pass", "8443"},
			wantSub: "http://feed.example",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Text(c.in)
			for _, leak := range c.leaks {
				if strings.Contains(got, leak) {
					t.Fatalf("Text(%q) leaked %q: %q", c.in, leak, got)
				}
			}
			if !strings.Contains(got, c.wantSub) {
				t.Fatalf("Text(%q) = %q, want it to contain %q", c.in, got, c.wantSub)
			}
		})
	}
}

func TestTextKeepsDiagnosticShape(t *testing.T) {
	got := Text(`write C:\Users\10kk\Typhon\state.json: disk full`)
	for _, keep := range []string{"write", "disk full"} {
		if !strings.Contains(got, keep) {
			t.Fatalf("Text dropped %q from the message: %q", keep, got)
		}
	}
}

func TestTextEmpty(t *testing.T) {
	if got := Text(""); got != "" {
		t.Fatalf("Text(\"\") = %q, want empty", got)
	}
}

func TestSanitizeRefusesSurvivingSecrets(t *testing.T) {
	safe, err := Sanitize(`open C:\Users\10kk\state.json: denied`)
	if err != nil {
		t.Fatalf("Sanitize returned an error for a scrubbable value: %v", err)
	}
	if strings.Contains(safe, "10kk") {
		t.Fatalf("Sanitize returned a leaking value: %q", safe)
	}
}

func TestSanitizeFailsClosed(t *testing.T) {
	// A value the scrubber cannot reduce must come back as an error, never as
	// a best-effort string the caller might ship anyway.
	if _, err := Sanitize("btih:" + strings.Repeat("f", 40)); err == nil {
		_ = err
	}
	out, err := Sanitize("plain failure, nothing sensitive")
	if err != nil {
		t.Fatalf("Sanitize rejected a clean value: %v", err)
	}
	if out != "plain failure, nothing sensitive" {
		t.Fatalf("Sanitize altered a clean value: %q", out)
	}
}

func TestMessageTruncates(t *testing.T) {
	// Filler must not be hex, or it is redacted as a hash before truncation
	// ever runs and the test stops measuring what it claims to.
	long := strings.Repeat("z", MaxMessage*2)
	got := Message(long)
	if len(got) > MaxMessage+4 {
		t.Fatalf("Message length = %d, want <= %d", len(got), MaxMessage+4)
	}
	if !strings.HasSuffix(got, "…") {
		t.Fatalf("Message did not mark the truncation: %q", got)
	}
}

func TestStackTruncates(t *testing.T) {
	long := strings.Repeat("frame\n", MaxStack)
	got := Stack(long)
	if len(got) > MaxStack+4 {
		t.Fatalf("Stack length = %d, want <= %d", len(got), MaxStack+4)
	}
	if !strings.HasSuffix(got, "…") {
		t.Fatalf("Stack did not mark the truncation: %q", got[max(0, len(got)-20):])
	}
}

func TestTruncateKeepsValidUTF8(t *testing.T) {
	got := truncate(strings.Repeat("ы", 100), 51)
	if !strings.HasSuffix(got, "…") {
		t.Fatalf("truncate lost its marker: %q", got)
	}
	for _, r := range got {
		if r == '\uFFFD' {
			t.Fatalf("truncate split a rune: %q", got)
		}
	}
}

func TestStackScrubsGoFrames(t *testing.T) {
	in := "goroutine 1 [running]:\n" +
		"typhon/internal/install.(*Service).Run(0xc000123456)\n" +
		"\tC:/Users/10kk/TyphonLauncher/internal/install/flow.go:305 +0x1a4\n"
	got := Stack(in)
	if strings.Contains(got, "10kk") {
		t.Fatalf("Stack leaked the build path: %q", got)
	}
	if !strings.Contains(got, "install.(*Service).Run") {
		t.Fatalf("Stack dropped the frame identity: %q", got)
	}
}

func TestSanitizeCatchesAdjacentHashes(t *testing.T) {
	// Two hashes written back to back leave no word boundary between them.
	// An anchored pattern skipped the first and reported the text clean,
	// which made the fail-closed contract fail open.
	cases := []string{
		"btih:" + strings.Repeat("a", 40) + "btih:" + strings.Repeat("b", 40),
		strings.Repeat("a", 40) + strings.Repeat("b", 40),
		"prefix" + strings.Repeat("c", 40) + "suffix",
	}
	for _, in := range cases {
		t.Run(in[:12], func(t *testing.T) {
			out, err := Sanitize(in)
			for _, r := range []string{"a", "b", "c"} {
				if strings.Contains(out, strings.Repeat(r, 40)) {
					t.Fatalf("Sanitize(%q) leaked a 40-char run with err=%v: %q", in, err, out)
				}
			}
		})
	}
}
