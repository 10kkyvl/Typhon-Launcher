package accountsync

import (
	"context"
	"time"

	"typhon/internal/settings"
)

type Game struct {
	IGDBID          string
	CanonicalGameID string
	PlaytimeSeconds int64
	LastPlayed      *time.Time
	Owned           bool
	Favorite        bool
	FavoriteAt      *time.Time
	Status          string
	StatusAt        *time.Time
}

type LibraryPort interface {
	Snapshot() ([]Game, error)
	Apply(items []Game) error
	Add(canonicalGameID, title string) error
}

type CatalogPort interface {
	IGDBIDOf(canonicalGameID string) string
	EnsureByIGDB(igdbID, title string) (string, error)
}

type MetadataPort interface {
	Title(ctx context.Context, igdbID string) (string, error)
}

type SettingsPort interface {
	Get() settings.Settings
	Save(settings.Settings) error
}
