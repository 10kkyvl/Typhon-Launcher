package achievementsync

import (
	"errors"
	"fmt"
	"time"
)

const (
	storeFile    = "achievement-outbox.json"
	storeVersion = 1

	maxEntries    = 500
	maxAge        = 7 * 24 * time.Hour
	maxFuture     = time.Hour
	maxBatch      = 100
	flushInterval = 10 * time.Minute
)

var (
	ErrNotSignedIn  = errors.New("achievementsync: not signed in")
	ErrOutboxBroken = errors.New("achievementsync: outbox file is unreadable, saving is stopped")
)

// PermanentError is a rejection that sending the same data again cannot fix.
type PermanentError struct {
	Status int
	Code   string
}

func (e *PermanentError) Error() string {
	return fmt.Sprintf("achievementsync: rejected with status %d (%s)", e.Status, e.Code)
}

// RetryableError is a failure of the server or the path to it; the data is
// worth sending again later.
type RetryableError struct {
	Status int
	Code   string
	cause  error
}

func (e *RetryableError) Error() string {
	if e.cause != nil {
		return fmt.Sprintf("achievementsync: request failed: %v", e.cause)
	}
	return fmt.Sprintf("achievementsync: server answered %d (%s)", e.Status, e.Code)
}

func (e *RetryableError) Unwrap() error { return e.cause }

type Unlock struct {
	Key        string    `json:"key"`
	UnlockedAt time.Time `json:"unlockedAt"`
}

type request struct {
	IGDBID     int64    `json:"igdbId"`
	SteamAppID int64    `json:"steamAppId"`
	Unlocks    []Unlock `json:"unlocks"`
}

type entry struct {
	Owner      string    `json:"owner"`
	IGDBID     int64     `json:"igdbId"`
	SteamAppID int64     `json:"steamAppId"`
	Key        string    `json:"key"`
	UnlockedAt time.Time `json:"unlockedAt"`
}

type identity struct {
	owner string
	igdb  int64
	key   string
}

func (e entry) id() identity { return identity{owner: e.Owner, igdb: e.IGDBID, key: e.Key} }

type state struct {
	Entries []entry `json:"entries"`
}
