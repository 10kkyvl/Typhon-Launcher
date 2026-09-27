package account

import "encoding/json"

const (
	VisibilityPublic  = "public"
	VisibilityFriends = "friends"
	VisibilityPrivate = "private"
)

type ProfileSettings struct {
	Visibility   string            `json:"visibility"`
	ShowOnline   bool              `json:"showOnline"`
	ShowPlaying  bool              `json:"showPlaying"`
	ShowPlaytime bool              `json:"showPlaytime"`
	ShowLibrary  bool              `json:"showLibrary"`
	ShowActivity bool              `json:"showActivity"`
	ShowStats    bool              `json:"showStats"`
	Showcase     []string          `json:"showcase"`
	Appearance   ProfileAppearance `json:"appearance"`
}

type ProfileAppearance struct {
	Theme         string `json:"theme"`
	Accent        string `json:"accent"`
	CoverURL      string `json:"coverUrl"`
	CoverDim      int    `json:"coverDim"`
	CoverPosition int    `json:"coverPosition"`
}

func (a *ProfileAppearance) UnmarshalJSON(data []byte) error {
	type fields struct {
		Theme         *string `json:"theme"`
		Accent        *string `json:"accent"`
		CoverURL      *string `json:"coverUrl"`
		CoverDim      *int    `json:"coverDim"`
		CoverPosition *int    `json:"coverPosition"`
	}
	var in fields
	if err := json.Unmarshal(data, &in); err != nil {
		return err
	}
	out := DefaultProfileAppearance()
	if in.Theme != nil {
		out.Theme = *in.Theme
	}
	if in.Accent != nil {
		out.Accent = *in.Accent
	}
	if in.CoverURL != nil {
		out.CoverURL = *in.CoverURL
	}
	if in.CoverDim != nil {
		out.CoverDim = *in.CoverDim
	}
	if in.CoverPosition != nil {
		out.CoverPosition = *in.CoverPosition
	}
	*a = out
	return nil
}

func DefaultProfileAppearance() ProfileAppearance {
	return ProfileAppearance{Theme: "midnight", Accent: "#67d8ef", CoverDim: 35, CoverPosition: 50}
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
		Appearance:   DefaultProfileAppearance(),
	}
}

func withProfileDefaults(user CurrentUser) CurrentUser {
	if user.Profile.Showcase == nil {
		legacyAppearance := user.Profile.Appearance
		user.Profile = DefaultProfileSettings()
		user.Profile.Appearance = legacyAppearance
	}
	if user.Profile.Visibility == "" {
		user.Profile.Visibility = VisibilityFriends
		user.Profile.ShowPlaytime = true
		user.Profile.ShowLibrary = true
	}
	user.Profile.Appearance = mergeAppearance(DefaultProfileAppearance(), user.Profile.Appearance)
	return user
}

func mergeAppearance(defaults, value ProfileAppearance) ProfileAppearance {
	if value.Theme == "" {
		return defaults
	}
	defaults.Theme = value.Theme
	if value.Accent != "" {
		defaults.Accent = value.Accent
	}
	if value.CoverURL != "" {
		defaults.CoverURL = value.CoverURL
	}
	if value.CoverDim >= 0 && value.CoverDim <= 100 {
		defaults.CoverDim = value.CoverDim
	}
	if value.CoverPosition >= 0 && value.CoverPosition <= 100 {
		defaults.CoverPosition = value.CoverPosition
	}
	return defaults
}
