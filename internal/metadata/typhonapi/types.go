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
	SteamAppID  int64             `json:"steamAppId"`
	ProviderID  string            `json:"providerId"`
	Title       string            `json:"title"`
	Summary     string            `json:"summary"`
	ReleaseDate *time.Time        `json:"releaseDate"`
	Developer   string            `json:"developer"`
	Publisher   string            `json:"publisher"`
	Genres      []string          `json:"genres"`
	Themes      []string          `json:"themes"`
	Platforms   []string          `json:"platforms"`
	GameType    string            `json:"gameType"`
	StoreLinks  map[string]string `json:"storeLinks"`
	Cover       *imagePayload     `json:"cover"`
	Screenshots []imagePayload    `json:"screenshots"`
	Steam       *steamPayload     `json:"steam"`
}

type requirementPayload struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

type steamPayload struct {
	AppID        int64  `json:"appId"`
	URL          string `json:"url"`
	Requirements struct {
		Minimum     []requirementPayload `json:"minimum"`
		Recommended []requirementPayload `json:"recommended"`
	} `json:"requirements"`
	DLC []struct {
		AppID int64  `json:"appId"`
		Name  string `json:"name"`
	} `json:"dlc"`
	Controller      string `json:"controller"`
	Metacritic      *int   `json:"metacritic"`
	Recommendations *int   `json:"recommendations"`
}

type savesResponse struct {
	PageURL string   `json:"pageUrl"`
	Windows []string `json:"windows"`
	Source  string   `json:"source"`
	License string   `json:"license"`
	Via     string   `json:"via"`
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

type achievementsResponse struct {
	AppID int64                `json:"appId"`
	Total int                  `json:"total"`
	Items []achievementPayload `json:"items"`
}

type achievementPayload struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Hidden      bool   `json:"hidden"`
	Icon        string `json:"icon"`
	IconGray    string `json:"iconGray"`
}

type Achievement struct {
	Key         string
	Name        string
	Description string
	Hidden      bool
	Icon        string
	IconGray    string
}

type AchievementSet struct {
	AppID string
	Items []Achievement
}
