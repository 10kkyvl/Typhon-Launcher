package metadata

import (
	"context"
	"errors"
	"testing"
	"typhon/internal/catalog"
)

func TestLanguageSwitchInvalidatesCacheAndRejectsOldResponse(t *testing.T) {
	p := &fakeProvider{meta: map[string]GameMetadata{"steam:620": {ProviderID: "steam:620", Title: "Portal 2", Summary: "English"}}}
	s, cat, _ := newTestService(t, p)
	game := addGame(t, cat, catalog.Game{Title: "Portal 2"})
	en, e := s.ApplyMatch(game.ID, "steam:620")
	if e != nil || en.Stale {
		t.Fatalf("EN %+v %v", en, e)
	}
	s.SetLanguage("ru")
	view, e := s.GetView(game.ID)
	if e != nil || !view.Stale {
		t.Fatal("locale change did not invalidate cache", e)
	}
	_, e = s.apply(requestLanguage(context.Background(), "en"), view.Game, GameMetadata{ProviderID: "steam:620", Title: "Portal 2", Summary: "Late English"}, modeArt)
	if !errors.Is(e, context.Canceled) {
		t.Fatal("old response accepted", e)
	}
	ru, e := s.apply(requestLanguage(context.Background(), "ru"), view.Game, GameMetadata{ProviderID: "steam:620", Title: "Portal 2", Summary: "Русское описание"}, modeArt)
	if e != nil || ru.Stale || ru.Game.MetadataLanguage != "ru" || ru.Game.Summary != "Русское описание" {
		t.Fatalf("RU %+v %v", ru, e)
	}
	s.SetLanguage("en")
	view, e = s.GetView(game.ID)
	if e != nil {
		t.Fatal(e)
	}
	if !view.Stale {
		t.Fatal("switch back stayed fresh")
	}
}
