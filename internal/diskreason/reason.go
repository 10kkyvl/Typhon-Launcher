// Package diskreason holds the closed set of codes that say why the disk
// usage screen does not offer an object for deletion. The owners of the data
// (install, download, relocate) return them next to their Russian reason text,
// and the frontend translates them; the text is kept for logs.
package diskreason

const (
	OwnedByInstall  = "diskusage.reason.owned_by_install"
	OnlyCopy        = "diskusage.reason.only_copy"
	NoRecord        = "diskusage.reason.no_record"
	RelocateActive  = "diskusage.reason.relocate_active"
	RelocateJournal = "diskusage.reason.relocate_journal"
	HoldsGame       = "diskusage.reason.holds_game"
	UpdateOwned     = "diskusage.reason.update_owned"
	Symlink         = "diskusage.reason.symlink"
	ForeignPart     = "diskusage.reason.foreign_part"
	OwnedByDownload = "diskusage.reason.owned_by_download"
	Seeding         = "diskusage.reason.seeding"
	Incomplete      = "diskusage.reason.incomplete"
	Unreadable      = "diskusage.reason.unreadable"
	Unknown         = "diskusage.reason.unknown"
)
