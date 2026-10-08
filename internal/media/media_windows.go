package media

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync"
)

type job struct {
	ctx context.Context
	fn  func(context.Context) (any, error)
	res chan jobResult
}

type jobResult struct {
	val any
	err error
}

// WinRT objects belong to the apartment of the thread that created them, so
// every call goes through one goroutine locked to one OS thread.
type apartment struct {
	mu      sync.Mutex
	running bool
	jobs    chan job
	stopped chan struct{}
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	cleanup func()
}

func newApartment(cleanup func()) *apartment {
	return &apartment{jobs: make(chan job), cleanup: cleanup}
}

func (a *apartment) start(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.running {
		return errors.New("media service is already started")
	}
	wctx, cancel := context.WithCancel(ctx)
	ready := make(chan error, 1)
	stopped := make(chan struct{})
	a.wg.Add(1)
	go a.loop(wctx, ready, stopped)
	if err := <-ready; err != nil {
		cancel()
		<-stopped
		a.wg.Wait()
		return err
	}
	a.running = true
	a.cancel = cancel
	a.stopped = stopped
	return nil
}

func (a *apartment) loop(ctx context.Context, ready chan<- error, stopped chan struct{}) {
	defer a.wg.Done()
	defer close(stopped)
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := roInitialize(); err != nil {
		ready <- err
		return
	}
	defer roUninitialize()
	defer a.cleanup()
	ready <- nil
	for {
		select {
		case <-ctx.Done():
			return
		case j := <-a.jobs:
			jctx, cancel := context.WithCancel(ctx)
			stop := context.AfterFunc(j.ctx, cancel) //nolint:contextcheck // the job runs under the worker lifetime ctx and is also cancelled by its caller's ctx, which is by design not derived from the worker's (invariants 19, 21).
			val, err := j.fn(jctx)
			stop()
			cancel()
			j.res <- jobResult{val: val, err: err}
		}
	}
}

func (a *apartment) stop() error {
	a.mu.Lock()
	cancel := a.cancel
	stopped := a.stopped
	a.running = false
	a.cancel = nil
	a.mu.Unlock()
	if cancel == nil {
		return nil
	}
	cancel()
	<-stopped
	a.wg.Wait()
	return nil
}

func runOn[T any](ctx context.Context, a *apartment, fn func(context.Context) (T, error)) (T, error) {
	var zero T
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	a.mu.Lock()
	running, stopped := a.running, a.stopped
	a.mu.Unlock()
	if !running {
		return zero, ErrNotStarted
	}
	j := job{
		ctx: ctx,
		fn: func(ctx context.Context) (any, error) {
			return fn(ctx)
		},
		res: make(chan jobResult, 1),
	}
	select {
	case a.jobs <- j:
	case <-stopped:
		return zero, ErrNotStarted
	case <-ctx.Done():
		return zero, ctx.Err()
	}
	select {
	case r := <-j.res:
		if r.err != nil {
			return zero, r.err
		}
		v, ok := r.val.(T)
		if !ok {
			return zero, fmt.Errorf("unexpected result type %T", r.val)
		}
		return v, nil
	case <-ctx.Done():
		return zero, ctx.Err()
	}
}

type winrtPlatform struct {
	apt *apartment
	mgr object
}

func newPlatform() (platform, error) {
	if err := procRoUninitialize.Find(); err != nil {
		return nil, fmt.Errorf("load combase.dll: %w", err)
	}
	p := &winrtPlatform{}
	p.apt = newApartment(p.dropManager)
	return p, nil
}

func (p *winrtPlatform) supported() bool { return true }

func (p *winrtPlatform) start(ctx context.Context) error { return p.apt.start(ctx) }

func (p *winrtPlatform) stop() error { return p.apt.stop() }

type reading struct {
	track  Track
	active bool
}

func (p *winrtPlatform) current(ctx context.Context) (Track, bool, error) {
	r, err := runOn(ctx, p.apt, p.read)
	if err != nil {
		return Track{}, false, err
	}
	return r.track, r.active, nil
}

func (p *winrtPlatform) control(ctx context.Context, cmd command) error {
	_, err := runOn(ctx, p.apt, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, p.send(ctx, cmd)
	})
	return err
}

func (p *winrtPlatform) read(ctx context.Context) (reading, error) {
	var r reading
	found, err := p.withSession(ctx, func(sess object) error {
		track, err := readTrack(ctx, sess)
		r.track = track
		return err
	})
	if err != nil {
		return reading{}, err
	}
	r.active = found
	return r, nil
}

