package main

import (
	"context"
	"errors"

	"typhon/internal/accountsync"
	"typhon/internal/catalog"
	"typhon/internal/library"
	"typhon/internal/metadata"
	"typhon/internal/settings"
)

type syncSettings struct{ svc *settings.Service }

func (a syncSettings) Get() settings.Settings { return a.svc.GetSettings() }

func (a syncSettings) Save(next settings.Settings) error { return a.svc.SaveSettings(next) }

type syncLibrary struct{ svc *library.Service }

func (a syncLibrary) Snapshot() ([]accountsync.Game, error) {
	items := a.svc.SyncSnapshot()
	out := make([]accountsync.Game, 0, len(items))
	for _, item := range items {
		out = append(out, accountsync.Game{
			CanonicalGameID: item.CanonicalGameID,
			PlaytimeSeconds: item.PlaytimeSeconds,
			LastPlayed:      item.LastPlayed,
			Owned:           item.Owned,
			Favorite:        item.Favorite,
			Status:          item.Status,
			StatusAt:        item.StatusAt,
		})
	}
	return out, nil
}

func (a syncLibrary) Apply(items []accountsync.Game) error {
	merged := make([]library.SyncGame, 0, len(items))
	for _, item := range items {
		merged = append(merged, library.SyncGame{
			CanonicalGameID: item.CanonicalGameID,
			PlaytimeSeconds: item.PlaytimeSeconds,
			LastPlayed:      item.LastPlayed,
			Owned:           item.Owned,
			Favorite:        item.Favorite,
			Status:          item.Status,
			StatusAt:        item.StatusAt,
		})
	}
	return a.svc.ApplySync(merged)
}

func (a syncLibrary) Add(canonicalGameID, title string) error {
	_, err := a.svc.AddCatalogGame(canonicalGameID, title, "")
	return err
}

type syncCatalog struct{ svc *catalog.Service }

func (a syncCatalog) IGDBIDOf(canonicalGameID string) string {
	return a.svc.IGDBIDOf(canonicalGameID)
}

func (a syncCatalog) EnsureByIGDB(igdbID, title string) (string, error) {
	game, err := a.svc.EnsureByIGDB(igdbID, title)
	if err != nil {
		return "", err
	}
	return game.ID, nil
}

var errNoMetadataProvider = errors.New("metadata provider unavailable")

type syncMetadata struct{ provider metadata.Provider }

func (a syncMetadata) Title(ctx context.Context, igdbID string) (string, error) {
	if a.provider == nil {
		return "", errNoMetadataProvider
	}
	meta, err := a.provider.Get(ctx, igdbID)
	if err != nil {
		return "", err
	}
	return meta.Title, nil
}
