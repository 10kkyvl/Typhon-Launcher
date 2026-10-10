package install

import (
	"path/filepath"
	"testing"
	"time"
)

// midRun описывает установку, которую можно остановить посреди работы, и то,
// как после этого довести её до конца повтором.
type midRun struct {
	dest    string
	opts    StartOptions
	started func()
	finish  func()
	exe     string
	chain   int
}

type midRunCase struct {
	name  string
	build func(t *testing.T, r *rig) midRun
}

func midRunCases() []midRunCase {
	return []midRunCase{
		{"silent installer", func(t *testing.T, r *rig) midRun {
			dest := silentSource(t, r, rgInnoMarker)
			run := newRgRunner(t, rgStep{act: func(runSpec) { mkFile(t, filepath.Join(dest, "half.bin"), 1024) }, wait: true})
			r.setRunner(run)
			return midRun{
				dest: dest, opts: StartOptions{Destination: dest}, started: func() { run.entered(0) },
				finish: func() { r.setRunner(newRgRunner(t, rgStep{act: installGame(t, dest)})) },
				exe:    filepath.Join(dest, "Game.exe"),
			}
		}},
		{"second installer of a chain", func(t *testing.T, r *rig) midRun {
			root := t.TempDir()
			cupheadSource(t, root)
			r.download("d1", "Cuphead", root)
			dest := filepath.Join(r.games, "Cuphead")
			install := func(runSpec) { mkFile(t, filepath.Join(dest, "Cuphead.exe"), 8192) }
			run := newRgRunner(t, rgStep{act: install}, rgStep{wait: true})
			r.setRunner(run)
			return midRun{
				dest: dest, opts: StartOptions{Destination: dest}, started: func() { run.entered(1) },
				finish: func() { r.setRunner(newRgRunner(t, rgStep{act: install})) },
				exe:    filepath.Join(dest, "Cuphead.exe"), chain: 2,
			}
		}},
	}
}

func TestFlowCancelMidRun(t *testing.T) {
	cases := midRunCases()
	cases = append(cases, midRunCase{"silent installer in the elevated worker", func(t *testing.T, r *rig) midRun {
		dest := silentSource(t, r, rgInnoMarker)
		started := make(chan struct{}, 1)
		gated := &rgWorker{t: t, act: func(workerSpec) workerState {
			mkFile(t, filepath.Join(dest, "half.bin"), 1024)
			return workerState{}
		}}
		withWorkerSeams(t, gated.launchGated(started))
		workerCancelWait = 30 * time.Second
		r.setRunner(elevatingRunner{})
		return midRun{
			dest: dest, opts: StartOptions{Destination: dest},
			started: func() {
				select {
				case <-started:
				case <-time.After(15 * time.Second):
					t.Fatal("the elevated worker never started")
				}
			},
			finish: func() {
				done := &rgWorker{t: t, act: func(workerSpec) workerState {
					mkFile(t, filepath.Join(dest, "Game.exe"), 8192)
					return workerState{}
				}}
				done.install()
			},
			exe: filepath.Join(dest, "Game.exe"),
		}
	}})
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newRig(t)
			p := tc.build(t, r)
			item, err := r.s.Start("d1", p.opts)
			if err != nil {
				t.Fatalf("Start: %v", err)
			}
			p.started()
			if err := r.s.Cancel(item.ID); err != nil {
				t.Fatalf("Cancel: %v", err)
			}
			got := r.settle(item.ID)
			if got.Status != StatusCancelled || got.Error != "" || got.Progress != 0 {
				t.Fatalf("after cancel: status %s error %q progress %v", got.Status, got.Error, got.Progress)
			}
			if exists(p.dest) {
				t.Fatalf("cancelled install left %s behind", p.dest)
			}
			if games := r.reg.registered(); len(games) != 0 {
				t.Fatalf("a cancelled install registered %+v", games)
			}
			if disk := r.diskItem(item.ID); disk.Status != StatusCancelled {
				t.Fatalf("installations.json holds %s, want cancelled", disk.Status)
			}
			if err := r.s.Cancel(item.ID); errCode(err) != "install.unavailable" {
				t.Fatalf("second Cancel = %v, want install.unavailable", err)
			}

			p.finish()
			if err := r.s.Retry(item.ID); err != nil {
				t.Fatalf("Retry after cancel: %v", err)
			}
			done := r.settle(item.ID)
			if done.Status != StatusCompleted || done.Error != "" || done.CompletedAt == nil || done.Executable != p.exe || done.ChainStep != p.chain {
				t.Fatalf("after retry: %+v", viewOf(done))
			}
			if !done.Owned {
				t.Fatal("a folder that did not exist before the first attempt must stay ours after a retry")
			}
			if games := r.reg.registered(); len(games) != 1 {
				t.Fatalf("registered %d games after the retry, want 1", len(games))
			}
			r.assertDurable(item.ID)
		})
	}
}

