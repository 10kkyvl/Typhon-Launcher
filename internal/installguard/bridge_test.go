package installguard

import (
	"errors"
	"testing"
)

func TestBridgeModeRoundTrip(t *testing.T) {
	for _, tc := range []struct {
		name  string
		b     Bridge
		token string
	}{
		{"music", Bridge{}, "music"},
		{"quiet", Bridge{Options: Options{HideProgress: true}}, "quiet"},
		{"quiet with verification", Bridge{Options: Options{HideProgress: true, VerifyRepack: true}}, "verify-quiet"},
		{"repack quiet", Bridge{Options: Options{HideProgress: true}, Limit32BitAddressSpace: true}, "repack-quiet"},
		{"repack quiet with verification", Bridge{Options: Options{HideProgress: true, VerifyRepack: true}, Limit32BitAddressSpace: true}, "repack-verify-quiet"},
		{"repack music", Bridge{Limit32BitAddressSpace: true}, "repack-music"},
		{"music with verification", Bridge{Options: Options{VerifyRepack: true}}, "verify-music"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.b.Mode(); got != tc.token {
				t.Fatalf("Mode() = %q, want %q", got, tc.token)
			}
			got, err := ParseBridgeMode(tc.token)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.b {
				t.Fatalf("ParseBridgeMode(%q) = %+v, want %+v", tc.token, got, tc.b)
			}
		})
	}
}

func TestParseBridgeModeRejectsUnknownTokens(t *testing.T) {
	for _, token := range []string{
		"",
		"loud",
		"Quiet",
		"quiet ",
		"verify",
		"repack-",
		"verify-",
		"verify-repack-quiet",
		"repack-repack-quiet",
		"verify-verify-quiet",
		"repack-verify-",
		"repack-verify-music-",
		"--verify-repack",
		"quiet,verify",
	} {
		t.Run(token, func(t *testing.T) {
			got, err := ParseBridgeMode(token)
			if !errors.Is(err, errBridgeMode) {
				t.Fatalf("ParseBridgeMode(%q) error = %v, want errBridgeMode", token, err)
			}
			if got != (Bridge{}) {
				t.Fatalf("ParseBridgeMode(%q) returned %+v with an error", token, got)
			}
		})
	}
}
