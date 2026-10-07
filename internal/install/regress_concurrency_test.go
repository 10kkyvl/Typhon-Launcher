package install

import (
	"fmt"
	"path/filepath"
	"sync"
	"testing"
)

// callTogether запускает n вызовов одновременно, по сигналу, и возвращает их ошибки.
func callTogether(n int, call func(i int) error) []error {
	errs := make([]error, n)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errs[i] = call(i)
		}()
	}
	close(start)
	wg.Wait()
	return errs
}

func countNil(errs []error) int {
	n := 0
	for _, err := range errs {
		if err == nil {
			n++
		}
	}
	return n
}

// Двойной клик по «Установить» приходит в сервис как два одновременных вызова:
// проверка «занято» и запуск обязаны быть одним действием (инвариант 17).
func TestConcurrentStartsOfOneDownloadAdmitExactlyOne(t *testing.T) {
	r := newRig(t)
	silentSource(t, r, rgInnoMarker)
	run := newRgRunner(t, rgStep{wait: true})
	r.setRunner(run)

	errs := callTogether(8, func(i int) error {
		_, err := r.s.Start("d1", StartOptions{Destination: filepath.Join(r.games, fmt.Sprintf("Game %d", i))})
		return err
	})

	if got := countNil(errs); got != 1 {
		t.Fatalf("%d of %d concurrent Start calls were accepted, want exactly 1: %v", got, len(errs), errs)
	}
	for _, err := range errs {
		if err != nil && errCode(err) != "install.busy" {
			t.Fatalf("a refused Start = %v, want install.busy", err)
		}
	}
	run.entered(0)
	if got := r.s.List(); len(got) != 1 {
		t.Fatalf("records = %d, want 1", len(got))
	}
	if n := len(run.calls()); n != 1 {
		t.Fatalf("the installer started %d times", n)
	}
	id := r.s.List()[0].ID
	if err := r.s.Cancel(id); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	r.settle(id)
}

func TestConcurrentStartsIntoOneFolderAdmitExactlyOne(t *testing.T) {
	r := newRig(t)
	const downloads = 6
	for i := 0; i < downloads; i++ {
		root := t.TempDir()
		mkInstaller(t, filepath.Join(root, "Game", "setup.exe"), rgInnoMarker)
		r.download(fmt.Sprintf("d%d", i), "Game", root)
	}
	dest := filepath.Join(r.games, "Game")
	run := newRgRunner(t, rgStep{wait: true})
	r.setRunner(run)

	errs := callTogether(downloads, func(i int) error {
		_, err := r.s.Start(fmt.Sprintf("d%d", i), StartOptions{Destination: dest})
		return err
	})

	if got := countNil(errs); got != 1 {
		t.Fatalf("%d of %d concurrent Start calls into one folder were accepted, want exactly 1: %v", got, len(errs), errs)
	}
	for _, err := range errs {
		if code := errCode(err); err != nil && code != "install.busy" && code != "install.dest_not_empty" {
			t.Fatalf("a refused Start = %v", err)
		}
	}
	run.entered(0)
	id := r.s.List()[0].ID
	if err := r.s.Cancel(id); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	r.settle(id)
}

func TestConcurrentCancelsStopOneInstallOnce(t *testing.T) {
	r := newRig(t)
	log := r.finished()
	dest := silentSource(t, r, rgInnoMarker)
	run := newRgRunner(t, rgStep{wait: true})
	r.setRunner(run)
	item, err := r.s.Start("d1", StartOptions{Destination: dest})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	run.entered(0)

	errs := callTogether(8, func(int) error { return r.s.Cancel(item.ID) })

	for _, err := range errs {
		if err != nil && errCode(err) != "install.unavailable" {
			t.Fatalf("Cancel = %v, want success or install.unavailable", err)
		}
	}
	if got := r.settle(item.ID); got.Status != StatusCancelled || got.Error != "" {
		t.Fatalf("status %s error %q", got.Status, got.Error)
	}
	if first := log.next(t); first.Status != StatusCancelled {
		t.Fatalf("onFinished status = %s", first.Status)
	}
	r.shutdown()
	if rest := log.drain(); len(rest) != 0 {
		t.Fatalf("a cancelled install was reported finished again: %+v", rest)
	}
}

func TestInstallsOfDifferentGamesRunSideBySide(t *testing.T) {
	r := newRig(t)
	for _, id := range []string{"d1", "d2"} {
		root := t.TempDir()
		mkInstaller(t, filepath.Join(root, id, "setup.exe"), rgInnoMarker)
		r.download(id, id, root)
	}
	run := newRgRunner(t, rgStep{wait: true})
	r.setRunner(run)

	first, err := r.s.Start("d1", StartOptions{Destination: filepath.Join(r.games, "One")})
	if err != nil {
		t.Fatalf("Start d1: %v", err)
	}
	second, err := r.s.Start("d2", StartOptions{Destination: filepath.Join(r.games, "Two")})
	if err != nil {
		t.Fatalf("Start d2: %v", err)
	}
	run.entered(1)
	for _, id := range []string{first.ID, second.ID} {
		if got := r.get(id); !transient(got.Status) {
			t.Fatalf("install %s is %s while both installers run", id, got.Status)
		}
	}
	for _, id := range []string{first.ID, second.ID} {
		if err := r.s.Cancel(id); err != nil {
			t.Fatalf("Cancel %s: %v", id, err)
		}
		if got := r.settle(id); got.Status != StatusCancelled {
			t.Fatalf("status of %s = %s", id, got.Status)
		}
	}
}
