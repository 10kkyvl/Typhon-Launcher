package main

import (
	"testing"

	"typhon/internal/devmock"
)

func TestWindowTitleMarksBeta(t *testing.T) {
	suffix := qaTitleSuffix
	if devmock.Enabled {
		suffix = " [devmock]"
	}
	cases := []struct {
		version string
		want    string
	}{
		{"1.2.0", "Typhon" + suffix},
		{"1.3.0-beta.1", "Typhon beta" + suffix},
	}
	for _, c := range cases {
		if got := windowTitleFor(c.version); got != c.want {
			t.Errorf("windowTitleFor(%q) = %q, want %q", c.version, got, c.want)
		}
	}
}
