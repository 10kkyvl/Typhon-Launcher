package social

import "time"

type UserCard struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
	AvatarURL   string `json:"avatarUrl"`
}

type GameCard struct {
	IGDBID   int64  `json:"igdbId"`
	Title    string `json:"title"`
	CoverURL string `json:"coverUrl"`
}

type PlayedGame struct {
	GameCard
	PlaytimeSeconds *int64     `json:"playtimeSeconds,omitempty"`
	Status          string     `json:"status"`
	Favorite        bool       `json:"favorite"`
	LastPlayedAt    *time.Time `json:"lastPlayedAt"`
}

type StatsView struct {
	Games     int  `json:"games"`
	Completed int  `json:"completed"`
	Hours     *int `json:"hours,omitempty"`
}

type CommonGame struct {
	GameCard
	ViewerOwned bool `json:"viewerOwned"`
	TargetOwned bool `json:"targetOwned"`
}

type CommonGames struct {
	Count int          `json:"count"`
	Games []CommonGame `json:"games"`
}

type ShowcaseBlock struct {
	Kind  string     `json:"kind"`
	Games []GameCard `json:"games"`
}

type PresenceView struct {
	Status     string     `json:"status"`
	GameID     *int64     `json:"gameId,omitempty"`
	GameTitle  string     `json:"gameTitle,omitempty"`
	Since      *time.Time `json:"since,omitempty"`
	LastSeenAt *time.Time `json:"lastSeenAt,omitempty"`
}

type PublicProfile struct {
	UserCard
	Bio            string          `json:"bio"`
	Relation       string          `json:"relation"`
	Visibility     string          `json:"visibility"`
	Stats          *StatsView      `json:"stats"`
	Favorites      []GameCard      `json:"favorites"`
	Showcase       []ShowcaseBlock `json:"showcase"`
	RecentlyPlayed []PlayedGame    `json:"recentlyPlayed"`
	Common         *CommonGames    `json:"common"`
	MutualFriends  []UserCard      `json:"mutualFriends"`
	MutualCount    int             `json:"mutualCount"`
	CreatedAt      time.Time       `json:"createdAt"`
	Presence       *PresenceView   `json:"presence,omitempty"`
}

type FriendView struct {
	UserCard
	Since    time.Time     `json:"since"`
	Presence *PresenceView `json:"presence,omitempty"`
}

type RequestView struct {
	UserCard
	CreatedAt   time.Time `json:"createdAt"`
	MutualCount int       `json:"mutualCount"`
	CommonCount int       `json:"commonCount"`
}

type FriendsPage struct {
	Friends  []FriendView  `json:"friends"`
	Incoming []RequestView `json:"incoming"`
	Outgoing []RequestView `json:"outgoing"`
}

type GameFriend struct {
	UserCard
	PlaytimeSeconds *int64 `json:"playtimeSeconds,omitempty"`
	Status          string `json:"status"`
}

type GameFriends struct {
	Played     []GameFriend `json:"played"`
	PlayingNow []UserCard   `json:"playingNow"`
}

type GamesPage struct {
	Games []PlayedGame `json:"games"`
	Next  string       `json:"next"`
}

type SendResult struct {
	Request  RequestView `json:"request"`
	Accepted bool        `json:"accepted"`
}

type RequestsSignal struct {
	Incoming int `json:"incoming"`
}
