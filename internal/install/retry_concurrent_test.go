package install

import (
	"sync"
	"testing"
	"time"

	"typhon/internal/download"
)

// barrierDownloads держит каждого вызывающего Get, пока не соберутся все want:
// так все повторы обязаны пройти проверку статуса до того, как первый из них
// успеет изменить запись, и гонка воспроизводится при любом расписании потоков.
type barrierDownloads struct {
	downloadSource
	want int
	mu   sync.Mutex
	in   int
	open chan struct{}
}

func (b *barrierDownloads) Get(id string) (download.Download, error) {
	b.mu.Lock()
	b.in++
	if b.in == b.want {
		close(b.open)
	}
	b.mu.Unlock()
	select {
	case <-b.open:
	case <-time.After(15 * time.Second):
	}
	return b.downloadSource.Get(id)
}

func failedSilentInstall(t *testing.T, r *rig) Installation {
	t.Helper()
	dest := silentSource(t, r, rgInnoMarker)
	r.setRunner(newRgRunner(t, rgStep{code: 1}))
	item, err := r.s.Start("d1", StartOptions{Destination: dest})
	if err != nil {
		t.Fatal(err)
	}
	if got := r.settle(item.ID); got.Status != StatusFailed {
		t.Fatalf("status = %s (%q)", got.Status, got.Error)
	}
	return item
}

func TestConcurrentRetriesAdmitExactlyOne(t *testing.T) {
	cases := []struct {
		name  string
		retry func(s *Service, id string) error
	}{
		{"Retry", func(s *Service, id string) error { return s.Retry(id) }},
		{"RetryInteractive", func(s *Service, id string) error { return s.RetryInteractive(id) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newRig(t)
			item := failedSilentInstall(t, r)
			run := newRgRunner(t, rgStep{wait: true})
			r.setRunner(run)
			const callers = 8
			r.s.downloads = &barrierDownloads{downloadSource: r.downloads, want: callers, open: make(chan struct{})}

			errs := callTogether(callers, func(int) error { return tc.retry(r.s, item.ID) })

			accepted := countNil(errs)
			for _, err := range errs {
				if err != nil && errCode(err) != "install.unavailable" {
					t.Fatalf("a refused retry = %v, want install.unavailable", err)
				}
			}
			run.entered(0)
			waitFor(t, "every accepted retry to reach the installer", func() bool { return len(run.calls()) >= accepted })
			runs := len(run.calls())
			r.shutdown()
			if accepted != 1 || runs != 1 {
				t.Fatalf("%d of %d concurrent retries were accepted and the installer started %d times, want 1 and 1", accepted, len(errs), runs)
			}
		})
	}
}

func TestRetryRefusesAnInstallThatIsAlreadyRetrying(t *testing.T) {
	r := newRig(t)
	item := failedSilentInstall(t, r)
	run := newRgRunner(t, rgStep{wait: true})
	r.setRunner(run)

	if err := r.s.Retry(item.ID); err != nil {
		t.Fatalf("first Retry: %v", err)
	}
	run.entered(0)
	if err := r.s.Retry(item.ID); errCode(err) != "install.unavailable" {
		t.Fatalf("second Retry = %v, want install.unavailable", err)
	}
	if err := r.s.RetryInteractive(item.ID); errCode(err) != "install.unavailable" {
		t.Fatalf("RetryInteractive of a running install = %v, want install.unavailable", err)
	}
	if err := r.s.Cancel(item.ID); err != nil {
		t.Fatal(err)
	}
	r.settle(item.ID)
	if err := r.s.Retry(item.ID); err != nil {
		t.Fatalf("Retry of a cancelled install = %v, want it accepted", err)
	}
	run.entered(1)
	r.shutdown()
	if n := len(run.calls()); n != 2 {
		t.Fatalf("installer started %d times, want once per accepted Retry (2)", n)
	}
}
