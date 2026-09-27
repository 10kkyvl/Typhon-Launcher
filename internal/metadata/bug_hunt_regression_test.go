package metadata

import (
	"context"
	"errors"
	"testing"
	"time"

	"typhon/internal/catalog"
)

func TestEnsureFreshHonorsDismissalWithEmptyMetadataLanguage(t *testing.T) {
	provider := &fakeProvider{}
	service, cat, _ := newTestService(t, provider)
	game, err := cat.AddGame(catalog.Game{Title: "Unresolved", MetadataLanguage: ""})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.DismissMatch(game.ID); err != nil {
		t.Fatal(err)
	}
	started, err := service.EnsureFresh(game.ID)
	if err != nil {
		t.Fatal(err)
	}
	if started {
		t.Fatal("dismissed game with empty metadata language started a search")
	}
	searches, gets := provider.counts()
	if searches != 0 || gets != 0 {
		t.Fatalf("provider calls after dismissal = searches:%d gets:%d", searches, gets)
	}
}

func TestApplyRejectsLocaleChangedDuringCatalogPersist(t *testing.T) {
	provider := &fakeProvider{}
	service, cat, _ := newTestService(t, provider)
	game, err := cat.AddGame(catalog.Game{Title: "Portal 2"})
	if err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	release := make(chan struct{})
	service.applyMetadata = func(id string, patch catalog.MetadataPatch) (catalog.Game, error) {
		close(started)
		<-release
		return cat.ApplyMetadata(id, patch)
	}
	done := make(chan error, 1)
	go func() {
		_, applyErr := service.apply(
			requestLanguage(context.Background(), "en"),
			game,
			GameMetadata{ProviderID: "steam:620", Title: "Portal 2", Summary: "English"},
			modeArt,
		)
		done <- applyErr
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("metadata apply did not reach catalog persistence")
	}
	service.SetLanguage("ru")
	close(release)
	select {
	case applyErr := <-done:
		if !errors.Is(applyErr, context.Canceled) {
			t.Fatalf("apply error = %v, want locale cancellation", applyErr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("metadata apply did not finish after locale change")
	}
}

func TestLanguageAndViewRemainResponsiveDuringCatalogPersist(t *testing.T) {
	provider := &fakeProvider{}
	service, cat, _ := newTestService(t, provider)
	game, err := cat.AddGame(catalog.Game{Title: "Portal 2"})
	if err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	release := make(chan struct{})
	service.applyMetadata = func(id string, patch catalog.MetadataPatch) (catalog.Game, error) {
		close(started)
		<-release
		return cat.ApplyMetadata(id, patch)
	}
	applyDone := make(chan error, 1)
	go func() {
		_, applyErr := service.apply(
			requestLanguage(context.Background(), "en"),
			game,
			GameMetadata{ProviderID: "steam:620", Title: "Portal 2", Summary: "English"},
			modeArt,
		)
		applyDone <- applyErr
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("metadata apply did not reach catalog persistence")
	}
	languageDone := make(chan struct{})
	go func() {
		service.SetLanguage("ru")
		close(languageDone)
	}()
	select {
	case <-languageDone:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("SetLanguage blocked during catalog persistence")
	}
	viewDone := make(chan error, 1)
	go func() {
		_, viewErr := service.GetView(game.ID)
		viewDone <- viewErr
	}()
	select {
	case viewErr := <-viewDone:
		if viewErr != nil {
			t.Fatal(viewErr)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("GetView blocked during catalog persistence")
	}
	close(release)
	select {
	case applyErr := <-applyDone:
		if !errors.Is(applyErr, context.Canceled) {
			t.Fatalf("apply error = %v, want locale cancellation", applyErr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("metadata apply did not finish after persistence release")
	}
}
