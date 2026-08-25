package catalog

import "time"

type ExternalIDs struct {
	Steam string `json:"steam,omitempty"`
	IGDB  string `json:"igdb,omitempty"`
	GOG   string `json:"gog,omitempty"`
}

func (e ExternalIDs) empty() bool {
	return e.Steam == "" && e.IGDB == "" && e.GOG == ""
}

type Game struct {
	ID                string      `json:"id"`
	Title             string      `json:"title"`
	SortTitle         string      `json:"sortTitle"`
	ReleaseYear       *int        `json:"releaseYear,omitempty"`
	ReleaseDate       *time.Time  `json:"releaseDate,omitempty"`
	Summary           string      `json:"summary,omitempty"`
	Developer         string      `json:"developer,omitempty"`
	Publisher         string      `json:"publisher,omitempty"`
	Genres            []string    `json:"genres,omitempty"`
	Themes            []string    `json:"themes,omitempty"`
	Platforms         []string    `json:"platforms,omitempty"`
	ExternalIDs       ExternalIDs `json:"externalIds"`
	Aliases           []string    `json:"aliases,omitempty"`
	CoverAssetID      string      `json:"coverAssetId,omitempty"`
	HeroAssetID       string      `json:"heroAssetId,omitempty"`
	MetadataUpdatedAt *time.Time  `json:"metadataUpdatedAt,omitempty"`
	MetadataPartial   bool        `json:"metadataPartial,omitempty"`
	Provisional       bool        `json:"provisional,omitempty"`
	CreatedAt         time.Time   `json:"createdAt"`
}

type MatchOverride struct {
	Pattern   string    `json:"pattern"`
	GameID    string    `json:"gameId"`
	CreatedAt time.Time `json:"createdAt"`
}

type Method string

const (
	MethodExternalID  Method = "external_id"
	MethodOverride    Method = "override"
	MethodExactTitle  Method = "exact_title"
	MethodAlias       Method = "alias"
	MethodFuzzy       Method = "fuzzy"
	MethodProvisional Method = "provisional"
	MethodNone        Method = "none"
)

type Status string

const (
	StatusMatched   Status = "matched"
	StatusReview    Status = "review"
	StatusUnmatched Status = "unmatched"
)

const (
	AutoThreshold   = 0.93
	ReviewThreshold = 0.75
	AmbiguityDelta  = 0.04
	MaxCandidates   = 6

	scoreExternalID = 1.0
	scoreOverride   = 1.0
	scoreExactTitle = 0.98
	scoreAlias      = 0.97
	scoreFuzzyCap   = 0.95
	yearBonus       = 0.03
	yearPenalty     = 0.08
	developerBonus  = 0.02

	candidatePool     = 48
	commonTokenShare  = 0.1
	minCommonTokenDoc = 200
)

type Query struct {
	Title       string      `json:"title"`
	Normalized  string      `json:"normalized"`
	Year        int         `json:"year"`
	Developer   string      `json:"developer"`
	ExternalIDs ExternalIDs `json:"externalIds"`
}

type Candidate struct {
	GameID string  `json:"gameId"`
	Title  string  `json:"title"`
	Year   *int    `json:"year,omitempty"`
	Score  float64 `json:"score"`
	Method Method  `json:"method"`
}

type Match struct {
	Status     Status      `json:"status"`
	GameID     string      `json:"gameId"`
	Confidence float64     `json:"confidence"`
	Method     Method      `json:"method"`
	Candidates []Candidate `json:"candidates"`
}
