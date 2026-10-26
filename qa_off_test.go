//go:build !qa && !devmock

package main

import "testing"

func TestReleaseBuildHasNoQAHooks(t *testing.T) {
	args, err := qaStart()
	if err != nil {
		t.Fatalf("qaStart() error = %v", err)
	}
	if len(args) != 0 {
		t.Fatalf("qaStart() args = %v, want none", args)
	}
	if got := windowTitleFor("1.2.0"); got != "Typhon" {
		t.Fatalf("windowTitleFor() = %q, want %q", got, "Typhon")
	}
}
