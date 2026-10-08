package install

import (
	"slices"
	"testing"

	"typhon/internal/installguard"
)

func TestBridgeForCarriesVerifyRepack(t *testing.T) {
	for _, tc := range []struct {
		name    string
		opts    installOptions
		hide    bool
		limit32 bool
		want    string
	}{
		{"skipped", installOptions{}, true, false, "quiet"},
		{"wanted", installOptions{VerifyRepack: true}, true, false, "verify-quiet"},
		{"repack skipped", installOptions{}, true, true, "repack-quiet"},
		{"repack wanted", installOptions{VerifyRepack: true}, true, true, "repack-verify-quiet"},
		{"visible install", installOptions{VerifyRepack: true}, false, false, "verify-music"},
		{"other options do not leak", installOptions{SkipExtras: true, SkipShortcuts: true}, true, false, "quiet"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bridge := bridgeFor(tc.opts, tc.hide, tc.limit32)
			if got := bridge.Mode(); got != tc.want {
				t.Fatalf("mode = %q, want %q", got, tc.want)
			}
			parsed, err := installguard.ParseBridgeMode(bridge.Mode())
			if err != nil || parsed != bridge {
				t.Fatalf("the bridge cannot read back its own mode: %+v, %v", parsed, err)
			}
		})
	}
}

func TestBridgeArgs(t *testing.T) {
	setup := []string{"/VERYSILENT", `/DIR=T:\Demo`}
	for _, tc := range []struct {
		name   string
		bridge installguard.Bridge
		args   []string
		want   []string
	}{
		{"skipped", bridgeFor(installOptions{}, true, false), setup, []string{"quiet", `Z:\cancel`, "--", `T:\Demo\setup.exe`, "/VERYSILENT", `/DIR=T:\Demo`}},
		{"wanted", bridgeFor(installOptions{VerifyRepack: true}, true, false), setup, []string{"verify-quiet", `Z:\cancel`, "--", `T:\Demo\setup.exe`, "/VERYSILENT", `/DIR=T:\Demo`}},
		{"repack wanted", bridgeFor(installOptions{VerifyRepack: true}, true, true), nil, []string{"repack-verify-quiet", `Z:\cancel`, "--", `T:\Demo\setup.exe`}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := slices.Clone(tc.args)
			got := bridgeArgs(tc.bridge, `Z:\cancel`, `T:\Demo\setup.exe`, tc.args)
			if !slices.Equal(got, tc.want) {
				t.Fatalf("argv = %q, want %q", got, tc.want)
			}
			if !slices.Equal(tc.args, before) {
				t.Fatalf("installer arguments were modified: %q, was %q", tc.args, before)
			}
			parsed, err := installguard.ParseBridgeMode(got[0])
			if err != nil || parsed != tc.bridge {
				t.Fatalf("the bridge reads %+v, %v from the first argument, launcher meant %+v", parsed, err, tc.bridge)
			}
		})
	}
}
