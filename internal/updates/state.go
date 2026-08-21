package updates

import "time"

type Update struct {
	GameID       string             `json:"gameId"`
	Title        string             `json:"title"`
	State        State              `json:"state"`
	Availability UpdateAvailability `json:"availability"`
	Plan         *UpdatePlan        `json:"plan,omitempty"`
	Planning     bool               `json:"planning"`
	DownloadID   string             `json:"downloadId,omitempty"`
	InstallID    string             `json:"installId,omitempty"`
	Step         StepKind           `json:"step,omitempty"`
	Progress     float64            `json:"progress"`
	Message      string             `json:"message,omitempty"`
	Error        string             `json:"error,omitempty"`
	CanRollback  bool               `json:"canRollback"`
	CheckedAt    time.Time          `json:"checkedAt"`
}

type VerifyMethod string

const (
	MethodTorrent     VerifyMethod = "torrent"
	MethodManifest    VerifyMethod = "manifest"
	MethodUnavailable VerifyMethod = "unavailable"
)

type VerifyState struct {
	GameID          string          `json:"gameId"`
	Method          VerifyMethod    `json:"method"`
	Running         bool            `json:"running"`
	Repairing       bool            `json:"repairing"`
	Progress        float64         `json:"progress"`
	ProcessedBytes  int64           `json:"processedBytes"`
	CurrentFile     string          `json:"currentFile,omitempty"`
	Ratio           float64         `json:"ratio"`
	TotalBytes      int64           `json:"totalBytes"`
	OkBytes         int64           `json:"okBytes"`
	MissingFiles    int             `json:"missingFiles"`
	CorruptedPieces int             `json:"corruptedPieces"`
	Issues          []ManifestIssue `json:"issues,omitempty"`
	Extra           []string        `json:"extra,omitempty"`
	Repairable      bool            `json:"repairable"`
	Flat            bool            `json:"flat,omitempty"`
	InfoHash        string          `json:"infoHash,omitempty"`
	CheckedAt       *time.Time      `json:"checkedAt"`
	Error           string          `json:"error,omitempty"`
}

type Rollback struct {
	GameID      string     `json:"gameId"`
	Path        string     `json:"path"`
	InstallDir  string     `json:"installDir"`
	Executable  string     `json:"executable"`
	Version     string     `json:"version"`
	ReleaseID   string     `json:"releaseId,omitempty"`
	SourceID    string     `json:"sourceId,omitempty"`
	AwaitLaunch bool       `json:"awaitLaunch"`
	KeepUntil   *time.Time `json:"keepUntil,omitempty"`
	CreatedAt   time.Time  `json:"createdAt"`
}
