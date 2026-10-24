package updates

import (
	"sync"
	"testing"
	"time"

	"typhon/internal/sources"
)

type gatedReleases struct {
	*fakeReleases
	once    sync.Once
	entered chan struct{}
	proceed chan struct{}
}

func (g *gatedReleases) ReleasesFor(canonicalID, title string) []sources.Release {
	list := g.fakeReleases.ReleasesFor(canonicalID, title)
	g.once.Do(func() {
		close(g.entered)
		<-g.proceed
	})
	return list
}

func TestConcurrentChecksDoNotPublishAStaleResult(t *testing.T) {
	h := newHarness(t)
	gate := &gatedReleases{fakeReleases: h.releases, entered: make(chan struct{}), proceed: make(chan struct{})}
	h.service.releases = gate
	game := h.library.games[0]

	stale := make(chan error, 1)
	go func() { stale <- h.service.check(game) }()
	<-gate.entered

	h.releases.edit(func(list []sources.Release) []sources.Release { return append(list, release("r3", "1.2", 13<<20)) })
	fresh := make(chan error, 1)
	go func() { fresh <- h.service.check(game) }()

	select {
	case err := <-fresh:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(100 * time.Millisecond):
	}
	close(gate.proceed)
	if err := <-stale; err != nil {
		t.Fatal(err)
	}
	if err := <-fresh; err != nil {
		t.Fatal(err)
	}

	got, ok := h.service.snapshot("local-1")
	if !ok {
		t.Fatal("no update entry after the checks")
	}
	if got.Availability.TargetVersion != "1.2" || got.State != StateAvailable {
		t.Fatalf("a check that read the old list overwrote the newer one: %+v", got.Availability)
	}
}
