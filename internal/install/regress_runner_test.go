package install

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"
)

// rgStep описывает один запуск установщика: что он делает на диске, чем
// кончается и держит ли поток до отмены. Последний шаг повторяется для всех
// следующих запусков.
type rgStep struct {
	act  func(spec runSpec)
	code int
	err  error
	log  string
	wait bool
}

type rgRunner struct {
	t     *testing.T
	mu    sync.Mutex
	steps []rgStep
	specs []runSpec
	enter chan int
}

func newRgRunner(t *testing.T, steps ...rgStep) *rgRunner {
	t.Helper()
	if len(steps) == 0 {
		steps = []rgStep{{}}
	}
	return &rgRunner{t: t, steps: steps, enter: make(chan int, 32)}
}

func (r *rgRunner) run(ctx context.Context, spec runSpec) (int, error) {
	r.mu.Lock()
	n := len(r.specs)
	r.specs = append(r.specs, spec)
	step := r.steps[min(n, len(r.steps)-1)]
	r.mu.Unlock()
	select {
	case r.enter <- n:
	default:
	}
	if step.act != nil {
		step.act(spec)
	}
	if step.log != "" && spec.LogPath != "" {
		if err := os.WriteFile(spec.LogPath, utf16Log(step.log), 0o600); err != nil {
			r.t.Errorf("write installer log: %v", err)
		}
	}
	if step.wait {
		<-ctx.Done()
		return 0, ctx.Err()
	}
	return step.code, step.err
}

func (r *rgRunner) calls() []runSpec {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]runSpec(nil), r.specs...)
}

// entered ждёт, пока установщик с номером n начнёт работу.
func (r *rgRunner) entered(n int) {
	r.t.Helper()
	deadline := time.NewTimer(15 * time.Second)
	defer deadline.Stop()
	for {
		select {
		case got := <-r.enter:
			if got >= n {
				return
			}
		case <-deadline.C:
			r.t.Fatalf("installer %d never started", n)
		}
	}
}

// rgDoneHandle — воркер, уже отработавший к моменту запуска.
type rgDoneHandle struct{}

func (rgDoneHandle) wait() (int, error) { return 0, nil }
func (rgDoneHandle) close()             {}
func (rgDoneHandle) terminate() error   { return nil }

// rgGateHandle — воркер, который живёт, пока его не отпустят.
type rgGateHandle struct {
	release chan struct{}
	once    sync.Once
}

func (h *rgGateHandle) wait() (int, error) {
	<-h.release
	return 0, nil
}

func (h *rgGateHandle) close() {}

func (h *rgGateHandle) terminate() error {
	h.stop()
	return nil
}

func (h *rgGateHandle) stop() {
	h.once.Do(func() { close(h.release) })
}

// rgWorker подставляется вместо startElevatedWorker: запуска UAC нет, а то, что
// сделал бы повышенный воркер, выполняет act. Состояние в файл пишет сам рабочий,
// поэтому помеченное токеном прогона Done: true читается так же, как настоящее.
type rgWorker struct {
	t        *testing.T
	mu       sync.Mutex
	specs    []workerSpec
	act      func(ws workerSpec) workerState
	silent   bool
	startErr error
}

func (w *rgWorker) install() {
	w.t.Helper()
	withWorkerSeams(w.t, w.launch)
}

func (w *rgWorker) all() []workerSpec {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]workerSpec(nil), w.specs...)
}

func (w *rgWorker) launch(rs runSpec) (workerHandle, error) {
	if w.startErr != nil {
		return nil, w.startErr
	}
	if len(rs.Args) < 2 {
		return nil, errors.New("rgWorker: launched without a spec")
	}
	specFile, digest := ParseWorkerArgs(rs.Args[1:])
	ws, err := readVerifiedWorkerSpec(specFile, digest)
	if err != nil {
		return nil, err
	}
	w.mu.Lock()
	w.specs = append(w.specs, ws)
	w.mu.Unlock()
	state := workerState{}
	if w.act != nil {
		state = w.act(ws)
	}
	if w.silent {
		return rgDoneHandle{}, nil
	}
	state.Run = ws.Run
	state.Done = true
	if err := writeWorkerState(ws.StatePath, state); err != nil {
		return nil, err
	}
	return rgDoneHandle{}, nil
}

// launchGated оставляет воркера жить до маркера отмены и только тогда пишет
// подтверждение: так выглядит воркер, которого отменили посреди установки.
func (w *rgWorker) launchGated(started chan<- struct{}) func(runSpec) (workerHandle, error) {
	return func(rs runSpec) (workerHandle, error) {
		specFile, digest := ParseWorkerArgs(rs.Args[1:])
		ws, err := readVerifiedWorkerSpec(specFile, digest)
		if err != nil {
			return nil, err
		}
		w.mu.Lock()
		w.specs = append(w.specs, ws)
		w.mu.Unlock()
		if w.act != nil {
			w.act(ws)
		}
		handle := &rgGateHandle{release: make(chan struct{})}
		go func() {
			ticker := time.NewTicker(2 * time.Millisecond)
			defer ticker.Stop()
			deadline := time.NewTimer(15 * time.Second)
			defer deadline.Stop()
			started <- struct{}{}
			for {
				select {
				case <-ticker.C:
					if !exists(ws.CancelPath) {
						continue
					}
					state := workerState{Run: ws.Run, Done: true, Cancelled: true, Error: context.Canceled.Error()}
					if err := writeWorkerState(ws.StatePath, state); err != nil {
						w.t.Errorf("write worker state: %v", err)
					}
					handle.stop()
					return
				case <-deadline.C:
					w.t.Errorf("cancel marker never appeared at %s", ws.CancelPath)
					handle.stop()
					return
				}
			}
		}()
		return handle, nil
	}
}
