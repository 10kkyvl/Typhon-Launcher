package download

import (
	"context"
	"testing"
	"time"
)

func withStallAfter(t *testing.T, d time.Duration) {
	t.Helper()
	previous := stallAfter
	stallAfter = d
	t.Cleanup(func() { stallAfter = previous })
}

func mustGet(t *testing.T, m *Manager, id string) Download {
	t.Helper()
	d, err := m.Get(id)
	if err != nil {
		t.Fatalf("get %s: %v", id, err)
	}
	return d
}

func TestSampleMarksStallAfterFrozenProgress(t *testing.T) {
	withStallAfter(t, 0)
	m := newTestManager(t, 2)
	m.addTestDownload("a")
	base := time.Now()

	m.sample(context.Background(), base)
	if d := mustGet(t, m, "a"); d.Stalled || d.StalledSince != nil {
		t.Fatalf("stalled on the first sample: %+v", d)
	}

	m.sample(context.Background(), base.Add(time.Millisecond))
	d := mustGet(t, m, "a")
	if !d.Stalled {
		t.Fatal("stalled not set after progress stopped growing past stallAfter")
	}
	if d.StalledSince == nil {
		t.Fatal("stalledSince not set")
	}
}

func TestSampleClearsStallOnceProgressResumes(t *testing.T) {
	withStallAfter(t, 0)
	m := newTestManager(t, 2)
	eng := m.addTestDownload("a")
	base := time.Now()

	m.sample(context.Background(), base)
	m.sample(context.Background(), base.Add(time.Millisecond))
	if d := mustGet(t, m, "a"); !d.Stalled {
		t.Fatal("precondition: download should be stalled before it resumes")
	}

	eng.mu.Lock()
	eng.done = 50
	eng.mu.Unlock()
	m.sample(context.Background(), base.Add(2*time.Millisecond))

	d := mustGet(t, m, "a")
	if d.Stalled {
		t.Fatal("stalled still set after progress resumed")
	}
	if d.StalledSince != nil {
		t.Fatal("stalledSince not cleared after progress resumed")
	}
}

func TestSampleDoesNotStallWithinTheGracePeriod(t *testing.T) {
	withStallAfter(t, time.Hour)
	m := newTestManager(t, 2)
	m.addTestDownload("a")
	base := time.Now()

	m.sample(context.Background(), base)
	m.sample(context.Background(), base.Add(time.Minute))

	if d := mustGet(t, m, "a"); d.Stalled {
		t.Fatal("stalled before stallAfter elapsed")
	}
}

func TestStallOnlyAppliesWhileDownloading(t *testing.T) {
	withStallAfter(t, 0)
	m := newTestManager(t, 2)
	eng := m.addTestDownload("a")
	eng.finish()
	eng.mu.Lock()
	eng.paths = []string{fakeFilePath(t, eng.size)}
	eng.mu.Unlock()

	m.sample(context.Background(), time.Now())
	waitUntil(t, "download to complete", func() bool { return m.statusOf(t, "a") == StatusCompleted })

	m.sample(context.Background(), time.Now().Add(time.Hour))
	if d := mustGet(t, m, "a"); d.Stalled {
		t.Fatal("a completed download must never be reported as stalled")
	}
}

func TestStalledStateIsNotPersisted(t *testing.T) {
	withStallAfter(t, 0)
	dir := t.TempDir()
	m := mustManagerAt(t, dir)
	m.max = 2
	m.addTestDownload("a")
	base := time.Now()
	m.sample(context.Background(), base)
	m.sample(context.Background(), base.Add(time.Millisecond))
	if d := mustGet(t, m, "a"); !d.Stalled {
		t.Fatal("precondition: download should be stalled")
	}

	reloaded := mustManagerAt(t, dir)
	reloaded.mu.Lock()
	if err := reloaded.loadLocked(); err != nil {
		t.Fatalf("reload: %v", err)
	}
	reloaded.mu.Unlock()

	items := reloaded.List()
	if len(items) != 1 {
		t.Fatalf("reloaded %d items, want 1", len(items))
	}
	if items[0].Stalled || items[0].StalledSince != nil {
		t.Fatalf("stalled state was persisted: %+v", items[0])
	}
}
