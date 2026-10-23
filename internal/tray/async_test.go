package tray

import "testing"

func TestAsyncRunsAndWaitsForWork(t *testing.T) {
	var a Async
	started := make(chan struct{})
	release := make(chan struct{})
	finished := false
	if !a.Go(func() {
		close(started)
		<-release
		finished = true
	}) {
		t.Fatal("Go refused work before Close")
	}
	<-started
	closed := make(chan struct{})
	go func() {
		a.Close()
		close(closed)
	}()
	close(release)
	<-closed
	if !finished {
		t.Fatal("Close returned before the handler finished")
	}
}

func TestAsyncRefusesWorkAfterClose(t *testing.T) {
	var a Async
	a.Close()
	ran := false
	if a.Go(func() { ran = true }) {
		t.Fatal("Go accepted work after Close")
	}
	if ran {
		t.Fatal("handler ran after Close")
	}
}
