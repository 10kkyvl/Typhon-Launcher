package typhonapi

import "time"

type searchResponse struct {
	Candidates []candidatePayload `json:"candidates"`
}

type candidatePayload struct {
	ProviderID  string `json:"providerId"`
	Title       string `json:"title"`
	ReleaseYear int    `json:"releaseYear"`
	Developer   string `json:"developer"`
	ThumbURL    string `json:"thumbUrl"`
}

type imagePayload struct {
	URL    string `json:"url"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

type gameResponse struct {
	ProviderID  string         `json:"providerId"`
	Title       string         `json:"title"`
	Summary     string         `json:"summary"`
	ReleaseDate *time.Time     `json:"releaseDate"`
	Developer   string         `json:"developer"`
	Publisher   string         `json:"publisher"`
	Genres      []string       `json:"genres"`
	Themes      []string       `json:"themes"`
	Platforms   []string       `json:"platforms"`
	Cover       *imagePayload  `json:"cover"`
	Screenshots []imagePayload `json:"screenshots"`
}

type resolveRequest struct {
	Titles []string `json:"titles"`
}

type resolveResponse struct {
	Games []resolvedPayload `json:"games"`
}

type resolvedPayload struct {
	Title string       `json:"title"`
	Game  gameResponse `json:"game"`
}

type errorResponse struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}
