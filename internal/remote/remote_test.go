package remote

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"

	"typhon/internal/account"
	"typhon/internal/download"
)

type fakeDownloads struct {
	mu    sync.Mutex
	items []download.Download
	calls []string
	fail  map[string]bool
}

func (f *fakeDownloads) List() []download.Download {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]download.Download(nil), f.items...)
}

func (f *fakeDownloads) act(name, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, name+":"+id)
	if f.fail[id] {
		return errors.New("boom")
	}
	return nil
}

func (f *fakeDownloads) Pause(id string) error {
	if err := f.act("pause", id); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.items {
		if f.items[i].ID == id {
			f.items[i].Status = download.StatusPaused
		}
	}
	return nil
}
func (f *fakeDownloads) Resume(id string) error     { return f.act("resume", id) }
func (f *fakeDownloads) ForceStart(id string) error { return f.act("start", id) }
func (f *fakeDownloads) Cancel(id string) error     { return f.act("cancel", id) }

func newTestService(t *testing.T, enabled bool, dl *fakeDownloads) *Service {
	t.Helper()
	on := enabled
	s, err := New(context.Background(), Deps{
		API: &fakeAPI{}, Downloads: dl, InstallationID: "inst-aaaaaaaa",
		Enabled: func() bool { return on },
		Running: func() []string { return []string{"Hades"} },
		Log:     slog.New(slog.DiscardHandler),
	})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func queue() *fakeDownloads {
	return &fakeDownloads{items: []download.Download{
		{ID: "a1", Name: "Hollow Knight", Status: download.StatusDownloading, Progress: 0.5, DownloadSpeed: 1000, ETASeconds: 60},
		{ID: "b2", Name: "Hollow Ships", Status: download.StatusPaused, Progress: 0.1},
		{ID: "c3", Name: "Celeste", Status: download.StatusQueued},
		{ID: "d4", Name: "Portal 2", Status: download.StatusCompleted, Progress: 1},
	}}
}

func TestRunRefusesWhenControlIsOff(t *testing.T) {
	dl := queue()
	s := newTestService(t, false, dl)
	for _, action := range []string{actionStatus, actionPause, actionCancel} {
		if a := s.Run(account.RemoteCommand{Action: action, Target: "a1"}); a.Status != "failed" || a.ErrorCode != codeDisabled {
			t.Fatalf("%s with control off: %+v", action, a)
		}
	}
	if len(dl.calls) != 0 {
		t.Fatalf("calls with control off: %v", dl.calls)
	}
}

func TestStatusListsDownloadsAndRunningGames(t *testing.T) {
	s := newTestService(t, true, queue())
	a := s.Run(account.RemoteCommand{Action: actionStatus})
	var got statusResult
	if err := json.Unmarshal(a.Result, &got); err != nil || a.Status != "done" {
		t.Fatalf("status %+v: %v", a, err)
	}
	if len(got.Running) != 1 || got.Running[0] != "Hades" || len(got.Downloads) != 4 {
		t.Fatalf("status %+v", got)
	}
	if got.Downloads[0].DownloadSpeed != 1000 || got.Downloads[1].DownloadSpeed != 0 {
		t.Fatalf("speed only for an active download: %+v", got.Downloads)
	}
}

func TestActionsPickTheirDownloads(t *testing.T) {
	for _, tc := range []struct {
		name, action, target string
		want                 []string
		code                 string
	}{
		{"pause all active", actionPause, "", []string{"pause:a1", "pause:c3"}, ""},
		{"pause by id", actionPause, "a1", []string{"pause:a1"}, ""},
		{"resume all paused", actionResume, "ALL", []string{"resume:b2"}, ""},
		{"resume by name part", actionResume, "ships", []string{"resume:b2"}, ""},
		{"start a queued one", actionStart, "celeste", []string{"start:c3"}, ""},
		{"start needs one download", actionStart, "", nil, codeNotFound},
		{"ambiguous name", actionPause, "hollow", nil, codeAmbiguous},
		{"unknown name", actionPause, "doom", nil, codeNotFound},
		{"nothing to resume", actionResume, "celeste", nil, codeNothing},
		{"cancel by name", actionCancel, "Celeste", []string{"cancel:c3"}, ""},
		{"cancel never all", actionCancel, "all", nil, codeNotFound},
		{"cancel never empty", actionCancel, "", nil, codeNotFound},
		{"finished download is not cancelled", actionCancel, "portal", nil, codeNothing},
		{"launching is not remote", "launch", "a1", nil, codeUnknown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dl := queue()
			a := newTestService(t, true, dl).Run(account.RemoteCommand{Action: tc.action, Target: tc.target})
			if tc.code != "" {
				if a.Status != "failed" || a.ErrorCode != tc.code {
					t.Fatalf("answer %+v, want %s", a, tc.code)
				}
			} else if a.Status != "done" {
				t.Fatalf("answer %+v", a)
			}
			if len(dl.calls) != len(tc.want) {
				t.Fatalf("calls %v, want %v", dl.calls, tc.want)
			}
			for i := range tc.want {
				if dl.calls[i] != tc.want[i] {
					t.Fatalf("calls %v, want %v", dl.calls, tc.want)
				}
			}
		})
	}
}

