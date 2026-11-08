package main

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"typhon/internal/deeplink"
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

func TestLinkTarget(t *testing.T) {
	steam := deeplink.Target{Provider: deeplink.ProviderSteam, ID: "440"}
	cases := []struct {
		name      string
		args      []string
		want      deeplink.Target
		requested bool
		err       error
	}{
		{"no arguments", []string{"typhon.exe"}, deeplink.Target{}, false, nil},
		{"empty argv", nil, deeplink.Target{}, false, nil},
		{"other flags", []string{"typhon.exe", "--open", "library"}, deeplink.Target{}, false, nil},
		{"program name is not a link", []string{"typhon://game?steam=440"}, deeplink.Target{}, false, nil},
		{"link", []string{"typhon.exe", "typhon://game?steam=440"}, steam, true, nil},
		{"browser slash", []string{"typhon.exe", "typhon://game/?steam=440"}, steam, true, nil},
		{"upper case scheme still counts", []string{"typhon.exe", "TYPHON://nope"}, deeplink.Target{}, true, deeplink.ErrHost},
		{"bad link", []string{"typhon.exe", "typhon://game?steam=abc"}, deeplink.Target{}, true, deeplink.ErrID},
		{"quote split the url into play arguments", []string{"typhon.exe", "typhon://game?steam=1", "--play", "g1"}, deeplink.Target{}, true, errLinkWithArguments},
		{"link after a play flag", []string{"typhon.exe", "--play", "g1", "typhon://game?steam=1"}, deeplink.Target{}, true, errLinkWithArguments},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, requested, err := linkTarget(tc.args)
			if !errors.Is(err, tc.err) || requested != tc.requested || got != (deeplink.Link{Target: tc.want}) {
				t.Fatalf("linkTarget = %+v, %v, %v; want %+v, %v, %v", got, requested, err, tc.want, tc.requested, tc.err)
			}
		})
	}
}

func TestFirstLaunch(t *testing.T) {
	cases := []struct {
		name      string
		args      []string
		play      playRequest
		requested bool
		route     string
		link      PendingLink
	}{
		{name: "plain", args: []string{"typhon.exe"}},
		{name: "play", args: []string{"typhon.exe", "--play", "g1"}, play: playRequest{id: "g1"}, requested: true},
		{name: "play wins over open", args: []string{"typhon.exe", "--play", "g1", "--open", "library"}, play: playRequest{id: "g1"}, requested: true},
		{name: "open", args: []string{"typhon.exe", "--open", "library"}, route: "library"},
		{name: "broken play still opens the route", args: []string{"typhon.exe", "--open", "library", "--play"}, route: "library"},
		{name: "link", args: []string{"typhon.exe", "typhon://game?igdb=7"}, link: PendingLink{Provider: deeplink.ProviderIGDB, ID: "7"}},
		{name: "bad link is reported to the page", args: []string{"typhon.exe", "typhon://game?igdb=x"}, link: PendingLink{Invalid: true}},
		{name: "injected play never plays", args: []string{"typhon.exe", "typhon://game?igdb=7", "--play", "g1"}, link: PendingLink{Invalid: true}},
		{name: "injected open never navigates", args: []string{"typhon.exe", "typhon://game?igdb=7", "--open", "library"}, link: PendingLink{Invalid: true}},
		{name: "search link", args: []string{"typhon.exe", "typhon://search?q=%D0%92%D0%B5%D0%B4%D1%8C%D0%BC%D0%B0%D0%BA"}, link: PendingLink{Query: "Ведьмак"}},
		{name: "play flag in a query stays text", args: []string{"typhon.exe", "typhon://search?q=--play"}, link: PendingLink{Query: "--play"}},
		{name: "bad search link is reported to the page", args: []string{"typhon.exe", "typhon://search?q=%ZZ"}, link: PendingLink{Invalid: true}},
		{name: "search with injected play never plays", args: []string{"typhon.exe", "typhon://search?q=a", "--play", "g1"}, link: PendingLink{Invalid: true}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			nav := &NavigationService{}
			play, requested := firstLaunch(tc.args, nav)
			if play != tc.play || requested != tc.requested {
				t.Fatalf("firstLaunch = %+v, %v; want %+v, %v", play, requested, tc.play, tc.requested)
			}
			if route := nav.TakePending(); route != tc.route {
				t.Fatalf("pending route = %q, want %q", route, tc.route)
			}
			if link := nav.TakePendingLink(); link != tc.link {
				t.Fatalf("pending link = %+v, want %+v", link, tc.link)
			}
		})
	}
}

