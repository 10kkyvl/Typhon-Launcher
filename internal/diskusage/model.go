package diskusage

import (
	"errors"
	"time"

	"typhon/internal/uierr"
)

const (
	KindInstallPartial  = "install_partial"
	KindDownloadPart    = "download_part"
	KindRelocateStaging = "relocate_staging"
	KindUpdateStaging   = "update_staging"
	KindUpdatePrevious  = "update_previous"
	KindUpdateReplaced  = "update_replaced"
	KindDownloadOrphan  = "download_orphan"
)

type GameUsage struct {
	ID          string     `json:"id"`
	Title       string     `json:"title"`
	InstallDir  string     `json:"installDir"`
	SizeBytes   int64      `json:"sizeBytes"`
	SizeUnknown bool       `json:"sizeUnknown"`
	Missing     bool       `json:"missing"`
	Linked      bool       `json:"linked"`
	External    bool       `json:"external"`
	LastPlayed  *time.Time `json:"lastPlayed"`
	InstalledAt time.Time  `json:"installedAt"`
	Running     bool       `json:"running"`
	Busy        bool       `json:"busy"`
}

type DownloadUsage struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Root        string `json:"root"`
	SizeBytes   int64  `json:"sizeBytes"`
	SizeUnknown bool   `json:"sizeUnknown"`
	Missing     bool   `json:"missing"`
	Seeding     bool   `json:"seeding"`
	Installed   bool   `json:"installed"`
	GameID      string `json:"gameId"`
	Referenced  bool   `json:"referenced"`
	Reason      string `json:"reason"`
	ReasonCode  string `json:"reasonCode"`
}

type Leftover struct {
	Path        string `json:"path"`
	Kind        string `json:"kind"`
	SizeBytes   int64  `json:"sizeBytes"`
	SizeUnknown bool   `json:"sizeUnknown"`
	Deletable   bool   `json:"deletable"`
	Reason      string `json:"reason"`
	ReasonCode  string `json:"reasonCode"`
}

type GroupUsage struct {
	GameID      string `json:"gameId"`
	Title       string `json:"title"`
	InLibrary   bool   `json:"inLibrary"`
	Count       int    `json:"count"`
	SizeBytes   int64  `json:"sizeBytes"`
	SizeUnknown bool   `json:"sizeUnknown"`
}

type Totals struct {
	GamesBytes       int64 `json:"gamesBytes"`
	DownloadsBytes   int64 `json:"downloadsBytes"`
	LeftoversBytes   int64 `json:"leftoversBytes"`
	ScreenshotsBytes int64 `json:"screenshotsBytes"`
	SnapshotsBytes   int64 `json:"snapshotsBytes"`
	TotalBytes       int64 `json:"totalBytes"`
	ReclaimableBytes int64 `json:"reclaimableBytes"`
	UnknownSizes     bool  `json:"unknownSizes"`
}

// Warning is a part of the scan that failed. The rest of the report is still
// true; the part named here is missing from it rather than shown as empty.
type Warning struct {
	Area    string `json:"area"`
	Message string `json:"message"`
}

type Report struct {
	Games       []GameUsage     `json:"games"`
	Downloads   []DownloadUsage `json:"downloads"`
	Leftovers   []Leftover      `json:"leftovers"`
	Screenshots []GroupUsage    `json:"screenshots"`
	Snapshots   []GroupUsage    `json:"snapshots"`
	Warnings    []Warning       `json:"warnings"`
	Totals      Totals          `json:"totals"`
	ScannedAt   time.Time       `json:"scannedAt"`
}

var (
	errNotStarted    = uierr.New("diskusage.not_started", "служба учёта места не запущена")
	errNotScanned    = uierr.New("diskusage.not_scanned", "этого объекта нет в последнем отчёте, обновите его")
	errNotDeletable  = uierr.New("diskusage.not_deletable", "этот объект удалять нельзя")
	errUnsafePath    = uierr.New("diskusage.unsafe_path", "путь ведёт за пределы папок лаунчера или является ссылкой")
	errEmptyGameID   = uierr.New("diskusage.no_game", "не указана игра")
	errMissingPort   = errors.New("diskusage: every dependency is required")
	errNoSettingsSrc = errors.New("diskusage: settings source is required")
)

func (r Report) clone() Report {
	out := r
	out.Games = append([]GameUsage(nil), r.Games...)
	out.Downloads = append([]DownloadUsage(nil), r.Downloads...)
	out.Leftovers = append([]Leftover(nil), r.Leftovers...)
	out.Screenshots = append([]GroupUsage(nil), r.Screenshots...)
	out.Snapshots = append([]GroupUsage(nil), r.Snapshots...)
	out.Warnings = append([]Warning(nil), r.Warnings...)
	return out
}

func (r *Report) recount() {
	var t Totals
	for _, g := range r.Games {
		t.GamesBytes += g.SizeBytes
		t.UnknownSizes = t.UnknownSizes || g.SizeUnknown
	}
	for _, d := range r.Downloads {
		t.DownloadsBytes += d.SizeBytes
		t.UnknownSizes = t.UnknownSizes || d.SizeUnknown
		if !d.Referenced {
			t.ReclaimableBytes += d.SizeBytes
		}
	}
	for _, l := range r.Leftovers {
		t.LeftoversBytes += l.SizeBytes
		t.UnknownSizes = t.UnknownSizes || l.SizeUnknown
		if l.Deletable {
			t.ReclaimableBytes += l.SizeBytes
		}
	}
	for _, g := range r.Screenshots {
		t.ScreenshotsBytes += g.SizeBytes
		t.UnknownSizes = t.UnknownSizes || g.SizeUnknown
	}
	for _, g := range r.Snapshots {
		t.SnapshotsBytes += g.SizeBytes
		t.UnknownSizes = t.UnknownSizes || g.SizeUnknown
	}
	t.TotalBytes = t.GamesBytes + t.DownloadsBytes + t.LeftoversBytes + t.ScreenshotsBytes + t.SnapshotsBytes
	r.Totals = t
}