func TestAFailedActionIsReported(t *testing.T) {
	dl := queue()
	dl.fail = map[string]bool{"a1": true}
	s := newTestService(t, true, dl)
	if a := s.Run(account.RemoteCommand{Action: actionPause, Target: "a1"}); a.ErrorCode != codeFailed {
		t.Fatalf("single failure: %+v", a)
	}
	// With several, the ones that worked are reported.
	a := s.Run(account.RemoteCommand{Action: actionPause, Target: "all"})
	var got actionResult
	if err := json.Unmarshal(a.Result, &got); err != nil || a.Status != "done" || len(got.Affected) != 1 || got.Affected[0].ID != "c3" {
		t.Fatalf("partial failure: %+v %s %v", a, a.Result, err)
	}
}

type fakeAPI struct {
	mu       sync.Mutex
	fetches  atomic.Int32
	commands []account.RemoteCommand
	answers  map[string]account.RemoteAnswer
	gate     chan struct{}
}

func (f *fakeAPI) RemoteCommands(ctx context.Context, inst string) ([]account.RemoteCommand, error) {
	f.fetches.Add(1)
	if f.gate != nil {
		<-f.gate
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	out := f.commands
	f.commands = nil
	return out, nil
}

func (f *fakeAPI) FinishRemoteCommand(ctx context.Context, inst, id string, a account.RemoteAnswer) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.answers == nil {
		f.answers = map[string]account.RemoteAnswer{}
	}
	f.answers[id] = a
	return nil
}

func TestWakeRunsAndAnswersEachCommandAndFoldsBursts(t *testing.T) {
	dl := queue()
	api := &fakeAPI{commands: []account.RemoteCommand{{ID: "c-1", Action: actionStatus}, {ID: "c-2", Action: actionPause, Target: "a1"}}, gate: make(chan struct{})}
	s, err := New(context.Background(), Deps{
		API: api, Downloads: dl, InstallationID: "inst-aaaaaaaa",
		Enabled: func() bool { return true }, Running: func() []string { return nil },
		Log: slog.New(slog.DiscardHandler),
	})
	if err != nil {
		t.Fatal(err)
	}
	for range 5 {
		s.Wake()
	}
	close(api.gate)
	s.Wait()
	if n := api.fetches.Load(); n != 2 {
		t.Fatalf("five wakes during one pass made %d fetches, want 2", n)
	}
	if api.answers["c-1"].Status != "done" || api.answers["c-2"].Status != "done" {
		t.Fatalf("answers %+v", api.answers)
	}
	var status statusResult
	if err := json.Unmarshal(api.answers["c-1"].Result, &status); err != nil || status.Running == nil {
		t.Fatalf("status result %s: %v", api.answers["c-1"].Result, err)
	}
}

func TestWakeDoesNothingWhenControlIsOff(t *testing.T) {
	api := &fakeAPI{}
	s, err := New(context.Background(), Deps{
		API: api, Downloads: queue(), InstallationID: "inst-aaaaaaaa",
		Enabled: func() bool { return false }, Running: func() []string { return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	// With control off Wake returns without starting a pass.
	s.Wake()
	s.Wait()
	if api.fetches.Load() != 0 {
		t.Fatal("a launcher with control off asked for commands")
	}
}

func TestActionReportsTheStateAfterIt(t *testing.T) {
	a := newTestService(t, true, queue()).Run(account.RemoteCommand{Action: actionPause, Target: "a1"})
	var got actionResult
	if err := json.Unmarshal(a.Result, &got); err != nil || len(got.Affected) != 1 || got.Affected[0].Status != string(download.StatusPaused) {
		t.Fatalf("affected %s: %v", a.Result, err)
	}
}
