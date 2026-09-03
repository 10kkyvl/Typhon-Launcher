package account

type ProfileSettings struct {
	ShowStats    bool     `json:"showStats"`
	ShowPlaying  bool     `json:"showPlaying"`
	ShowActivity bool     `json:"showActivity"`
	ShowOnline   bool     `json:"showOnline"`
	Showcase     []string `json:"showcase"`
}

func DefaultProfileSettings() ProfileSettings {
	return ProfileSettings{
		ShowStats:    true,
		ShowPlaying:  true,
		ShowActivity: true,
		ShowOnline:   true,
		Showcase:     []string{"favorites"},
	}
}

func withProfileDefaults(user CurrentUser) CurrentUser {
	if user.Profile.Showcase == nil {
		user.Profile = DefaultProfileSettings()
	}
	return user
}
