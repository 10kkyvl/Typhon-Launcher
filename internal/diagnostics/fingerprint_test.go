package diagnostics

import "testing"

func TestFingerprintSameLogicalErrorSameFingerprint(t *testing.T) {
	stackA := "typhon/internal/install.(*Service).Run(0xc000123456)\n" +
		"\tC:/Users/10kk/TyphonLauncher/internal/install/flow.go:305 +0x1a4\n" +
		"typhon/internal/install.(*Service).apply(0xc000654321)\n" +
		"\tC:/Users/10kk/TyphonLauncher/internal/install/flow.go:210 +0x88\n"
	stackB := "typhon/internal/install.(*Service).Run(0xdeadbeef)\n" +
		"\t/home/egor/typhon/internal/install/flow.go:305 +0x1a4\n" +
		"typhon/internal/install.(*Service).apply(0xfeedface)\n" +
		"\t/home/egor/typhon/internal/install/flow.go:999 +0x99\n"

	fpA := Fingerprint("timeout", "install", stackA)
	fpB := Fingerprint("timeout", "install", stackB)
	if fpA != fpB {
		t.Fatalf("same logical error produced different fingerprints: %q vs %q", fpA, fpB)
	}
}

func TestFingerprintDifferentComponentDifferentFingerprint(t *testing.T) {
	stack := "typhon/internal/install.(*Service).Run(0xc000123456)\n" +
		"\tC:/Users/10kk/TyphonLauncher/internal/install/flow.go:305 +0x1a4\n"

	fpInstall := Fingerprint("timeout", "install", stack)
	fpDownload := Fingerprint("timeout", "download", stack)
	if fpInstall == fpDownload {
		t.Fatalf("different components produced the same fingerprint: %q", fpInstall)
	}
}

func TestFingerprintDifferentErrorCodeDifferentFingerprint(t *testing.T) {
	stack := "main.foo(0x1)\n\t/path/main.go:1 +0x1\n"
	fpTimeout := Fingerprint("timeout", "install", stack)
	fpNetwork := Fingerprint("network", "install", stack)
	if fpTimeout == fpNetwork {
		t.Fatalf("different error codes produced the same fingerprint: %q", fpTimeout)
	}
}

func TestFingerprintDifferentStackShapeDifferentFingerprint(t *testing.T) {
	stackA := "main.foo(0x1)\n\t/path/main.go:1 +0x1\n"
	stackB := "main.bar(0x1)\n\t/path/main.go:1 +0x1\n"
	fpA := Fingerprint("unknown", "install", stackA)
	fpB := Fingerprint("unknown", "install", stackB)
	if fpA == fpB {
		t.Fatalf("different frame identities produced the same fingerprint: %q", fpA)
	}
}

func TestFingerprintIsCaseAndWhitespaceInsensitiveForErrorCodeAndComponent(t *testing.T) {
	stack := "main.foo(0x1)\n\t/path/main.go:1 +0x1\n"
	fpA := Fingerprint("Timeout", "  Install  ", stack)
	fpB := Fingerprint("timeout", "install", stack)
	if fpA != fpB {
		t.Fatalf("fingerprint sensitive to case/whitespace of error_code/component: %q vs %q", fpA, fpB)
	}
}

func TestFingerprintJSStackFramesResolve(t *testing.T) {
	stack := "Error: boom\n" +
		"    at Foo (https://typhon.app/assets/main.js:120:15)\n" +
		"    at Bar (https://typhon.app/assets/main.js:88:4)\n" +
		"    at https://typhon.app/assets/main.js:10:1\n"
	got := normalizeFrames(stack, 3)
	want := []string{"Foo", "Bar", "<anonymous>"}
	if len(got) != len(want) {
		t.Fatalf("normalizeFrames = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("normalizeFrames[%d] = %q, want %q (full: %v)", i, got[i], want[i], got)
		}
	}
}

func TestFingerprintGoStackFramesResolve(t *testing.T) {
	stack := "goroutine 1 [running]:\n" +
		"typhon/internal/install.(*Service).Run(0xc000123456)\n" +
		"\tC:/Users/10kk/TyphonLauncher/internal/install/flow.go:305 +0x1a4\n" +
		"main.main()\n" +
		"\tC:/Users/10kk/TyphonLauncher/main.go:91 +0x25\n"
	got := normalizeFrames(stack, 3)
	want := []string{"typhon/internal/install.(*Service).Run", "main.main"}
	if len(got) != len(want) {
		t.Fatalf("normalizeFrames = %v, want %v", got, want)
	}
	if got[0] != want[0] {
		t.Fatalf("normalizeFrames[0] = %q, want %q", got[0], want[0])
	}
	if got[1] != want[1] {
		t.Fatalf("normalizeFrames[1] = %q, want %q", got[1], want[1])
	}
}

func TestFingerprintEmptyStackStillProducesStableValue(t *testing.T) {
	fpA := Fingerprint("unknown", "install", "")
	fpB := Fingerprint("unknown", "install", "")
	if fpA != fpB || fpA == "" {
		t.Fatalf("Fingerprint with empty stack not stable: %q", fpA)
	}
}
