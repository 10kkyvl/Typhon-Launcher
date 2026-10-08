package install

import "testing"

func sweepSnapshot(r *rig, id string) Installation {
	r.t.Helper()
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	return snapshotOf(r.s.findLocked(id))
}

func TestPartialSweepIsSkippedOnceTheServiceIsClosing(t *testing.T) {
	r := newRig(t)
	started, release := blockPartialRemoval(t)
	release()
	item, partial := partialRecord(r, "sweep2", StatusFailed)
	snap := sweepSnapshot(r, item.ID)
	r.shutdown()

	r.s.spawnSweep(r.s.ctx, snap)
	r.s.wg.Wait()

	select {
	case got := <-started:
		t.Fatalf("a sweep of %s started after the service began closing", got)
	default:
	}
	if !exists(partial) {
		t.Fatalf("%s was removed after the service began closing", partial)
	}
}

func TestPartialSweepStopsWhenTheServiceContextIsCancelled(t *testing.T) {
	r := newRig(t)
	started, release := blockPartialRemoval(t)
	release()
	item, partial := partialRecord(r, "sweep3", StatusFailed)
	snap := sweepSnapshot(r, item.ID)

	r.s.cancel()
	r.s.spawnSweep(r.s.ctx, snap)
	r.s.wg.Wait()

	select {
	case got := <-started:
		t.Fatalf("a sweep of %s started under a cancelled context", got)
	default:
	}
	if !exists(partial) {
		t.Fatalf("%s was removed under a cancelled context", partial)
	}
}
