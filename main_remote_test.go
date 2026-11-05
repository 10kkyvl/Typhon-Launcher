package main

import (
	"os"
	"strings"
	"testing"
	"unicode/utf8"

	"typhon/internal/library"
)

func TestDeviceNameCarriesNoHostName(t *testing.T) {
	got := deviceName("e455bf5c-902e-4889-844b-ce697bd3e3b0")
	host, _ := os.Hostname()
	if !strings.HasPrefix(got, "Typhon · ") || !strings.HasSuffix(got, " · e455") || (host != "" && strings.Contains(got, host)) {
		t.Fatalf("deviceName = %q", got)
	}
	if utf8.RuneCountInString(got) > 64 {
		t.Fatalf("deviceName is longer than the backend accepts: %q", got)
	}
}

func TestRunningTitlesListsOnlyRunningGames(t *testing.T) {
	got := runningTitles([]library.RecentGame{{Title: "Hades", Running: true}, {Title: "Celeste"}})
	if len(got) != 1 || got[0] != "Hades" {
		t.Fatalf("running = %v", got)
	}
	if got := runningTitles(nil); got == nil || len(got) != 0 {
		t.Fatalf("no games must be an empty list, got %#v", got)
	}
}
