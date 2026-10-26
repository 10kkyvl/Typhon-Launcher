package main

import (
	"errors"
	"reflect"
	"runtime"
	"testing"

	"typhon/internal/library"
	"typhon/internal/tray"
)

func TestOpenTarget(t *testing.T) {
	cases := []struct {
		name      string
		args      []string
		route     string
		requested bool
		err       error
	}{
		{"none", []string{"typhon.exe"}, "", false, nil},
		{"library", []string{"typhon.exe", "--open", "library"}, "library", true, nil},
		{"downloads", []string{"typhon.exe", "--open", "downloads"}, "downloads", true, nil},
		{"padded value", []string{"typhon.exe", "--open", " library "}, "library", true, nil},
		{"no value", []string{"typhon.exe", "--open"}, "", true, errBadOpenTarget},
		{"blank value", []string{"typhon.exe", "--open", " "}, "", true, errBadOpenTarget},
		{"unknown route", []string{"typhon.exe", "--open", "settings"}, "", true, errBadOpenTarget},
		{"wrong case", []string{"typhon.exe", "--open", "Library"}, "", true, errBadOpenTarget},
		{"flag-like value", []string{"typhon.exe", "--open", "--play"}, "", true, errBadOpenTarget},
		{"program name is not an argument", []string{"--open", "library"}, "", false, nil},
		{"after other flags", []string{"typhon.exe", "--profile", "ab12", "--open", "downloads"}, "downloads", true, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			route, requested, err := openTarget(tc.args)
			if !errors.Is(err, tc.err) || requested != tc.requested || route != tc.route {
				t.Fatalf("openTarget = %q, %v, %v; want %q, %v, %v", route, requested, err, tc.route, tc.requested, tc.err)
			}
		})
	}
}

func TestNavigateFromArgs(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want []string
		ok   bool
	}{
		{"navigates to the route", []string{"typhon.exe", "--open", "downloads"}, []string{"downloads"}, true},
		{"nothing requested", []string{"typhon.exe"}, nil, false},
		{"a bad route is dropped, not guessed", []string{"typhon.exe", "--open", "nowhere"}, nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got []string
			nav := &NavigationService{}
			ok := navigateFromArgs(tc.args, nav, func(route string) { got = append(got, route) })
			if ok != tc.ok || !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("navigateFromArgs = %v, emitted %v; want %v, %v", ok, got, tc.ok, tc.want)
			}
			wantPending := ""
			if len(tc.want) > 0 {
				wantPending = tc.want[0]
			}
			if pending := nav.TakePending(); pending != wantPending {
				t.Fatalf("pending = %q, want %q", pending, wantPending)
			}
		})
	}
}

func TestTrayGamesKeepsOrderAndRunning(t *testing.T) {
	got := trayGames([]library.RecentGame{
		{ID: "b", Title: "B", Executable: "b.exe", Running: true},
		{ID: "a", Title: "A"},
	})
	want := []tray.Game{{ID: "b", Title: "B", Running: true}, {ID: "a", Title: "A"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("trayGames = %+v, want %+v", got, want)
	}
}

func TestRecentMenuBuildFollowsTheModel(t *testing.T) {
	menu := &recentMenu{language: func() string { return "en" }}
	menu.setGames([]tray.Game{
		{ID: "a", Title: "Tom & Jerry", Running: true},
		{ID: "b", Title: "Beta"},
	})

	built := menu.build()

	type row struct {
		label     string
		enabled   bool
		separator bool
	}
	var got []row
	for i := 0; ; i++ {
		item := built.ItemAt(i)
		if item == nil {
			break
		}
		if item.IsSeparator() {
			got = append(got, row{separator: true})
			continue
		}
		got = append(got, row{label: item.Label(), enabled: item.Enabled()})
	}
	want := []row{
		{"Recent games", false, false},
		{"Tom && Jerry — playing", false, false},
		{"Stop", true, false},
		{"Beta", true, false},
		{"", false, true},
		{"Open Typhon", true, false},
		{"", false, true},
		{"Quit", true, false},
	}
	if runtime.GOOS == "windows" {
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("menu = %+v\nwant   %+v", got, want)
		}
		return
	}
	want[1].label = "Tom & Jerry — playing"
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("menu = %+v\nwant   %+v", got, want)
	}
}

func TestRecentMenuWithoutGamesHasOnlyOpenAndQuit(t *testing.T) {
	menu := &recentMenu{language: func() string { return "ru" }}
	built := menu.build()
	first, last := built.ItemAt(0), built.ItemAt(2)
	if first == nil || first.Label() != "Открыть Typhon" || last == nil || last.Label() != "Выход" || built.ItemAt(3) != nil {
		t.Fatal("an empty library must leave the original two-item menu")
	}
}

func TestNavigationServiceHandsTheRouteOverOnce(t *testing.T) {
	nav := &NavigationService{}
	if got := nav.TakePending(); got != "" {
		t.Fatalf("TakePending on a fresh service = %q, want empty", got)
	}
	nav.setPending("downloads")
	if got := nav.TakePending(); got != "downloads" {
		t.Fatalf("first TakePending = %q, want downloads", got)
	}
	if got := nav.TakePending(); got != "" {
		t.Fatalf("second TakePending = %q, want empty", got)
	}
}

func TestSecondInstanceBothEmitsAndKeepsPending(t *testing.T) {
	nav := &NavigationService{}
	var emitted []string
	navigateFromArgs([]string{"typhon.exe", "--open", "library"}, nav, func(route string) { emitted = append(emitted, route) })
	if !reflect.DeepEqual(emitted, []string{"library"}) {
		t.Fatalf("emitted = %v, want [library]", emitted)
	}
	if got := nav.TakePending(); got != "library" {
		t.Fatalf("pending = %q, want library: a page that was still loading missed the event", got)
	}
	if got := nav.TakePending(); got != "" {
		t.Fatalf("second TakePending = %q, want empty", got)
	}
}

func TestNavigationServiceIsSafeUnderConcurrentUse(t *testing.T) {
	nav := &NavigationService{}
	nav.setPending("library")
	taken := make(chan string, 8)
	for range 8 {
		go func() { taken <- nav.TakePending() }()
	}
	nonEmpty := 0
	for range 8 {
		if <-taken != "" {
			nonEmpty++
		}
	}
	if nonEmpty != 1 {
		t.Fatalf("%d callers got the route, want exactly 1", nonEmpty)
	}
}
