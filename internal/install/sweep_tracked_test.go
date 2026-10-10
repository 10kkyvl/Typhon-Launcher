package install

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// blockPartialRemoval подменяет удаление .partial: оно сообщает о начале и стоит,
// пока тест его не отпустит, а затем удаляет по-настоящему.
func blockPartialRemoval(t *testing.T) (started <-chan string, release func()) {
	t.Helper()
	begun := make(chan string, 4)
	gate := make(chan struct{})
	previous := removePartialDir
	removePartialDir = func(path string) error {
		begun <- path
		<-gate
		return os.RemoveAll(path)
	}
	released := false
	release = func() {
		if !released {
			released = true
			close(gate)
		}
	}
	t.Cleanup(func() {
		release()
		removePartialDir = previous
	})
	return begun, release
}

func partialRecord(r *rig, id string, status Status) (Installation, string) {
	r.t.Helper()
	dest := filepath.Join(r.games, id)
	partial := dest + partialSuffix
	mkFile(r.t, filepath.Join(partial, "data.bin"), 64)
	item := Installation{ID: id, Name: id, Type: TypePortable, Status: status, Destination: dest, Mode: ModeCopy}
	r.add(item)
	return item, partial
}

// Подметание .partial после Cancel и Dismiss идёт в фоне, но принадлежит сервису:
// выключение дожидается его (инвариант 19), иначе процесс завершается посреди
// удаления каталога.
func TestShutdownWaitsForThePartialSweep(t *testing.T) {
	cases := []struct {
		name   string
		status Status
		do     func(r *rig, id string) error
	}{
		{"Dismiss of a failed install", StatusFailed, func(r *rig, id string) error { return r.s.Dismiss(id) }},
		{"Cancel of a pending install", StatusPending, func(r *rig, id string) error { return r.s.Cancel(id) }},
		{"cancellation confirmed by a resumed worker", StatusInstalling, func(r *rig, id string) error {
			r.s.cancelResumed(r.s.ctx, id)
			return nil
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newRig(t)
			started, release := blockPartialRemoval(t)
			item, partial := partialRecord(r, "sweep1", tc.status)

			if err := tc.do(r, item.ID); err != nil {
				t.Fatal(err)
			}
			select {
			case got := <-started:
				if got != partial {
					t.Fatalf("sweep removes %s, want %s", got, partial)
				}
			case <-time.After(15 * time.Second):
				t.Fatal("the partial sweep never started")
			}

			down := make(chan struct{})
			go func() {
				defer close(down)
				r.shutdown()
			}()
			select {
			case <-down:
				t.Fatal("ServiceShutdown returned while the partial sweep was still running")
			case <-time.After(150 * time.Millisecond):
			}
			release()
			select {
			case <-down:
			case <-time.After(15 * time.Second):
				t.Fatal("ServiceShutdown never returned after the sweep finished")
			}
			if exists(partial) {
				t.Fatalf("%s survived the sweep that shutdown waited for", partial)
			}
		})
	}
}
