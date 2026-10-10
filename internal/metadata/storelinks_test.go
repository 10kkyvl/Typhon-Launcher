package metadata

import (
	"path/filepath"
	"reflect"
	"testing"

	"typhon/internal/catalog"
	"typhon/internal/storage"
	"typhon/internal/uierr"
)

func serviceWithGame(t *testing.T, game catalog.Game) *Service {
	t.Helper()
	dir := t.TempDir()
	if err := storage.Save(filepath.Join(dir, "catalog.json"), 1, []catalog.Game{game}); err != nil {
		t.Fatal(err)
	}
	cat, err := catalog.NewServiceAt(dir)
	if err != nil {
		t.Fatal(err)
	}
	svc, err := NewServiceAt(dir, cat, nil)
	if err != nil {
		t.Fatal(err)
	}
	return svc
}

func TestGetViewStoreLinksDropsInvalidStored(t *testing.T) {
	svc := serviceWithGame(t, catalog.Game{ID: "g", Title: "G", StoreLinks: map[string]string{
		"steam": "https://store.steampowered.com/app/1",
		"gog":   "https://evil.example/x",
	}})
	view, err := svc.GetView("g")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"steam": "https://store.steampowered.com/app/1"}
	if !reflect.DeepEqual(view.StoreLinks, want) {
		t.Fatalf("view store links = %v, want %v", view.StoreLinks, want)
	}
}

func TestOpenStoreLink(t *testing.T) {
	svc := serviceWithGame(t, catalog.Game{ID: "g", Title: "G", StoreLinks: map[string]string{
		"steam": "https://store.steampowered.com/app/1",
		"gog":   "http://www.gog.com/game/g",
		"epic":  "https://evil.example/p",
	}})
	var opened []string
	svc.openURL = func(u string) error { opened = append(opened, u); return nil }

	cases := []struct {
		name, game, store, code string
		opened                  string
	}{
		{"valid", "g", "steam", "", "https://store.steampowered.com/app/1"},
		{"missing key", "g", "itch", "metadata.store_link_missing", ""},
		{"insecure stored", "g", "gog", "metadata.store_link_invalid", ""},
		{"foreign host stored", "g", "epic", "metadata.store_link_invalid", ""},
		{"no game id", " ", "steam", "metadata.no_game_id", ""},
		{"unknown game", "nope", "steam", "catalog.game_not_found", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opened = nil
			err := svc.OpenStoreLink(tc.game, tc.store)
			if tc.code == "" {
				if err != nil {
					t.Fatal(err)
				}
				if len(opened) != 1 || opened[0] != tc.opened {
					t.Fatalf("opened = %v, want %q", opened, tc.opened)
				}
				return
			}
			if uierr.Code(err) != tc.code {
				t.Fatalf("err = %v, want code %s", err, tc.code)
			}
			if len(opened) != 0 {
				t.Fatalf("browser was opened: %v", opened)
			}
		})
	}
}