func TestFlowLauncherClosedMidRun(t *testing.T) {
	for _, tc := range midRunCases() {
		t.Run(tc.name, func(t *testing.T) {
			r := newRig(t)
			p := tc.build(t, r)
			item, err := r.s.Start("d1", p.opts)
			if err != nil {
				t.Fatalf("Start: %v", err)
			}
			p.started()
			before := r.get(item.ID)
			if !transient(before.Status) {
				t.Fatalf("status %s while the installer runs, want a working one", before.Status)
			}

			r.restart()
			got := r.get(item.ID)
			if got.Status != StatusInterrupted || got.Error != errInterrupted.Error() || uiCode(got.Error) != "install.interrupted" {
				t.Fatalf("after the launcher was closed: status %s error %q", got.Status, got.Error)
			}
			if got.ChainStep != p.chain {
				t.Fatalf("chain step = %d, want %d: the step the installer was on must survive", got.ChainStep, p.chain)
			}
			if disk := r.diskItem(item.ID); disk.Status != StatusInterrupted {
				t.Fatalf("installations.json holds %s, want interrupted", disk.Status)
			}
			if games := r.reg.registered(); len(games) != 0 {
				t.Fatalf("an interrupted install registered %+v", games)
			}

			p.finish()
			if err := r.s.Retry(item.ID); err != nil {
				t.Fatalf("Retry after restart: %v", err)
			}
			done := r.settle(item.ID)
			if done.Status != StatusCompleted || done.Error != "" || done.Executable != p.exe || done.ChainStep != p.chain {
				t.Fatalf("after retry: %+v", viewOf(done))
			}
			if !done.Owned {
				t.Fatal("a folder that did not exist before the first attempt must stay ours after a restart and a retry")
			}
			if games := r.reg.registered(); len(games) != 1 {
				t.Fatalf("registered %d games after the retry, want 1", len(games))
			}
			r.assertDurable(item.ID)
		})
	}
}

// Окно выбора исполняемого файла переживает перезапуск: установщик уже отработал,
// и пользователю не нужно ставить игру заново.
func TestWaitingForUserSurvivesRestart(t *testing.T) {
	r := newRig(t)
	installed := interactiveSource(t, r)
	r.setRunner(newRgRunner(t, rgStep{act: func(runSpec) { mkFile(t, filepath.Join(installed, "MyGame.exe"), 512<<10) }}))
	item, err := r.s.Start("d1", StartOptions{})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	waiting := r.settle(item.ID)
	if waiting.Status != StatusWaitingForUser {
		t.Fatalf("status = %s (%q)", waiting.Status, waiting.Error)
	}

	r.restart()
	again := r.get(item.ID)
	if again.Status != StatusWaitingForUser || again.Destination != installed || len(again.Candidates) != len(waiting.Candidates) {
		t.Fatalf("after restart: %+v", viewOf(again))
	}
	exe := filepath.Join(installed, "MyGame.exe")
	if err := r.s.ConfirmExecutable(item.ID, exe); err != nil {
		t.Fatalf("ConfirmExecutable after restart: %v", err)
	}
	done := r.get(item.ID)
	if done.Status != StatusCompleted || done.Executable != exe || done.GameID == "" {
		t.Fatalf("after confirm: %+v", viewOf(done))
	}
	if n := len(r.reg.registered()); n != 1 {
		t.Fatalf("registered %d games, want 1", n)
	}
}
