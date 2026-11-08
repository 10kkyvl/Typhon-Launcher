package remote

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"typhon/internal/account"
	"typhon/internal/download"
	"typhon/internal/sources"
	"typhon/internal/uierr"
)

type fakeReleases struct{ views []sources.ReleaseView }

func (f fakeReleases) SearchDownloadable(q string, limit int) []sources.ReleaseView {
	var out []sources.ReleaseView
	for _, v := range f.views {
		if strings.Contains(strings.ToLower(v.Release.RawTitle), strings.ToLower(q)) && len(out) < limit {
			out = append(out, v)
		}
	}
	return out
}

func (f fakeReleases) PrepareDownloadable(id string) (sources.DownloadRequest, error) {
	for _, v := range f.views {
		if v.Release.ID == id {
			return sources.DownloadRequest{URI: "magnet:" + id, Name: v.Release.RawTitle, ReleaseID: id, SourceID: v.Release.SourceID, Version: "1.4"}, nil
		}
	}
	return sources.DownloadRequest{}, errors.New("release not found")
}

type fakeStarter struct {
	mu        sync.Mutex
	gate      chan struct{}
	fetchErr  error
	startErr  error
	started   []string
	dest      string
	origin    download.Origin
	discarded []string
	selected  []int
}

func (f *fakeStarter) FetchMetadata(source string) (download.TorrentInfo, error) {
	if f.gate != nil {
		<-f.gate
	}
	if f.fetchErr != nil {
		return download.TorrentInfo{}, f.fetchErr
	}
	return download.TorrentInfo{InfoHash: "hash-" + source, Files: make([]download.FileState, 3)}, nil
}

func (f *fakeStarter) StartDownloadFrom(hash, dest string, sel []int, origin download.Origin) (download.Download, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.startErr != nil {
		return download.Download{}, f.startErr
	}
	f.started, f.dest, f.origin, f.selected = append(f.started, hash), dest, origin, sel
	return download.Download{ID: "new"}, nil
}

func (f *fakeStarter) DiscardMetadata(hash string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.discarded = append(f.discarded, hash)
}

type sent struct{ kind, title, code string }

type addEnv struct {
	svc     *Service
	starter *fakeStarter
	mu      sync.Mutex
	sent    []sent
	addOn   bool
	dl      *fakeDownloads
}

func (e *addEnv) messages() []sent {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]sent(nil), e.sent...)
}

