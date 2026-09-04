package settings

type Portable struct {
	Theme                    *string  `json:"theme,omitempty"`
	UIScale                  *float64 `json:"uiScale,omitempty"`
	Language                 *string  `json:"language,omitempty"`
	AnimationsEnabled        *bool    `json:"animationsEnabled,omitempty"`
	MinimizeToTray           *bool    `json:"minimizeToTray,omitempty"`
	DiscordRichPresence      *bool    `json:"discordRichPresence,omitempty"`
	SeedAfterDownload        *bool    `json:"seedAfterDownload,omitempty"`
	UploadWhileDownloading   *bool    `json:"uploadWhileDownloading,omitempty"`
	InstallCleanupPolicy     *string  `json:"installCleanupPolicy,omitempty"`
	AutoInstall              *bool    `json:"autoInstall,omitempty"`
	SourceRefreshInterval    *string  `json:"sourceRefreshInterval,omitempty"`
	VerifyAfterInstall       *bool    `json:"verifyAfterInstall,omitempty"`
	InstallSkipShortcuts     *bool    `json:"installSkipShortcuts,omitempty"`
	InstallSkipExtras        *bool    `json:"installSkipExtras,omitempty"`
	DesktopShortcuts         *bool    `json:"desktopShortcuts,omitempty"`
	UpdateCheckAutomatically *bool    `json:"updateCheckAutomatically,omitempty"`
	UpdateAutoDownload       *bool    `json:"updateAutoDownload,omitempty"`
	UpdateAutoInstall        *bool    `json:"updateAutoInstall,omitempty"`
	UpdateSaveBackup         *bool    `json:"updateSaveBackup,omitempty"`
	KeepPreviousVersion      *string  `json:"keepPreviousVersion,omitempty"`
	AllowTorrentReuse        *bool    `json:"allowTorrentReuse,omitempty"`
}

func PortableOf(s Settings) Portable {
	return Portable{
		Theme:                    &s.Theme,
		UIScale:                  &s.UIScale,
		Language:                 &s.Language,
		AnimationsEnabled:        &s.AnimationsEnabled,
		MinimizeToTray:           &s.MinimizeToTray,
		DiscordRichPresence:      &s.DiscordRichPresence,
		SeedAfterDownload:        &s.SeedAfterDownload,
		UploadWhileDownloading:   &s.UploadWhileDownloading,
		InstallCleanupPolicy:     &s.InstallCleanupPolicy,
		AutoInstall:              &s.AutoInstall,
		SourceRefreshInterval:    &s.SourceRefreshInterval,
		VerifyAfterInstall:       &s.VerifyAfterInstall,
		InstallSkipShortcuts:     &s.InstallSkipShortcuts,
		InstallSkipExtras:        &s.InstallSkipExtras,
		DesktopShortcuts:         &s.DesktopShortcuts,
		UpdateCheckAutomatically: &s.UpdateCheckAutomatically,
		UpdateAutoDownload:       &s.UpdateAutoDownload,
		UpdateAutoInstall:        &s.UpdateAutoInstall,
		UpdateSaveBackup:         &s.UpdateSaveBackup,
		KeepPreviousVersion:      &s.KeepPreviousVersion,
		AllowTorrentReuse:        &s.AllowTorrentReuse,
	}
}

func ApplyPortable(s Settings, p Portable) Settings {
	applyString(&s.Theme, p.Theme)
	applyFloat(&s.UIScale, p.UIScale)
	applyString(&s.Language, p.Language)
	applyBool(&s.AnimationsEnabled, p.AnimationsEnabled)
	applyBool(&s.MinimizeToTray, p.MinimizeToTray)
	applyBool(&s.DiscordRichPresence, p.DiscordRichPresence)
	applyBool(&s.SeedAfterDownload, p.SeedAfterDownload)
	applyBool(&s.UploadWhileDownloading, p.UploadWhileDownloading)
	applyString(&s.InstallCleanupPolicy, p.InstallCleanupPolicy)
	applyBool(&s.AutoInstall, p.AutoInstall)
	applyString(&s.SourceRefreshInterval, p.SourceRefreshInterval)
	applyBool(&s.VerifyAfterInstall, p.VerifyAfterInstall)
	applyBool(&s.InstallSkipShortcuts, p.InstallSkipShortcuts)
	applyBool(&s.InstallSkipExtras, p.InstallSkipExtras)
	applyBool(&s.DesktopShortcuts, p.DesktopShortcuts)
	applyBool(&s.UpdateCheckAutomatically, p.UpdateCheckAutomatically)
	applyBool(&s.UpdateAutoDownload, p.UpdateAutoDownload)
	applyBool(&s.UpdateAutoInstall, p.UpdateAutoInstall)
	applyBool(&s.UpdateSaveBackup, p.UpdateSaveBackup)
	applyString(&s.KeepPreviousVersion, p.KeepPreviousVersion)
	applyBool(&s.AllowTorrentReuse, p.AllowTorrentReuse)
	return s
}

func applyString(dst *string, src *string) {
	if src != nil {
		*dst = *src
	}
}

func applyFloat(dst *float64, src *float64) {
	if src != nil {
		*dst = *src
	}
}

func applyBool(dst *bool, src *bool) {
	if src != nil {
		*dst = *src
	}
}
