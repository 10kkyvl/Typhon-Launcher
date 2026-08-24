package install

import (
	"time"

	"typhon/internal/download"
	"typhon/internal/library"
)

type Type string

const (
	TypePortable     Type = "portable"
	TypeArchiveZip   Type = "archive_zip"
	TypeArchive7z    Type = "archive_7z"
	TypeArchiveRar   Type = "archive_rar"
	TypeExeInstaller Type = "exe_installer"
	TypeMsiInstaller Type = "msi_installer"
	TypeUnknown      Type = "unknown"
)

type Status string

const (
	StatusPending        Status = "pending"
	StatusPreparing      Status = "preparing"
	StatusInstalling     Status = "installing"
	StatusExtracting     Status = "extracting"
	StatusVerifying      Status = "verifying"
	StatusWaitingForUser Status = "waiting_for_user"
	StatusCompleted      Status = "completed"
	StatusFailed         Status = "failed"
	StatusCancelled      Status = "cancelled"
	StatusInterrupted    Status = "interrupted"
)

type Candidate struct {
	Path  string  `json:"path"`
	Score float64 `json:"score"`
}

type Plan struct {
	Type                    Type        `json:"type"`
	SourcePath              string      `json:"sourcePath"`
	ContentRoot             string      `json:"contentRoot"`
	Destination             string      `json:"destination"`
	InstallerPath           string      `json:"installerPath"`
	ExtraInstallers         []string    `json:"extraInstallers,omitempty"`
	WorkingDir              string      `json:"workingDir"`
	Engine                  Engine      `json:"engine"`
	Silent                  bool        `json:"silent"`
	ArchivePath             string      `json:"archivePath"`
	CompressedSize          int64       `json:"compressedSize"`
	EstimatedSize           int64       `json:"estimatedSize"`
	RequiresUserInteraction bool        `json:"requiresUserInteraction"`
	CanAutoInstall          bool        `json:"canAutoInstall"`
	Candidates              []Candidate `json:"candidates"`
}

type Progress struct {
	BytesDone   int64
	BytesTotal  int64
	CurrentFile string
}

const (
	ModeMove = "move"
	ModeCopy = "copy"
)

type Installation struct {
	ID               string            `json:"id"`
	DownloadID       string            `json:"downloadId"`
	GameID           string            `json:"gameId"`
	Name             string            `json:"name"`
	Type             Type              `json:"type"`
	Status           Status            `json:"status"`
	Mode             string            `json:"mode"`
	SourcePath       string            `json:"sourcePath"`
	ContentRoot      string            `json:"contentRoot"`
	Destination      string            `json:"destination"`
	InstallerPath    string            `json:"installerPath"`
	ExtraInstallers  []string          `json:"extraInstallers,omitempty"`
	ManualInstaller  bool              `json:"manualInstaller,omitempty"`
	WorkingDir       string            `json:"workingDir"`
	Engine           Engine            `json:"engine"`
	Silent           bool              `json:"silent"`
	ArchivePath      string            `json:"archivePath"`
	Progress         float64           `json:"progress"`
	CurrentFile      string            `json:"currentFile"`
	BytesDone        int64             `json:"bytesDone"`
	BytesTotal       int64             `json:"bytesTotal"`
	Executable       string            `json:"executable"`
	Candidates       []Candidate       `json:"candidates"`
	Origin           download.Origin   `json:"origin"`
	Unattended       bool              `json:"unattended,omitempty"`
	SkipRegister     bool              `json:"skipRegister,omitempty"`
	Owned            bool              `json:"owned,omitempty"`
	Uninstall        library.Uninstall `json:"uninstall,omitzero"`
	UninstallUnknown bool              `json:"uninstallUnknown,omitempty"`
	DetectedVersion  string            `json:"detectedVersion"`
	VersionSource    string            `json:"versionSource"`
	StartedAt        time.Time         `json:"startedAt"`
	CompletedAt      *time.Time        `json:"completedAt"`
	Error            string            `json:"error"`
}

type PlanInfo struct {
	Plan          Plan   `json:"plan"`
	DownloadID    string `json:"downloadId"`
	Name          string `json:"name"`
	RequiredBytes int64  `json:"requiredBytes"`
	FreeBytes     int64  `json:"freeBytes"`
	Seeding       bool   `json:"seeding"`
}

type StartOptions struct {
	Destination   string `json:"destination"`
	Mode          string `json:"mode"`
	Type          Type   `json:"type"`
	InstallerPath string `json:"installerPath"`
	Unattended    bool   `json:"unattended,omitempty"`
	SkipRegister  bool   `json:"skipRegister,omitempty"`
}

type RemovedEvent struct {
	ID string `json:"id"`
}

func snapshotOf(i *Installation) Installation {
	out := *i
	out.Candidates = append([]Candidate(nil), i.Candidates...)
	out.ExtraInstallers = append([]string(nil), i.ExtraInstallers...)
	if i.CompletedAt != nil {
		at := *i.CompletedAt
		out.CompletedAt = &at
	}
	return out
}

func controlled(t Type) bool {
	switch t {
	case TypePortable, TypeArchiveZip, TypeArchive7z, TypeArchiveRar:
		return true
	}
	return false
}

func archived(t Type) bool {
	switch t {
	case TypeArchiveZip, TypeArchive7z, TypeArchiveRar:
		return true
	}
	return false
}

func external(t Type) bool {
	return t == TypeExeInstaller || t == TypeMsiInstaller
}

func ownDestination(t Type, silent bool) bool {
	return controlled(t) || (external(t) && silent)
}

func transient(s Status) bool {
	switch s {
	case StatusPending, StatusPreparing, StatusInstalling, StatusExtracting, StatusVerifying:
		return true
	}
	return false
}

func active(s Status) bool {
	return transient(s) || s == StatusWaitingForUser
}

func retryable(s Status) bool {
	switch s {
	case StatusFailed, StatusCancelled, StatusInterrupted:
		return true
	}
	return false
}

func ratio(done, total int64) float64 {
	if total <= 0 {
		return 0
	}
	if done >= total {
		return 1
	}
	return float64(done) / float64(total)
}
