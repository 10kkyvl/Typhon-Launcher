package catalog

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"typhon/internal/storage"
)

func TestValidStoreLink(t *testing.T) {
	long := "https://store.steampowered.com/app/1/" + strings.Repeat("a", 512)
	cases := []struct {
		name, store, raw string
		ok               bool
	}{
		{"steam", "steam", "https://store.steampowered.com/app/105600", true},
		{"gog www", "gog", "https://www.gog.com/en/game/prey", true},
		{"gog bare", "gog", "https://gog.com/game/prey", true},
		{"epic", "epic", "https://store.epicgames.com/p/prey", true},
		{"epic www", "epic", "https://www.epicgames.com/store/p/prey", true},
		{"http", "steam", "http://store.steampowered.com/app/1", false},
		{"userinfo", "steam", "https://user:pw@store.steampowered.com/app/1", false},
		{"foreign host", "steam", "https://evil.example/app/1", false},
		{"suffix host", "steam", "https://store.steampowered.com.evil.example/app/1", false},
		{"host of other store", "steam", "https://www.gog.com/game/prey", false},
		{"port", "gog", "https://gog.com:8443/game/prey", false},
		{"unknown store", "itch", "https://store.steampowered.com/app/1", false},
		{"too long", "steam", long, false},
		{"empty", "steam", "", false},
		{"javascript", "steam", "javascript:alert(1)", false},
		{"padded", "steam", " https://store.steampowered.com/app/1", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, ok := ValidStoreLink(tc.store, tc.raw)
			if ok != tc.ok {
				t.Fatalf("ValidStoreLink(%q, %q) = %v, want %v", tc.store, tc.raw, ok, tc.ok)
			}
		})
	}
}

func TestApplyMetadataStoreLinks(t *testing.T) {
	svc, _ := metadataService(t)
	game, err := svc.AddGame(Game{Title: "Prey"})
	if err != nil {
		t.Fatal(err)
	}
	patch := samplePatch()
	patch.StoreLinks = map[string]string{
		"steam": "https://store.steampowered.com/app/480490",
		"gog":   "https://evil.example/x",
		"itch":  "https://store.steampowered.com/app/1",
	}
	got, err := svc.ApplyMetadata(game.ID, patch)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"steam": "https://store.steampowered.com/app/480490"}
	if !reflect.DeepEqual(got.StoreLinks, want) {
		t.Fatalf("store links = %v, want %v", got.StoreLinks, want)
	}

	patch.StoreLinks = map[string]string{"epic": "https://store.epicgames.com/p/prey"}
	got, err = svc.ApplyMetadata(game.ID, patch)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.StoreLinks, patch.StoreLinks) {
		t.Fatalf("after update store links = %v", got.StoreLinks)
	}

	patch.StoreLinks = nil
	got, err = svc.ApplyMetadata(game.ID, patch)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.StoreLinks) != 1 {
		t.Fatalf("absent field must keep links, got %v", got.StoreLinks)
	}

	patch.StoreLinks = map[string]string{}
	got, err = svc.ApplyMetadata(game.ID, patch)
	if err != nil {
		t.Fatal(err)
	}
	if got.StoreLinks != nil {
		t.Fatalf("empty object must clear links, got %v", got.StoreLinks)
	}
}

func TestLoadCatalogWithoutStoreLinks(t *testing.T) {
	dir := t.TempDir()
	if err := storage.Save(filepath.Join(dir, "catalog.json"), gamesVersion, []Game{{ID: "old", Title: "Old"}}); err != nil {
		t.Fatal(err)
	}
	svc, err := NewServiceAt(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, err := svc.GetGame("old")
	if err != nil {
		t.Fatal(err)
	}
	if got.StoreLinks != nil {
		t.Fatalf("store links = %v, want nil", got.StoreLinks)
	}
}
