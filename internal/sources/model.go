package sources

import (
	"time"

	"typhon/internal/catalog"
)

type Type string

const (
	TypeURL  Type = "url"
	TypeFile Type = "file"
)

type Status string

const (
	StatusActive   Status = "active"
	StatusDisabled Status = "disabled"
	StatusError    Status = "error"
	StatusUpdating Status = "updating"
)

type Health string

const (
	HealthHealthy Health = "healthy"
	HealthWarning Health = "warning"
	HealthError   Health = "error"
)

type Availability string

const (
	AvailabilityAvailable Availability = "available"
	AvailabilityRemoved   Availability = "removed"
)

type Source struct {
	ID            string     `json:"id"`
	Name          string     `json:"name"`
	Type          Type       `json:"type"`
	URL           string     `json:"url"`
	Path          string     `json:"path,omitempty"`
	Enabled       bool       `json:"enabled"`
	Status        Status     `json:"status"`
	Health        Health     `json:"health"`
	LastUpdatedAt *time.Time `json:"lastUpdatedAt"`
	LastError     string     `json:"lastError"`
	Fingerprint   string     `json:"fingerprint"`
	ETag          string     `json:"etag,omitempty"`
	LastModified  string     `json:"lastModified,omitempty"`
	FeedVersion   int        `json:"feedVersion"`
	Entries       int        `json:"entries"`
	Invalid       int        `json:"invalid"`
	Matched       int        `json:"matched"`
	Review        int        `json:"review"`
	Unmatched     int        `json:"unmatched"`
	Warnings      []string   `json:"warnings,omitempty"`
	CreatedAt     time.Time  `json:"createdAt"`
}

type Kind string

const (
	KindRelease Kind = "release"
	KindPatch   Kind = "patch"
)

type Release struct {
	ID              string         `json:"id"`
	SourceID        string         `json:"sourceId"`
	Kind            Kind           `json:"kind,omitempty"`
	RawTitle        string         `json:"rawTitle"`
	Title           string         `json:"title"`
	NormalizedTitle string         `json:"normalizedTitle"`
	CanonicalGameID *string        `json:"canonicalGameId"`
	Version         string         `json:"version"`
	RawVersion      string         `json:"rawVersion,omitempty"`
	FromVersion     string         `json:"fromVersion,omitempty"`
	ToVersion       string         `json:"toVersion,omitempty"`
	Sequence        int            `json:"sequence,omitempty"`
	Edition         string         `json:"edition,omitempty"`
	Languages       []string       `json:"languages,omitempty"`
	Year            int            `json:"year,omitempty"`
	Tags            []string       `json:"tags,omitempty"`
	Repacker        string         `json:"repacker,omitempty"`
	DLCCount        int            `json:"dlcCount,omitempty"`
	Size            int64          `json:"size"`
	SizeUnknown     bool           `json:"sizeUnknown,omitempty"`
	UploadedAt      *time.Time     `json:"uploadedAt"`
	URIs            []string       `json:"uris"`
	InfoHash        string         `json:"infoHash,omitempty"`
	MatchStatus     catalog.Status `json:"matchStatus"`
	MatchConfidence float64        `json:"matchConfidence"`
	MatchMethod     string         `json:"matchMethod"`
	Availability    Availability   `json:"availability"`
	Locked          bool           `json:"locked,omitempty"`
	Ignored         bool           `json:"ignored,omitempty"`
	New             bool           `json:"new,omitempty"`
	FirstSeenAt     time.Time      `json:"firstSeenAt"`
	LastSeenAt      time.Time      `json:"lastSeenAt"`
	CreatedAt       time.Time      `json:"createdAt"`
}

func (r *Release) identity() string {
	if r.InfoHash != "" {
		return "hash:" + r.InfoHash
	}
	if r.Kind == KindPatch {
		return "patch:" + r.NormalizedTitle + "|" + r.FromVersion + "|" + r.ToVersion
	}
	return "title:" + r.NormalizedTitle + "|" + r.RawTitle
}

type SourceRef struct {
	ReleaseID  string `json:"releaseId"`
	SourceID   string `json:"sourceId"`
	SourceName string `json:"sourceName"`
}

type ReleaseView struct {
	Release    Release `json:"release"`
	SourceName string  `json:"sourceName"`
	GameTitle  string  `json:"gameTitle"`
}

type ReleaseGroup struct {
	Release    Release     `json:"release"`
	SourceName string      `json:"sourceName"`
	Duplicates []SourceRef `json:"duplicates,omitempty"`
}

type GameReleaseInfo struct {
	Title         string
	Releases      int
	Sources       int
	LatestVersion string
}

type ReleaseMatches struct {
	Games         map[string]GameReleaseInfo
	Unmatched     []ReleaseView
	MoreUnmatched int
}

type ReleaseQuery struct {
	SourceID string `json:"sourceId"`
	Search   string `json:"search"`
	Status   string `json:"status"`
	Sort     string `json:"sort"`
	Page     int    `json:"page"`
	PageSize int    `json:"pageSize"`
}

type ReleasePage struct {
	Items    []ReleaseView `json:"items"`
	Total    int           `json:"total"`
	Page     int           `json:"page"`
	PageSize int           `json:"pageSize"`
}

type Preview struct {
	Name        string   `json:"name"`
	Type        Type     `json:"type"`
	URL         string   `json:"url"`
	Path        string   `json:"path,omitempty"`
	FeedVersion int      `json:"feedVersion"`
	Entries     int      `json:"entries"`
	Invalid     int      `json:"invalid"`
	Warnings    []string `json:"warnings,omitempty"`
	Fingerprint string   `json:"fingerprint"`
	Duplicate   bool     `json:"duplicate"`
}

type Summary struct {
	SourceID    string `json:"sourceId"`
	Name        string `json:"name"`
	Entries     int    `json:"entries"`
	Invalid     int    `json:"invalid"`
	Added       int    `json:"added"`
	Updated     int    `json:"updated"`
	Removed     int    `json:"removed"`
	Restored    int    `json:"restored"`
	Matched     int    `json:"matched"`
	Review      int    `json:"review"`
	Unmatched   int    `json:"unmatched"`
	New         int    `json:"new"`
	NotModified bool   `json:"notModified"`
	DurationMs  int64  `json:"durationMs"`
	Error       string `json:"error,omitempty"`
}

type Details struct {
	Source    Source `json:"source"`
	Total     int    `json:"total"`
	Available int    `json:"available"`
	Removed   int    `json:"removed"`
	New       int    `json:"new"`
	Ignored   int    `json:"ignored"`
}

type SourceError struct {
	SourceID  string `json:"sourceId"`
	Name      string `json:"name"`
	Message   string `json:"message"`
	Scheduled bool   `json:"scheduled"`
}

type ReleaseBatch struct {
	SourceID string `json:"sourceId"`
	Count    int    `json:"count"`
}

type DownloadRequest struct {
	URI       string `json:"uri"`
	Name      string `json:"name"`
	ReleaseID string `json:"releaseId"`
	SourceID  string `json:"sourceId"`
	GameID    string `json:"gameId"`
	Version   string `json:"version"`
}
