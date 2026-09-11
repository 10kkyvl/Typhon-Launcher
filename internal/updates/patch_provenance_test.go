package updates

import (
	"testing"
	"typhon/internal/library"
	"typhon/internal/sources"
)

func TestIntermediatePatchCannotSupplyWholeInstallTorrent(t *testing.T) {
	r := patchRelease("p1", "1", "2", 100)
	r.InfoHash = "abcdef"
	s, err := newServiceAt(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	s.releases = &fakeReleases{list: []sources.Release{r}}
	if _, ok := s.torrentIdentity(library.Game{ReleaseID: "p1", SourceID: "src", DistributionID: "main", Version: "2"}); ok {
		t.Fatal("patch payload treated as whole installed game")
	}
}
