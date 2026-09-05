package account

const (
	VisibilityPublic  = "public"
	VisibilityFriends = "friends"
	VisibilityPrivate = "private"
)

type ProfileSettings struct {
	Visibility   string   `json:"visibility"`
	ShowOnline   bool     `json:"showOnline"`
	ShowPlaying  bool     `json:"showPlaying"`
	ShowPlaytime bool     `json:"showPlaytime"`
	ShowLibrary  bool     `json:"showLibrary"`
	ShowActivity bool     `json:"showActivity"`
	ShowStats    bool     `json:"showStats"`
	Showcase     []string `json:"showcase"`
}

func DefaultProfileSettings() ProfileSettings {
	return ProfileSettings{
		Visibility:   VisibilityFriends,
		ShowOnline:   true,
		ShowPlaying:  true,
		ShowPlaytime: true,
		ShowLibrary:  true,
		ShowActivity: true,
		ShowStats:    true,
		Showcase:     []string{"favorites"},
	}
}

func withProfileDefaults(user CurrentUser) CurrentUser {
	if user.Profile.Showcase == nil {
		user.Profile = DefaultProfileSettings()
		return user
	}
	if user.Profile.Visibility == "" {
		user.Profile.Visibility = VisibilityFriends
		user.Profile.ShowPlaytime = true
		user.Profile.ShowLibrary = true
	}
	return user
}