func (p *winrtPlatform) send(ctx context.Context, cmd command) error {
	slot, what, err := commandSlot(cmd)
	if err != nil {
		return err
	}
	found, err := p.withSession(ctx, func(sess object) error {
		op, err := sess.child(what, slot)
		if err != nil {
			return err
		}
		ok, err := awaitFlag(ctx, op)
		if err != nil {
			return fmt.Errorf("%s: %w", what, err)
		}
		if !ok {
			return fmt.Errorf("%s: %w", what, ErrRefused)
		}
		return nil
	})
	if err != nil {
		return err
	}
	if !found {
		return ErrNoSession
	}
	return nil
}

func commandSlot(cmd command) (uintptr, string, error) {
	switch cmd {
	case cmdTogglePlayPause:
		return slotTryTogglePlayPause, "TryTogglePlayPauseAsync", nil
	case cmdNext:
		return slotTrySkipNext, "TrySkipNextAsync", nil
	case cmdPrevious:
		return slotTrySkipPrevious, "TrySkipPreviousAsync", nil
	}
	return 0, "", fmt.Errorf("unknown media command %d", cmd)
}

func (p *winrtPlatform) dropManager() {
	p.mgr.release()
	p.mgr = object{}
}

func (p *winrtPlatform) manager(ctx context.Context) (object, error) {
	if p.mgr.p != nil {
		return p.mgr, nil
	}
	factory, err := activationFactory()
	if err != nil {
		return object{}, err
	}
	defer factory.release()
	op, err := factory.child("RequestAsync", slotRequestAsync)
	if err != nil {
		return object{}, err
	}
	mgr, err := awaitResult(ctx, op)
	if err != nil {
		return object{}, fmt.Errorf("RequestAsync: %w", err)
	}
	if mgr.p == nil {
		return object{}, errors.New("RequestAsync: no session manager returned")
	}
	p.mgr = mgr
	return mgr, nil
}

// A manager that fails is dropped so the next call asks the system for a new
// one; refusals and a cancelled ctx say nothing about the manager.
func (p *winrtPlatform) withSession(ctx context.Context, fn func(sess object) error) (bool, error) {
	mgr, err := p.manager(ctx)
	if err != nil {
		return false, err
	}
	sess, err := mgr.child("GetCurrentSession", slotGetCurrentSession)
	if err != nil {
		p.dropManager()
		return false, err
	}
	if sess.p == nil {
		return false, nil
	}
	defer sess.release()
	if err := fn(sess); err != nil {
		if !errors.Is(err, ErrRefused) && ctx.Err() == nil {
			p.dropManager()
		}
		return true, err
	}
	return true, nil
}

func readTrack(ctx context.Context, sess object) (Track, error) {
	var t Track
	app, err := sess.str("SourceAppUserModelId", slotSourceAppUserModelID)
	if err != nil {
		return Track{}, err
	}
	t.App = app

	op, err := sess.child("TryGetMediaPropertiesAsync", slotTryGetMediaProperties)
	if err != nil {
		return Track{}, err
	}
	props, err := awaitResult(ctx, op)
	if err != nil {
		return Track{}, fmt.Errorf("TryGetMediaPropertiesAsync: %w", err)
	}
	if props.p == nil {
		return Track{}, errors.New("TryGetMediaPropertiesAsync: no media properties returned")
	}
	defer props.release()
	if t.Title, err = props.str("Title", slotPropsTitle); err != nil {
		return Track{}, err
	}
	if t.Artist, err = props.str("Artist", slotPropsArtist); err != nil {
		return Track{}, err
	}
	if t.Album, err = props.str("AlbumTitle", slotPropsAlbumTitle); err != nil {
		return Track{}, err
	}

	info, err := sess.child("GetPlaybackInfo", slotGetPlaybackInfo)
	if err != nil {
		return Track{}, err
	}
	if info.p == nil {
		return Track{}, errors.New("GetPlaybackInfo: no playback info returned")
	}
	defer info.release()
	status, err := info.int32Value("PlaybackStatus", slotInfoPlaybackStatus)
	if err != nil {
		return Track{}, err
	}
	t.Playing = status == playbackStatusPlaying

	controls, err := info.child("Controls", slotInfoControls)
	if err != nil {
		return Track{}, err
	}
	if controls.p == nil {
		return Track{}, errors.New("playback controls: none returned")
	}
	defer controls.release()
	if t.CanPlayPause, err = controls.flag("IsPlayPauseToggleEnabled", slotControlsPlayPause); err != nil {
		return Track{}, err
	}
	if t.CanNext, err = controls.flag("IsNextEnabled", slotControlsNext); err != nil {
		return Track{}, err
	}
	if t.CanPrev, err = controls.flag("IsPreviousEnabled", slotControlsPrevious); err != nil {
		return Track{}, err
	}
	return t, nil
}
