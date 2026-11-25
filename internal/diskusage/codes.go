package diskusage

import "typhon/internal/diskreason"

// Warning codes say which part of a scan failed and why, from a closed set
// the window translates; the area follows from the code.
const (
	WarnSizeNotSaved          = "diskusage.warning.size_not_saved"
	WarnFolderUnreadable      = "diskusage.warning.folder_unreadable"
	WarnMovesUnchecked        = "diskusage.warning.moves_unchecked"
	WarnRemovalsUnreadable    = "diskusage.warning.removals_unreadable"
	WarnScreenshotsNoFolder   = "diskusage.warning.screenshots_no_folder"
	WarnScreenshotsUnreadable = "diskusage.warning.screenshots_unreadable"
	WarnSnapshotsDisabled     = "diskusage.warning.snapshots_disabled"
	WarnSnapshotsUnreadable   = "diskusage.warning.snapshots_unreadable"
)

var warningArea = map[string]string{
	WarnSizeNotSaved:          "games",
	WarnFolderUnreadable:      "leftovers",
	WarnMovesUnchecked:        "leftovers",
	WarnRemovalsUnreadable:    "leftovers",
	WarnScreenshotsNoFolder:   "screenshots",
	WarnScreenshotsUnreadable: "screenshots",
	WarnSnapshotsDisabled:     "snapshots",
	WarnSnapshotsUnreadable:   "snapshots",
}

// The owners' codes the warnings tell apart.
const (
	screenshotsNoLibrary = "screenshots.no_library"
	snapshotsDisabled    = "savebackup.disabled"
)

// Reason codes disk usage gives itself, next to the ones the owners of the
// data return (diskreason).
const (
	// reasonGamesUnchecked: the installs could not compare a download with the
	// game list (install.no_game_list).
	reasonGamesUnchecked = "diskusage.reason.games_unchecked"
)

// reasonCodes is every reason code a report can carry. The frontend's table
// lists the same set; a code outside it reaches the window untranslated.
var reasonCodes = []string{
	diskreason.OwnedByInstall,
	diskreason.OnlyCopy,
	diskreason.NoRecord,
	diskreason.RelocateActive,
	diskreason.RelocateJournal,
	diskreason.HoldsGame,
	diskreason.UpdateOwned,
	diskreason.Symlink,
	diskreason.ForeignPart,
	diskreason.OwnedByDownload,
	diskreason.Seeding,
	diskreason.Incomplete,
	diskreason.Unreadable,
	diskreason.Unknown,
	reasonGamesUnchecked,
}