func TestPendingLinkIsHandedOverOnce(t *testing.T) {
	nav := &NavigationService{}
	if got := nav.TakePendingLink(); got != (PendingLink{}) {
		t.Fatalf("fresh service = %+v", got)
	}
	want := PendingLink{Provider: deeplink.ProviderSteam, ID: "440"}
	nav.setPendingLink(want)
	if got := nav.TakePendingLink(); got != want {
		t.Fatalf("first take = %+v", got)
	}
	if got := nav.TakePendingLink(); got != (PendingLink{}) {
		t.Fatalf("second take = %+v", got)
	}
}

func TestNewestLaunchReplacesThePendingRequest(t *testing.T) {
	link := PendingLink{Provider: deeplink.ProviderSteam, ID: "440"}

	nav := &NavigationService{}
	nav.setPending("library")
	nav.setPendingLink(link)
	if route, got := nav.TakePending(), nav.TakePendingLink(); route != "" || got != link {
		t.Fatalf("link after route: %q, %+v", route, got)
	}

	nav.setPendingLink(link)
	nav.setPending("downloads")
	if route, got := nav.TakePending(), nav.TakePendingLink(); route != "downloads" || got != (PendingLink{}) {
		t.Fatalf("route after link: %q, %+v", route, got)
	}
}

func TestPendingLinkIsSafeUnderConcurrentUse(t *testing.T) {
	nav := &NavigationService{}
	nav.setPendingLink(PendingLink{Invalid: true})
	taken := make(chan PendingLink, 8)
	for range 8 {
		go func() { taken <- nav.TakePendingLink() }()
	}
	got := 0
	for range 8 {
		if (<-taken).Invalid {
			got++
		}
	}
	if got != 1 {
		t.Fatalf("%d callers got the link, want exactly 1", got)
	}
}

func TestURLEventCarriesLinkOnlyOnMac(t *testing.T) {
	for goos, want := range map[string]bool{"darwin": true, "windows": false, "linux": false} {
		if got := urlEventCarriesLink(goos); got != want {
			t.Errorf("%s = %v, want %v", goos, got, want)
		}
	}
}

func TestLaunchFromURLReadsAsALinkLaunch(t *testing.T) {
	link, requested, err := linkTarget(launchFromURL("typhon://game?igdb=7"))
	if err != nil || !requested || link.Target.ID != "7" {
		t.Fatalf("linkTarget = %+v, %v, %v", link, requested, err)
	}
}

func TestRegisterProtocol(t *testing.T) {
	root := t.TempDir()
	install := filepath.Join(root, "Typhon")
	other := filepath.Join(root, "bin")
	for _, dir := range []string{install, other} {
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	installed := filepath.Join(install, "typhon.exe")
	built := filepath.Join(other, "typhon.exe")
	for _, exe := range []string{installed, built} {
		f, err := os.Create(exe)
		if err != nil {
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
	}
	release := protocolOwner{production: true}
	locate := errors.New("no executable path")
	unreadable := errors.New("registry denied")
	denied := errors.New("access denied")
	cases := []struct {
		name        string
		owner       protocolOwner
		exe         string
		exeErr      error
		dir         string
		recorded    bool
		dirErr      error
		registerErr error
		want        []string
		err         error
	}{
		{name: "installed copy", owner: release, exe: installed, dir: install, recorded: true, want: []string{installed}},
		{name: "development build", owner: protocolOwner{}, exe: installed, dir: install, recorded: true},
		{name: "portable copy", owner: protocolOwner{production: true, portable: true}, exe: installed, dir: install, recorded: true},
		{name: "qa build", owner: protocolOwner{production: true, qa: true}, exe: installed, dir: install, recorded: true},
		{name: "bin output beside an install", owner: release, exe: built, dir: install, recorded: true},
		{name: "unpacked exe with no install recorded", owner: release, exe: built},
		{name: "installed path without the record", owner: release, exe: installed},
		{name: "empty record", owner: release, exe: installed, recorded: true},
		{name: "removed install", owner: release, exe: built, dir: filepath.Join(root, "gone"), recorded: true},
		{name: "unreadable record", owner: release, exe: installed, dirErr: unreadable, err: unreadable},
		{name: "unknown path", owner: release, exeErr: locate, dir: install, recorded: true, err: locate},
		{name: "refused registry", owner: release, exe: installed, dir: install, recorded: true, registerErr: denied, want: []string{installed}, err: denied},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got []string
			err := registerProtocol(tc.owner,
				func() (string, error) { return tc.exe, tc.exeErr },
				func() (string, bool, error) { return tc.dir, tc.recorded, tc.dirErr },
				func(exe string) error {
					got = append(got, exe)
					return tc.registerErr
				})
			if !errors.Is(err, tc.err) || !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("registerProtocol = %v, registered %v; want %v, %v", err, got, tc.err, tc.want)
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