func newAddEnv(t *testing.T) *addEnv {
	t.Helper()
	e := &addEnv{starter: &fakeStarter{}, addOn: true, dl: queue()}
	releases := fakeReleases{views: []sources.ReleaseView{
		{Release: sources.Release{ID: "rel-1", SourceID: "src", RawTitle: "Celeste v1.4", Title: "Celeste", Version: "1.4", Repacker: "Team", Size: 1 << 30}, SourceName: "My feed", GameTitle: "Celeste"},
		{Release: sources.Release{ID: "rel-2", SourceID: "src", RawTitle: "Celeste Farewell", Title: "Celeste Farewell", SizeUnknown: true, Size: 99}, SourceName: "My feed"},
	}}
	svc, err := New(context.Background(), Deps{
		API: &fakeAPI{}, Downloads: e.dl, InstallationID: "inst-aaaaaaaa",
		Enabled:  func() bool { return true },
		Running:  func() []string { return nil },
		Log:      slog.New(slog.DiscardHandler),
		Releases: releases, Starter: e.starter,
		DownloadsPath: func() string { return "/games/downloads" },
		AddEnabled: func() bool {
			e.mu.Lock()
			defer e.mu.Unlock()
			return e.addOn
		},
		Notify: func(kind, title, code string) {
			e.mu.Lock()
			defer e.mu.Unlock()
			e.sent = append(e.sent, sent{kind, title, code})
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	e.svc = svc
	return e
}

func TestSearchListsReleasesForTheBot(t *testing.T) {
	e := newAddEnv(t)
	a := e.svc.Run(account.RemoteCommand{Action: actionSearch, Target: "celeste"})
	var got searchResult
	if err := json.Unmarshal(a.Result, &got); err != nil || a.Status != "done" {
		t.Fatalf("search %+v: %v", a, err)
	}
	if len(got.Releases) != 2 || got.Releases[0].Title != "Celeste" || got.Releases[0].Source != "My feed" ||
		got.Releases[0].Size != 1<<30 || got.Releases[0].Repacker != "Team" || got.Releases[1].Size != 0 {
		t.Fatalf("releases %+v", got.Releases)
	}
	e.addOn = false
	if a := e.svc.Run(account.RemoteCommand{Action: actionSearch, Target: "celeste"}); a.ErrorCode != codeAddOff {
		t.Fatalf("search without permission to add: %+v", a)
	}
}

func TestAddStartsInTheBackgroundAndReports(t *testing.T) {
	e := newAddEnv(t)
	a := e.svc.Run(account.RemoteCommand{Action: actionAdd, Target: "rel-1"})
	if a.Status != "done" || !strings.Contains(string(a.Result), "Celeste v1.4") {
		t.Fatalf("add %+v", a)
	}
	e.svc.Wait()
	if len(e.starter.started) != 1 || e.starter.dest != "/games/downloads" || len(e.starter.selected) != 3 {
		t.Fatalf("started %v in %q with %v", e.starter.started, e.starter.dest, e.starter.selected)
	}
	if o := e.starter.origin; o.ReleaseID != "rel-1" || o.SourceID != "src" || o.Version != "1.4" || o.Purpose != download.PurposeRelease || o.AutoInstall != nil {
		t.Fatalf("origin %+v", o)
	}
	if got := e.messages(); len(got) != 1 || got[0] != (sent{KindAddStarted, "Celeste v1.4", ""}) {
		t.Fatalf("messages %+v", got)
	}
}

func TestAddFailuresAreAnsweredOrReported(t *testing.T) {
	for _, tc := range []struct {
		name   string
		target string
		setup  func(e *addEnv)
		code   string
		report sent
	}{
		{"permission off", "rel-1", func(e *addEnv) { e.addOn = false }, codeAddOff, sent{}},
		{"unknown release", "rel-9", nil, codeReleaseMissing, sent{}},
		{"already downloading", "rel-1", func(e *addEnv) {
			e.dl.items = append(e.dl.items, download.Download{ID: "x", Status: download.StatusPaused, Origin: download.Origin{ReleaseID: "rel-1"}})
		}, codeAlreadyThere, sent{}},
		{"queue full", "rel-1", func(e *addEnv) {
			for range maxOpenForAdd {
				e.dl.items = append(e.dl.items, download.Download{ID: "q", Status: download.StatusQueued})
			}
		}, codeQueueFull, sent{}},
		{"no metadata", "rel-1", func(e *addEnv) {
			e.starter.fetchErr = uierr.New("download.no_metadata", "x")
		}, "", sent{KindAddFailed, "Celeste v1.4", "download.no_metadata"}},
		{"no space", "rel-1", func(e *addEnv) {
			e.starter.startErr = uierr.New("download.not_enough_space", "x")
		}, "", sent{KindAddFailed, "Celeste v1.4", "download.not_enough_space"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newAddEnv(t)
			if tc.setup != nil {
				tc.setup(e)
			}
			a := e.svc.Run(account.RemoteCommand{Action: actionAdd, Target: tc.target})
			e.svc.Wait()
			if tc.code != "" {
				if a.Status != "failed" || a.ErrorCode != tc.code {
					t.Fatalf("answer %+v, want %s", a, tc.code)
				}
				if len(e.starter.started) != 0 || len(e.messages()) != 0 {
					t.Fatalf("a refused add started %v / sent %v", e.starter.started, e.messages())
				}
				return
			}
			if a.Status != "done" {
				t.Fatalf("answer %+v", a)
			}
			if got := e.messages(); len(got) != 1 || got[0] != tc.report {
				t.Fatalf("messages %+v, want %+v", got, tc.report)
			}
			if e.starter.startErr != nil && len(e.starter.discarded) != 1 {
				t.Fatalf("the reserved torrent was not released: %v", e.starter.discarded)
			}
		})
	}
}

func TestAddNeedsControlOnToo(t *testing.T) {
	e := newAddEnv(t)
	e.svc.d.Enabled = func() bool { return false }
	if a := e.svc.Run(account.RemoteCommand{Action: actionAdd, Target: "rel-1"}); a.ErrorCode != codeDisabled {
		t.Fatalf("add with control off: %+v", a)
	}
}

type fakeNotifyAPI struct {
	mu   sync.Mutex
	got  []account.RemoteNotification
	gate chan struct{}
	err  error
}

func (f *fakeNotifyAPI) PostRemoteNotification(ctx context.Context, n account.RemoteNotification) error {
	if f.gate != nil {
		select {
		case <-f.gate:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.got = append(f.got, n)
	return f.err
}

func TestNotifierSendsOnlyWhenOnAndOnlyForGames(t *testing.T) {
	api := &fakeNotifyAPI{}
	on := false
	n, err := NewNotifier(context.Background(), NotifierDeps{API: api, InstallationID: "inst-aaaaaaaa", Enabled: func() bool { return on }, Log: slog.New(slog.DiscardHandler)})
	if err != nil {
		t.Fatal(err)
	}
	n.OnOutcome(download.Download{Name: "Celeste", Status: download.StatusCompleted})
	n.Wait()
	if len(api.got) != 0 {
		t.Fatalf("sent with the setting off: %+v", api.got)
	}
	on = true
	n.OnOutcome(download.Download{Name: "Celeste\nv1.4", Status: download.StatusCompleted})
	n.OnOutcome(download.Download{Name: "x", Status: download.StatusFailed, Origin: download.Origin{Title: "Hades"}})
	n.OnOutcome(download.Download{Name: "patch", Status: download.StatusCompleted, Origin: download.Origin{Purpose: download.PurposeUpdate}})
	n.OnOutcome(download.Download{Name: "paused", Status: download.StatusPaused})
	n.Wait()
	byKind := map[string]string{}
	for _, m := range api.got {
		byKind[m.Kind] = m.Title
		if m.InstallationID != "inst-aaaaaaaa" {
			t.Fatalf("installation %q", m.InstallationID)
		}
	}
	if len(api.got) != 2 || byKind[KindDownloadCompleted] != "Celeste v1.4" || byKind[KindDownloadFailed] != "Hades" {
		t.Fatalf("sent %+v", api.got)
	}
}

func TestNotifierDropsABurstAndStopsAtShutdown(t *testing.T) {
	api := &fakeNotifyAPI{gate: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	n, err := NewNotifier(ctx, NotifierDeps{API: api, InstallationID: "inst-aaaaaaaa", Enabled: func() bool { return true }, Log: slog.New(slog.DiscardHandler)})
	if err != nil {
		t.Fatal(err)
	}
	for range maxNotifyInFlight + 3 {
		n.Send(KindDownloadCompleted, "Celeste", "")
	}
	n.mu.Lock()
	flying := n.flying
	n.mu.Unlock()
	if flying != maxNotifyInFlight {
		t.Fatalf("%d in flight, cap %d", flying, maxNotifyInFlight)
	}
	cancel()
	done := make(chan struct{})
	go func() { n.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Wait did not return after shutdown")
	}
	n.Send(KindDownloadCompleted, "Celeste", "")
	n.Wait()
	if len(api.got) != 0 {
		t.Fatalf("sent after shutdown: %+v", api.got)
	}
}

func TestABurstOfAddsCannotPassTheLimitOrDuplicate(t *testing.T) {
	e := newAddEnv(t)
	e.dl.items = nil
	e.starter.gate = make(chan struct{})
	views := make([]sources.ReleaseView, 0, maxOpenForAdd+5)
	for i := range maxOpenForAdd + 5 {
		id := "burst-" + string(rune('a'+i))
		views = append(views, sources.ReleaseView{Release: sources.Release{ID: id, RawTitle: id}})
	}
	e.svc.d.Releases = fakeReleases{views: views}
	var wg sync.WaitGroup
	var mu sync.Mutex
	accepted, dup := 0, 0
	for i := range maxOpenForAdd + 5 {
		id := views[i].Release.ID
		for range 2 {
			wg.Go(func() {
				a := e.svc.Run(account.RemoteCommand{Action: actionAdd, Target: id})
				mu.Lock()
				defer mu.Unlock()
				switch {
				case a.Status == "done":
					accepted++
				case a.ErrorCode == codeAlreadyThere:
					dup++
				}
			})
		}
	}
	wg.Wait()
	close(e.starter.gate)
	e.svc.Wait()
	if accepted != maxOpenForAdd {
		t.Fatalf("%d adds accepted while their torrents were fetched, limit %d", accepted, maxOpenForAdd)
	}
	if len(e.starter.started) != maxOpenForAdd {
		t.Fatalf("%d downloads started", len(e.starter.started))
	}
	if dup == 0 {
		t.Fatal("the same release was accepted twice")
	}
}

func TestWithdrawingPermissionStopsAnAddInFlight(t *testing.T) {
	e := newAddEnv(t)
	e.starter.gate = make(chan struct{})
	if a := e.svc.Run(account.RemoteCommand{Action: actionAdd, Target: "rel-1"}); a.Status != "done" {
		t.Fatalf("add %+v", a)
	}
	e.mu.Lock()
	e.addOn = false
	e.mu.Unlock()
	close(e.starter.gate)
	e.svc.Wait()
	if len(e.starter.started) != 0 || len(e.starter.discarded) != 1 {
		t.Fatalf("started %v discarded %v", e.starter.started, e.starter.discarded)
	}
	if got := e.messages(); len(got) != 1 || got[0] != (sent{KindAddFailed, "Celeste v1.4", codeAddOff}) {
		t.Fatalf("messages %+v", got)
	}
}
