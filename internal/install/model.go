package install

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
	WorkingDir              string      `json:"workingDir"`
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
