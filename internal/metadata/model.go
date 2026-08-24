package metadata

import "time"

type AssetType string

const (
	AssetCover      AssetType = "cover"
	AssetScreenshot AssetType = "screenshot"
)

type MediaAsset struct {
	ID        string    `json:"id"`
	GameID    string    `json:"gameId"`
	Type      AssetType `json:"type"`
	SourceURL string    `json:"sourceUrl"`
	Path      string    `json:"path"`
	URL       string    `json:"url,omitempty"`
	Width     int       `json:"width"`
	Height    int       `json:"height"`
	CreatedAt time.Time `json:"createdAt"`
}

type ImageRef struct {
	URL    string `json:"url"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

type Candidate struct {
	ProviderID  string  `json:"providerId"`
	Title       string  `json:"title"`
	ReleaseYear int     `json:"releaseYear,omitempty"`
	Developer   string  `json:"developer,omitempty"`
	ThumbURL    string  `json:"-"`
	Thumb       string  `json:"thumb,omitempty"`
	Confidence  float64 `json:"confidence"`
}

type GameMetadata struct {
	ProviderID  string     `json:"providerId"`
	Title       string     `json:"title"`
	Summary     string     `json:"summary"`
	ReleaseDate *time.Time `json:"releaseDate,omitempty"`
	Developer   string     `json:"developer"`
	Publisher   string     `json:"publisher"`
	Genres      []string   `json:"genres,omitempty"`
	Themes      []string   `json:"themes,omitempty"`
	Platforms   []string   `json:"platforms,omitempty"`
	Cover       *ImageRef  `json:"cover,omitempty"`
	Screenshots []ImageRef `json:"screenshots,omitempty"`
}
