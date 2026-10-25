package achievements

import (
	"context"
	"errors"
	"time"

	"typhon/internal/uierr"
)

const (
	EventName = "achievements:unlocked"

	SourceNone  = "none"
	SourceLocal = "local"

	SchemaUnavailable = "unavailable"
	SchemaNone        = "none"

	schemaTTL    = 24 * time.Hour
	storeVersion = 1
)

const (
	codeNoSteamID    = "achievements.no_steam_id"
	codeGameNotFound = "achievements.game_not_found"
	codeStoreBroken  = "achievements.store_broken"
)

var (
	ErrNoSteamID    = uierr.New(codeNoSteamID, "у игры нет Steam AppID, достижения для неё недоступны")
	ErrGameNotFound = uierr.New(codeGameNotFound, "игра не найдена в библиотеке")
	ErrStoreBroken  = uierr.New(codeStoreBroken, "файл достижений не читается, сохранение остановлено")
	ErrNoSchema     = errors.New("у игры нет списка достижений")
	ErrSchemaDown   = errors.New("список достижений сейчас недоступен")

	ErrBadJSON     = errors.New("файл прогресса достижений не является корректным JSON")
	ErrBadINI      = errors.New("файл прогресса достижений не является корректным INI")
	ErrFileTooBig  = errors.New("файл прогресса достижений больше допустимого размера")
	ErrNotProgress = errors.New("файл описывает достижения, а не прогресс")
)

type GameRef struct {
	InstallDir string
	AppID      string
}

// GameLookup resolves a library game id. The bool is false for an id the
// library does not hold; every other failure is the error.
type GameLookup interface {
	Lookup(gameID string) (GameRef, bool, error)
}

type SchemaItem struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Hidden      bool   `json:"hidden"`
	Icon        string `json:"icon"`
	IconGray    string `json:"iconGray"`
}

// SchemaSource returns the achievement list of an app. ErrNoSchema means the
// app has none; ErrSchemaDown means the answer is unknown for now.
type SchemaSource interface {
	Schema(ctx context.Context, appID, lang string) ([]SchemaItem, error)
}

// Unlock is a freshly earned achievement and the time the game recorded for it.
type Unlock struct {
	Key string
	At  time.Time
}

// Reporter takes the fresh unlocks of a game once they are announced. It
// decides by itself whether they are worth sending anywhere.
type Reporter interface {
	Report(gameID, appID string, unlocks []Unlock) error
}

type Config struct {
	ConfigDir string
	Reporter  Reporter
	Games     GameLookup
	Schema    SchemaSource
	Clock     func() time.Time
	Language  func() string
}

type Item struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Hidden      bool   `json:"hidden"`
	Icon        string `json:"icon"`
	IconGray    string `json:"iconGray"`
	Unlocked    bool   `json:"unlocked"`
	UnlockedAt  string `json:"unlockedAt"`
}

type View struct {
	AppID       string `json:"appId"`
	Total       int    `json:"total"`
	Unlocked    int    `json:"unlocked"`
	Items       []Item `json:"items"`
	Source      string `json:"source"`
	SchemaError string `json:"schemaError"`
	Partial     bool   `json:"partial"`
}

type Progress struct {
	GameID   string
	AppID    string
	Unlocked int
	NewKeys  []string
	Found    bool
	Partial  bool
}

type Event struct {
	GameID string   `json:"gameId"`
	Keys   []string `json:"keys"`
	Names  []string `json:"names"`
}
