package main

import (
	"bytes"
	"log/slog"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func TestSecondLaunchesQueueUntilReady(t *testing.T) {
	var s secondLaunches
	s.Receive([]string{"a"})
	s.Receive([]string{"b"})
	var got [][]string
	s.Ready(func(args []string) { got = append(got, args) })
	s.Receive([]string{"c"})
	want := [][]string{{"a"}, {"b"}, {"c"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestSecondLaunchesPanicDoesNotStopTheHandler(t *testing.T) {
	var s secondLaunches
	s.Receive([]string{"boom"})
	calls := 0
	s.Ready(func(args []string) {
		calls++
		if args[0] == "boom" {
			panic("handler failed")
		}
	})
	s.Receive([]string{"after"})
	if calls != 2 {
		t.Fatalf("handler ran %d times, want 2", calls)
	}
}

func TestSecondLaunchesConcurrentReceiveLosesNothing(t *testing.T) {
	var s secondLaunches
	var mu sync.Mutex
	count := 0
	var wg sync.WaitGroup
	for range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.Receive([]string{"x"})
		}()
	}
	s.Ready(func([]string) {
		mu.Lock()
		count++
		mu.Unlock()
	})
	wg.Wait()
	mu.Lock()
	defer mu.Unlock()
	if count != 50 {
		t.Fatalf("delivered %d of 50", count)
	}
}

type recordedPlayer struct {
	id, profile string
	err         error
}

func (p *recordedPlayer) PlayGameProfile(id, profile string) error {
	p.id, p.profile = id, profile
	return p.err
}

type countOpener struct{ n int }

func (o *countOpener) Open() { o.n++ }

func TestHandleSecondLaunch(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		playErr   error
		wantOpens int
		wantID    string
		wantRoute string
	}{
		{name: "plain launch opens the window", args: []string{"typhon.exe"}, wantOpens: 1},
		{name: "open route navigates", args: []string{"typhon.exe", "--open", "downloads"}, wantOpens: 1, wantRoute: "downloads"},
		{name: "play starts the game without opening", args: []string{"typhon.exe", "--play", "g1"}, wantID: "g1"},
		{name: "failed play reveals the window", args: []string{"typhon.exe", "--play", "g1"}, playErr: errNoPlayTarget, wantID: "g1", wantOpens: 1},
		{name: "broken play argument reveals the window", args: []string{"typhon.exe", "--play"}, wantOpens: 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			player := &recordedPlayer{err: tc.playErr}
			opener := &countOpener{}
			nav := &NavigationService{}
			route := ""
			handleSecondLaunch(tc.args, player, opener, nav, func(r string) { route = r })
			if opener.n != tc.wantOpens || player.id != tc.wantID || route != tc.wantRoute {
				t.Fatalf("opens=%d id=%q route=%q", opener.n, player.id, route)
			}
		})
	}
}

func TestHandleSecondLaunchLink(t *testing.T) {
	tests := []struct {
		name string
		args []string
		link PendingLink
	}{
		{"link opens the window and waits for the page", []string{"typhon.exe", "typhon://game?steam=440"}, PendingLink{Provider: "steam", ID: "440"}},
		{"bad link is reported to the page", []string{"typhon.exe", "typhon://game?steam=x"}, PendingLink{Invalid: true}},
		{"injected play never plays", []string{"typhon.exe", "typhon://game?steam=1", "--play", "g1"}, PendingLink{Invalid: true}},
		{"search link opens the window and waits for the page", []string{"typhon.exe", "typhon://search?q=Half%20Life"}, PendingLink{Query: "Half Life"}},
		{"play flag in a query stays text", []string{"typhon.exe", "typhon://search?q=--play%20g1"}, PendingLink{Query: "--play g1"}},
		{"search with injected play never plays", []string{"typhon.exe", "typhon://search?q=a", "--play", "g1"}, PendingLink{Invalid: true}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			player := &recordedPlayer{}
			opener := &countOpener{}
			nav := &NavigationService{}
			emitted := []string{}
			handleSecondLaunch(tc.args, player, opener, nav, func(r string) { emitted = append(emitted, r) })
			if opener.n != 1 || player.id != "" {
				t.Fatalf("opens=%d played=%q", opener.n, player.id)
			}
			if !reflect.DeepEqual(emitted, []string{""}) {
				t.Fatalf("emitted %q, want one event without a route", emitted)
			}
			if got := nav.TakePendingLink(); got != tc.link {
				t.Fatalf("pending = %+v, want %+v", got, tc.link)
			}
		})
	}
}

func TestSearchLinkFromURLEventTakesTheSameQueue(t *testing.T) {
	var s secondLaunches
	s.Receive(launchFromURL("typhon://search?q=Hades"))
	nav := &NavigationService{}
	player := &recordedPlayer{}
	opener := &countOpener{}
	events := 0
	s.Ready(func(args []string) {
		handleSecondLaunch(args, player, opener, nav, func(string) { events++ })
	})
	if opener.n != 1 || events != 1 || player.id != "" {
		t.Fatalf("opens=%d events=%d played=%q", opener.n, events, player.id)
	}
	if got := nav.TakePendingLink(); got != (PendingLink{Query: "Hades"}) {
		t.Fatalf("pending = %+v", got)
	}
}

func TestPanicLogDoesNotCarryTheLink(t *testing.T) {
	var logged bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logged, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	runRecovered(func([]string) { panic("boom") }, []string{"typhon", "typhon://search?q=secret-payload"})

	if !strings.Contains(logged.String(), "second launch handler panicked") {
		t.Fatalf("panic was not logged: %q", logged.String())
	}
	if strings.Contains(logged.String(), "secret-payload") {
		t.Fatalf("log carries the link: %q", logged.String())
	}
}

func TestLinkDeliveredOnceThroughTheQueue(t *testing.T) {
	var s secondLaunches
	s.Receive(launchFromURL("typhon://game?igdb=7"))
	nav := &NavigationService{}
	opener := &countOpener{}
	events := 0
	s.Ready(func(args []string) {
		handleSecondLaunch(args, &recordedPlayer{}, opener, nav, func(string) { events++ })
	})
	if opener.n != 1 || events != 1 {
		t.Fatalf("opens=%d events=%d", opener.n, events)
	}
	if got := nav.TakePendingLink(); got != (PendingLink{Provider: "igdb", ID: "7"}) {
		t.Fatalf("pending = %+v", got)
	}
}

func TestIsWorkerLaunch(t *testing.T) {
	for args, want := range map[string]bool{"--install-worker": true, "--install-broker": true, "--selfupdate-worker": true, "--play": false} {
		if got := isWorkerLaunch([]string{"typhon.exe", args}); got != want {
			t.Errorf("%s = %v", args, got)
		}
	}
	if isWorkerLaunch([]string{"typhon.exe"}) {
		t.Error("no arguments is not a worker")
	}
}
