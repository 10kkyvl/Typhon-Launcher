//go:build windows

package main

import (
	"errors"
	"reflect"
	"testing"

	"typhon/internal/installguard"
)

func TestParseArgsCarriesVerifyRepack(t *testing.T) {
	installer := []string{`C:\setup.exe`, "/VERYSILENT", "/DIR=C:\\Games\\X"}
	for _, tc := range []struct {
		name string
		mode string
		want installguard.Bridge
	}{
		{"quiet skips verification", "quiet", installguard.Bridge{Options: installguard.Options{HideProgress: true}}},
		{"quiet keeps verification on request", "verify-quiet", installguard.Bridge{Options: installguard.Options{HideProgress: true, VerifyRepack: true}}},
		{"repack quiet keeps both", "repack-verify-quiet", installguard.Bridge{Options: installguard.Options{HideProgress: true, VerifyRepack: true}, Limit32BitAddressSpace: true}},
		{"music hides nothing", "music", installguard.Bridge{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := append([]string{"installguard.exe", tc.mode, `C:\cancel`, "--"}, installer...)
			got, err := parseArgs(args)
			if err != nil {
				t.Fatal(err)
			}
			if got.bridge != tc.want {
				t.Fatalf("bridge = %+v, want %+v", got.bridge, tc.want)
			}
			if got.cancelFile != `C:\cancel` {
				t.Fatalf("cancelFile = %q", got.cancelFile)
			}
			if !reflect.DeepEqual(got.installer, installer) {
				t.Fatalf("installer = %q, want %q", got.installer, installer)
			}
		})
	}
}

func TestRunRejectsMalformedArguments(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"no arguments", []string{"installguard.exe"}},
		{"missing installer", []string{"installguard.exe", "quiet", `C:\cancel`, "--"}},
		{"missing separator", []string{"installguard.exe", "quiet", `C:\cancel`, "x", `C:\setup.exe`}},
		{"unknown mode", []string{"installguard.exe", "loud", `C:\cancel`, "--", `C:\setup.exe`}},
		{"flag before the mode", []string{"installguard.exe", "--verify-repack", "quiet", `C:\cancel`, "--", `C:\setup.exe`}},
		{"flags in the wrong order", []string{"installguard.exe", "verify-repack-quiet", `C:\cancel`, "--", `C:\setup.exe`}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := parseArgs(tc.args); !errors.Is(err, errUsage) {
				t.Fatalf("parseArgs error = %v, want errUsage", err)
			}
			if code := run(tc.args); code != 1 {
				t.Fatalf("exit code = %d, want 1", code)
			}
		})
	}
}
