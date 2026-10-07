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
	if got := windowTitle(); got != "Typhon" {
		t.Fatalf("windowTitle() = %q, want %q", got, "Typhon")
	}
}
