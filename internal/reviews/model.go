package reviews

import "time"

type Author struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
	AvatarURL   string `json:"avatarUrl"`
}

type Review struct {
	ID              int64     `json:"id"`
	GameID          int64     `json:"gameId"`
	Author          Author    `json:"author"`
	Recommended     bool      `json:"recommended"`
	Body            string    `json:"body"`
	PlaytimeSeconds int64     `json:"playtimeSeconds"`
	Helpful         int       `json:"helpful"`
	Unhelpful       int       `json:"unhelpful"`
	MyVote          string    `json:"myVote"`
	Mine            bool      `json:"mine"`
	Hidden          bool      `json:"hidden"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

type Summary struct {
	Total    int `json:"total"`
	Positive int `json:"positive"`
}

type Page struct {
	Summary Summary  `json:"summary"`
	Reviews []Review `json:"reviews"`
	Next    string   `json:"next"`
}

type Eligibility struct {
	CanPost                 bool   `json:"canPost"`
	Reason                  string `json:"reason"`
	RetryAt                 string `json:"retryAt"`
	PlaytimeSeconds         int64  `json:"playtimeSeconds"`
	RequiredPlaytimeSeconds int64  `json:"requiredPlaytimeSeconds"`
}

type Mine struct {
	Review      *Review     `json:"review"`
	Eligibility Eligibility `json:"eligibility"`
}

type VoteResult struct {
	Helpful   int    `json:"helpful"`
	Unhelpful int    `json:"unhelpful"`
	MyVote    string `json:"myVote"`
}

type Limits struct {
	MinBodyRunes       int   `json:"minBodyRunes"`
	MaxBodyRunes       int   `json:"maxBodyRunes"`
	MinPlaytimeSeconds int64 `json:"minPlaytimeSeconds"`
}
