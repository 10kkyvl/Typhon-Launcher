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
	StatusEmoji  *string           `json:"statusEmoji,omitempty"`
	StatusText   *string           `json:"statusText,omitempty"`
	Layout       OptionalLayout    `json:"layout,omitzero"`
}

type ProfileAppearance struct {
	Theme         string `json:"theme"`
	Accent        string `json:"accent"`
	CoverURL      string `json:"coverUrl"`
	CoverDim      int    `json:"coverDim"`
	CoverPosition int    `json:"coverPosition"`
	CustomFrom    string `json:"customFrom"`
	CustomTo      string `json:"customTo"`
	CustomAngle   int    `json:"customAngle"`
	AutoSource    string `json:"autoSource"`
	AvatarFrame   string `json:"avatarFrame"`
	NameStyle     string `json:"nameStyle"`
	Parallax      bool   `json:"parallax"`
}

func (a *ProfileAppearance) UnmarshalJSON(data []byte) error {
	type fields struct {
		Theme         *string `json:"theme"`
		Accent        *string `json:"accent"`
		CoverURL      *string `json:"coverUrl"`
		CoverDim      *int    `json:"coverDim"`
		CoverPosition *int    `json:"coverPosition"`
		CustomFrom    *string `json:"customFrom"`
		CustomTo      *string `json:"customTo"`
		CustomAngle   *int    `json:"customAngle"`
		AutoSource    *string `json:"autoSource"`
		AvatarFrame   *string `json:"avatarFrame"`
		NameStyle     *string `json:"nameStyle"`
		Parallax      *bool   `json:"parallax"`
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
	if in.CustomFrom != nil {
		out.CustomFrom = *in.CustomFrom
	}
	if in.CustomTo != nil {
		out.CustomTo = *in.CustomTo
	}
	if in.CustomAngle != nil {
		out.CustomAngle = *in.CustomAngle
	}
	if in.AutoSource != nil {
		out.AutoSource = *in.AutoSource
	}
	if in.AvatarFrame != nil {
		out.AvatarFrame = *in.AvatarFrame
	}
	if in.NameStyle != nil {
		out.NameStyle = *in.NameStyle
	}
	if in.Parallax != nil {
		out.Parallax = *in.Parallax
	}
	*a = out
	return nil
}

func DefaultProfileAppearance() ProfileAppearance {
	return ProfileAppearance{
		Theme:         "midnight",
		Accent:        "#67d8ef",
		CoverDim:      35,
		CoverPosition: 50,
		CustomFrom:    "#142235",
		CustomTo:      "#111923",
		CustomAngle:   125,
		AutoSource:    "playing",
		AvatarFrame:   "none",
		NameStyle:     "plain",
		Parallax:      true,
	}
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
		legacy := user.Profile
		user.Profile = DefaultProfileSettings()
		user.Profile.Appearance = legacy.Appearance
		user.Profile.StatusEmoji = legacy.StatusEmoji
		user.Profile.StatusText = legacy.StatusText
		user.Profile.Layout = legacy.Layout
	}
	if user.Profile.Layout.Value == nil {
		user.Profile.Layout = OptionalLayout{}
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
	if value.CustomFrom != "" {
		defaults.CustomFrom = value.CustomFrom
	}
	if value.CustomTo != "" {
		defaults.CustomTo = value.CustomTo
	}
	if value.CustomAngle >= 0 && value.CustomAngle <= 360 {
		defaults.CustomAngle = value.CustomAngle
	}
	if value.AutoSource != "" {
		defaults.AutoSource = value.AutoSource
	}
	if value.AvatarFrame != "" {
		defaults.AvatarFrame = value.AvatarFrame
	}
	if value.NameStyle != "" {
		defaults.NameStyle = value.NameStyle
	}
	defaults.Parallax = value.Parallax
	return defaults
}
