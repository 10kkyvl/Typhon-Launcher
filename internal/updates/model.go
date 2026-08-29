package updates

import (
	"time"
	"typhon/internal/hashdir"
)

type VersionSource string

const (
	VersionSourceRelease    VersionSource = "release_metadata"
	VersionSourceExecutable VersionSource = "executable_metadata"
	VersionSourceManifest   VersionSource = "manifest"
	VersionSourceManual     VersionSource = "manual"
	VersionSourceUnknown    VersionSource = "unknown"
)

type StrategyType string

const (
	StrategyFullRelease  StrategyType = "full_release"
	StrategyTorrentReuse StrategyType = "torrent_reuse"
	StrategyPatchChain   StrategyType = "patch_chain"
	StrategyNone         StrategyType = ""
)

type AvailabilityKind string

const (
	KindNone       AvailabilityKind = "none"
	KindUpdate     AvailabilityKind = "update"
	KindNewRelease AvailabilityKind = "new_release"
)

type State string

const (
	StateIdle        State = "idle"
	StateAvailable   State = "update_available"
	StateDownloading State = "update_downloading"
	StateReady       State = "update_ready"
	StateUpdating    State = "updating"
	StateFailed      State = "update_failed"
	StateRollback    State = "update_rollback"
)

type InstalledGame struct {
	GameID            string        `json:"gameId"`
	CanonicalGameID   string        `json:"canonicalGameId"`
	Title             string        `json:"title"`
	InstallDir        string        `json:"installDir"`
	Executable        string        `json:"executable"`
	ReleaseID         string        `json:"releaseId"`
	SourceID          string        `json:"sourceId"`
	Version           string        `json:"version"`
	VersionSource     VersionSource `json:"versionSource"`
	VersionConfidence float64       `json:"versionConfidence"`
	Edition           string        `json:"edition,omitempty"`
	Languages         []string      `json:"languages,omitempty"`
	SizeBytes         int64         `json:"sizeBytes"`
}

type Patch struct {
	ID            string    `json:"id"`
	GameID        string    `json:"gameId"`
	FromVersion   string    `json:"fromVersion"`
	ToVersion     string    `json:"toVersion"`
	FromReleaseID *string   `json:"fromReleaseId,omitempty"`
	ToReleaseID   *string   `json:"toReleaseId,omitempty"`
	ReleaseID     string    `json:"releaseId"`
	SourceID      string    `json:"sourceId,omitempty"`
	Title         string    `json:"title,omitempty"`
	Size          int64     `json:"size"`
	Priority      int       `json:"priority"`
	CreatedAt     time.Time `json:"createdAt"`
}

type CompatibilityResult struct {
	Compatible bool     `json:"compatible"`
	Confidence float64  `json:"confidence"`
	Reasons    []string `json:"reasons,omitempty"`
}

type UpdateAvailability struct {
	Available bool             `json:"available"`
	Kind      AvailabilityKind `json:"kind"`

	GameID             string `json:"gameId"`
	InstalledReleaseID string `json:"installedReleaseId"`
	TargetReleaseID    string `json:"targetReleaseId"`

	InstalledVersion string `json:"installedVersion"`
	TargetVersion    string `json:"targetVersion"`

	Confidence float64      `json:"confidence"`
	Strategy   StrategyType `json:"strategy"`

	EstimatedDownloadBytes int64 `json:"estimatedDownloadBytes"`
	RequiresFullInstall    bool  `json:"requiresFullInstall"`

	PatchCount int    `json:"patchCount"`
	Reason     string `json:"reason,omitempty"`
	TargetSize int64  `json:"targetSize"`
}

type StepKind string

const (
	StepDownload   StepKind = "download"
	StepRecheck    StepKind = "recheck"
	StepApplyPatch StepKind = "apply_patch"
	StepExtract    StepKind = "extract"
	StepInstall    StepKind = "install"
	StepVerify     StepKind = "verify"
	StepSwap       StepKind = "swap"
	StepCleanup    StepKind = "cleanup"
)

type UpdateStep struct {
	Kind        StepKind `json:"kind"`
	Label       string   `json:"label"`
	ReleaseID   string   `json:"releaseId,omitempty"`
	PatchID     string   `json:"patchId,omitempty"`
	Bytes       int64    `json:"bytes,omitempty"`
	FromVersion string   `json:"fromVersion,omitempty"`
	ToVersion   string   `json:"toVersion,omitempty"`
}

type UpdatePlan struct {
	ID     string `json:"id"`
	GameID string `json:"gameId"`

	InstalledReleaseID string `json:"installedReleaseId"`
	TargetReleaseID    string `json:"targetReleaseId"`

	InstalledVersion string `json:"installedVersion"`
	TargetVersion    string `json:"targetVersion"`

	Strategy StrategyType `json:"strategy"`
	Steps    []UpdateStep `json:"steps"`

	DownloadBytes     int64 `json:"downloadBytes"`
	ReusedBytes       int64 `json:"reusedBytes"`
	RequiredDiskBytes int64 `json:"requiredDiskBytes"`

	BackupRecommended bool `json:"backupRecommended"`
	BackupAvailable   bool `json:"backupAvailable"`
	BackupCreated     bool `json:"backupCreated"`
	RequiresRestart   bool `json:"requiresRestart"`
	RollbackAvailable bool `json:"rollbackAvailable"`
	ReuseFlat         bool `json:"reuseFlat,omitempty"`

	Patches []Patch `json:"patches,omitempty"`

	Confidence float64   `json:"confidence"`
	CreatedAt  time.Time `json:"createdAt"`
}

type UpdateHistory struct {
	ID     string `json:"id"`
	GameID string `json:"gameId"`

	FromVersion string `json:"fromVersion"`
	ToVersion   string `json:"toVersion"`

	Strategy string `json:"strategy"`

	DownloadBytes int64 `json:"downloadBytes"`

	StartedAt   time.Time  `json:"startedAt"`
	CompletedAt *time.Time `json:"completedAt"`

	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

const (
	HistoryRunning    = "running"
	HistoryCompleted  = "completed"
	HistoryFailed     = "failed"
	HistoryRolledBack = "rolled_back"
)

type FileManifestEntry = hashdir.Entry

type FileManifest = hashdir.Manifest
