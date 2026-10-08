package install

import (
	"path/filepath"
	"testing"
	"time"
)

type finishedLog struct {
	ch chan Installation
}

func (r *rig) finished() *finishedLog {
	log := &finishedLog{ch: make(chan Installation, 16)}
	r.s.SetOnFinished(func(item Installation) { log.ch <- item })
	return log
}

func (f *finishedLog) next(t *testing.T) Installation {
	t.Helper()
	select {
	case item := <-f.ch:
		return item
	case <-time.After(15 * time.Second):
		t.Fatal("onFinished was never called")
		return Installation{}
	}
}

// drain отдаёт всё, что пришло сверх уже прочитанного; вызывать после остановки
// сервиса, когда незавершённых обратных вызовов уже нет.
func (f *finishedLog) drain() []Installation {
	var rest []Installation
	for {
		select {
		case item := <-f.ch:
			rest = append(rest, item)
		default:
			return rest
		}
	}
}

// Сервис обновлений узнаёт о конце установки только через onFinished (main.go):
// потерянный или двойной вызов оставляет обновление висеть или применить дважды.
func TestOnFinishedReportsEveryOutcomeExactlyOnce(t *testing.T) {
	cases := []struct {
		name  string
		build func(t *testing.T, r *rig) (item func() Installation, settle func(id string))
		want  Status
	}{
		{"completed", func(t *testing.T, r *rig) (func() Installation, func(string)) {
			root := t.TempDir()
			portableSource(t, root, "Game")
			r.download("d1", "Game", root)
			dest := filepath.Join(t.TempDir(), "Game")
			return func() Installation {
				item, err := r.s.Start("d1", StartOptions{Destination: dest, Mode: ModeCopy})
				if err != nil {
					t.Fatalf("Start: %v", err)
				}
				return item
			}, func(id string) { r.settle(id) }
		}, StatusCompleted},
		{"failed", func(t *testing.T, r *rig) (func() Installation, func(string)) {
			dest := silentSource(t, r, rgInnoMarker)
			r.setRunner(newRgRunner(t, rgStep{code: 1}))
			return func() Installation {
				item, err := r.s.Start("d1", StartOptions{Destination: dest})
				if err != nil {
					t.Fatalf("Start: %v", err)
				}
				return item
			}, func(id string) { r.settle(id) }
		}, StatusFailed},
		{"cancelled", func(t *testing.T, r *rig) (func() Installation, func(string)) {
			dest := silentSource(t, r, rgInnoMarker)
			run := newRgRunner(t, rgStep{wait: true})
			r.setRunner(run)
			return func() Installation {
					item, err := r.s.Start("d1", StartOptions{Destination: dest})
					if err != nil {
						t.Fatalf("Start: %v", err)
					}
					return item
				}, func(id string) {
					run.entered(0)
					if err := r.s.Cancel(id); err != nil {
						t.Fatalf("Cancel: %v", err)
					}
					r.settle(id)
				}
		}, StatusCancelled},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newRig(t)
			log := r.finished()
			start, settle := tc.build(t, r)
			item := start()
			settle(item.ID)

			got := log.next(t)
			if got.ID != item.ID || got.Status != tc.want {
				t.Fatalf("onFinished got %s %s, want %s %s", got.ID, got.Status, item.ID, tc.want)
			}
			r.restart()
			if rest := log.drain(); len(rest) != 0 {
				t.Fatalf("onFinished was called again: %+v", rest)
			}
		})
	}
}

func TestOnFinishedWaitsForTheExecutableChoice(t *testing.T) {
	r := newRig(t)
	log := r.finished()
	installed := interactiveSource(t, r)
	r.setRunner(newRgRunner(t, rgStep{act: func(runSpec) { mkFile(t, filepath.Join(installed, "MyGame.exe"), 512<<10) }}))
	item, err := r.s.Start("d1", StartOptions{})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if got := r.settle(item.ID); got.Status != StatusWaitingForUser {
		t.Fatalf("status = %s", got.Status)
	}
	select {
	case got := <-log.ch:
		t.Fatalf("onFinished fired while the install still waits for the player: %+v", got)
	default:
	}

	if err := r.s.ConfirmExecutable(item.ID, filepath.Join(installed, "MyGame.exe")); err != nil {
		t.Fatalf("ConfirmExecutable: %v", err)
	}
	if got := log.next(t); got.ID != item.ID || got.Status != StatusCompleted {
		t.Fatalf("onFinished got %s %s", got.ID, got.Status)
	}
}

func TestOnFinishedIsSilentWhenTheLauncherClosesMidInstall(t *testing.T) {
	r := newRig(t)
	log := r.finished()
	dest := silentSource(t, r, rgInnoMarker)
	run := newRgRunner(t, rgStep{wait: true})
	r.setRunner(run)
	if _, err := r.s.Start("d1", StartOptions{Destination: dest}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	run.entered(0)
	r.shutdown()
	if rest := log.drain(); len(rest) != 0 {
		t.Fatalf("closing the launcher reported %+v as finished", rest)
	}
}

func TestDeleteDownloadDataGoesToTheDownloadManager(t *testing.T) {
	r := newRig(t)
	root := t.TempDir()
	portableSource(t, root, "Game")
	r.download("d1", "Game", root)
	if err := r.s.DeleteDownloadData("d1"); err != nil {
		t.Fatalf("DeleteDownloadData: %v", err)
	}
	if got := r.downloads.deletedIDs(); len(got) != 1 || got[0] != "d1" {
		t.Fatalf("deleted downloads = %v", got)
	}
}

func TestNewServiceWiresItsDependenciesAndTheStateFolder(t *testing.T) {
	home := t.TempDir()
	t.Setenv("APPDATA", home)
	t.Setenv("XDG_CONFIG_HOME", home)
	t.Setenv("HOME", home)
	s, err := NewService(nil, nil, nil)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	if s.downloads != nil || s.library != nil {
		t.Fatalf("missing dependencies became non-nil interfaces: downloads %v library %v", s.downloads, s.library)
	}
	if s.store == nil || s.store.dir == "" || s.runner == nil || s.prepareRuntime == nil || s.releaseRuntime == nil {
		t.Fatalf("service is not fully wired: %+v", s)
	}
	if _, err := s.Start("d1", StartOptions{}); errCode(err) != "install.no_downloads" {
		t.Fatalf("Start without a download manager = %v, want install.no_downloads", err)
	}
	if got := s.List(); len(got) != 0 {
		t.Fatalf("a new service lists %+v", got)
	}
}
