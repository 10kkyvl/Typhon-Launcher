package achievements

import (
	"context"
	"errors"
	"fmt"

	"typhon/internal/catalog"
	"typhon/internal/library"
	"typhon/internal/metadata"
	"typhon/internal/metadata/typhonapi"
)

type libraryPort interface {
	Find(id string) (library.Game, error)
}

type catalogPort interface {
	GetGames(ids []string) []catalog.Game
}

type lookup struct {
	lib libraryPort
	cat catalogPort
}

// NewLookup resolves a library game to its install directory and Steam AppID.
// An imported Steam game carries the id itself; any other game gets it from
// the catalog record it is bound to.
func NewLookup(lib libraryPort, cat catalogPort) (GameLookup, error) {
	if lib == nil || cat == nil {
		return nil, errors.New("achievements: library and catalog are required")
	}
	return lookup{lib: lib, cat: cat}, nil
}

func (l lookup) Lookup(gameID string) (GameRef, bool, error) {
	game, err := l.lib.Find(gameID)
	if library.IsGameNotFound(err) {
		return GameRef{}, false, nil
	}
	if err != nil {
		return GameRef{}, false, fmt.Errorf("library: %w", err)
	}
	ref := GameRef{InstallDir: game.InstallDir}
	switch {
	case game.External != nil && game.External.Provider == library.ProviderSteam && validAppID(game.External.AppID):
		ref.AppID = game.External.AppID
	case game.CanonicalGameID != "":
		found := l.cat.GetGames([]string{game.CanonicalGameID})
		if len(found) == 1 {
			ref.AppID = steamID(found[0])
		}
	}
	return ref, true, nil
}

func steamID(g catalog.Game) string {
	for _, id := range []string{g.ExternalIDs.Steam, g.LocalExternalIDs.Steam} {
		if validAppID(id) {
			return id
		}
	}
	return ""
}

type apiClient interface {
	Achievements(ctx context.Context, appID string) (typhonapi.AchievementSet, error)
}

type apiSchema struct {
	client apiClient
}

// NewSchemaSource adapts the Typhon API client to SchemaSource.
func NewSchemaSource(client apiClient) (SchemaSource, error) {
	if client == nil {
		return nil, errors.New("achievements: api client is required")
	}
	return apiSchema{client: client}, nil
}

func (a apiSchema) Schema(ctx context.Context, appID, lang string) ([]SchemaItem, error) {
	set, err := a.client.Achievements(metadata.WithLanguage(ctx, lang), appID)
	switch {
	case errors.Is(err, typhonapi.ErrNoAchievements):
		return nil, fmt.Errorf("%w: %w", ErrNoSchema, err)
	case errors.Is(err, typhonapi.ErrSteamUnavailable):
		return nil, fmt.Errorf("%w: %w", ErrSchemaDown, err)
	case err != nil:
		return nil, err
	}
	items := make([]SchemaItem, len(set.Items))
	for i, it := range set.Items {
		items[i] = SchemaItem{Key: it.Key, Name: it.Name, Description: it.Description, Hidden: it.Hidden, Icon: it.Icon, IconGray: it.IconGray}
	}
	return items, nil
}
