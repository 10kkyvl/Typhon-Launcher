package typhonapi

import (
	"net/http"
	"sync/atomic"
	"testing"
	"typhon/internal/catalog"
)

func TestBrowseHTTPPreservesNewProviderLinkAndDeveloper(t *testing.T) {
	var linked atomic.Bool
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/catalog/games" {
			t.Errorf("path=%s", r.URL.Path)
		}
		if linked.Load() {
			writeJSON(t, w, 200, `{"items":[{"id":"canonical","title":"0xFF","developer":"Sandstrom","externalIds":{"igdb":"242303","steam":"2218760"},"providerLinks":{"igdb":["242303"],"steam":["2218760"]}}],"total":1,"providers":[{"provider":"igdb","complete":false},{"provider":"steam","complete":false}]}`)
		} else {
			writeJSON(t, w, 200, `{"items":[{"id":"canonical","title":"0xFF","externalIds":{"igdb":"242303"},"providerLinks":{"igdb":["242303"]}},{"id":"steam-home","title":"0xFF","externalIds":{"steam":"2218760"},"providerLinks":{"steam":["2218760"]}}],"total":2,"providers":[{"provider":"igdb","complete":false},{"provider":"steam","complete":false}]}`)
		}
	}), nil)
	dir := t.TempDir()
	svc, err := catalog.NewServiceAt(dir)
	if err != nil {
		t.Fatal(err)
	}
	svc.SetRemoteCatalog(client)
	first, err := svc.BrowseGames(catalog.GameQuery{})
	if err != nil || len(first.Items) != 2 {
		t.Fatalf("first=%+v err=%v", first, err)
	}
	linked.Store(true)
	page, err := svc.BrowseGames(catalog.GameQuery{})
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("linked=%+v err=%v", page, err)
	}
	g := page.Items[0]
	if g.Developer != "Sandstrom" || g.ExternalIDs.Steam != "2218760" || len(g.AliasIDs) != 1 || g.AliasIDs[0] != "steam-home" {
		t.Fatalf("HTTP -> service -> UI row=%+v", g)
	}
	restarted, err := catalog.NewServiceAt(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !restarted.SameGame("canonical", "steam-home") {
		t.Fatal("personal reference lost after restart")
	}
}
