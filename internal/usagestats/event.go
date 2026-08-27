package usagestats

import (
	"fmt"
	"regexp"
	"time"
)

type Event struct {
	Type       string
	Timestamp  time.Time
	Properties Properties
}

type Properties struct {
	GameID            string `json:"game_id,omitempty"`
	DurationSeconds   int64  `json:"duration_seconds,omitempty"`
	BytesTotal        int64  `json:"bytes_total,omitempty"`
	AverageSpeedBytes int64  `json:"average_speed_bytes,omitempty"`
	InstallerType     string `json:"installer_type,omitempty"`
	ErrorCode         string `json:"error_code,omitempty"`
}

const (
	TypeLauncherSessionStarted = "launcher_session_started"
	TypeLauncherSessionStopped = "launcher_session_stopped"
	TypeGameStarted            = "game_started"
	TypeGameStopped            = "game_stopped"
	TypeDownloadStarted        = "download_started"
	TypeDownloadCompleted      = "download_completed"
	TypeDownloadFailed         = "download_failed"
	TypeDownloadCancelled      = "download_cancelled"
	TypeInstallStarted         = "install_started"
	TypeInstallCompleted       = "install_completed"
	TypeInstallFailed          = "install_failed"
	TypeUpdateStarted          = "update_started"
	TypeUpdateCompleted        = "update_completed"
	TypeUpdateFailed           = "update_failed"
	TypeVerifyStarted          = "verify_started"
	TypeVerifyCompleted        = "verify_completed"
	TypeVerifyFailed           = "verify_failed"
	TypeRepairStarted          = "repair_started"
	TypeRepairCompleted        = "repair_completed"
	TypeRepairFailed           = "repair_failed"
)

type fieldSet uint8

const (
	fieldGameID fieldSet = 1 << iota
	fieldDuration
	fieldBytesTotal
	fieldAvgSpeed
	fieldInstallerType
	fieldErrorCode
)

var allowedFields = map[string]fieldSet{
	TypeLauncherSessionStarted: 0,
	TypeLauncherSessionStopped: fieldDuration,
	TypeGameStarted:            fieldGameID,
	TypeGameStopped:            fieldGameID | fieldDuration,
	TypeDownloadStarted:        fieldGameID,
	TypeDownloadCompleted:      fieldGameID | fieldDuration | fieldBytesTotal | fieldAvgSpeed,
	TypeDownloadFailed:         fieldGameID | fieldDuration | fieldBytesTotal | fieldErrorCode,
	TypeDownloadCancelled:      fieldGameID | fieldDuration | fieldBytesTotal,
	TypeInstallStarted:         fieldGameID | fieldInstallerType,
	TypeInstallCompleted:       fieldGameID | fieldInstallerType | fieldDuration,
	TypeInstallFailed:          fieldGameID | fieldInstallerType | fieldDuration | fieldErrorCode,
	TypeUpdateStarted:          fieldGameID,
	TypeUpdateCompleted:        fieldGameID | fieldDuration,
	TypeUpdateFailed:           fieldGameID | fieldDuration | fieldErrorCode,
	TypeVerifyStarted:          fieldGameID,
	TypeVerifyCompleted:        fieldGameID | fieldDuration,
	TypeVerifyFailed:           fieldGameID | fieldDuration | fieldErrorCode,
	TypeRepairStarted:          fieldGameID,
	TypeRepairCompleted:        fieldGameID | fieldDuration,
	TypeRepairFailed:           fieldGameID | fieldDuration | fieldErrorCode,
}

var (
	gameIDPattern    = regexp.MustCompile(`^[1-9][0-9]{0,19}$`)
	errorCodePattern = regexp.MustCompile(`^[a-z0-9_]{1,40}$`)
)

var installerTypes = map[string]bool{
	"portable":      true,
	"archive_zip":   true,
	"archive_7z":    true,
	"archive_rar":   true,
	"exe_installer": true,
	"msi_installer": true,
	"unknown":       true,
}

func validate(ev Event) error {
	allowed, ok := allowedFields[ev.Type]
	if !ok {
		return fmt.Errorf("unknown event type %q", ev.Type)
	}
	if ev.Timestamp.IsZero() {
		return fmt.Errorf("event %q: missing timestamp", ev.Type)
	}
	p := ev.Properties

	if p.GameID != "" {
		if allowed&fieldGameID == 0 {
			return fmt.Errorf("event %q: game_id not allowed", ev.Type)
		}
		if !gameIDPattern.MatchString(p.GameID) {
			return fmt.Errorf("event %q: invalid game_id %q", ev.Type, p.GameID)
		}
	}

	if p.DurationSeconds != 0 && allowed&fieldDuration == 0 {
		return fmt.Errorf("event %q: duration_seconds not allowed", ev.Type)
	}
	if p.DurationSeconds < 0 {
		return fmt.Errorf("event %q: negative duration_seconds", ev.Type)
	}

	if p.BytesTotal != 0 && allowed&fieldBytesTotal == 0 {
		return fmt.Errorf("event %q: bytes_total not allowed", ev.Type)
	}
	if p.BytesTotal < 0 {
		return fmt.Errorf("event %q: negative bytes_total", ev.Type)
	}

	if p.AverageSpeedBytes != 0 && allowed&fieldAvgSpeed == 0 {
		return fmt.Errorf("event %q: average_speed_bytes not allowed", ev.Type)
	}
	if p.AverageSpeedBytes < 0 {
		return fmt.Errorf("event %q: negative average_speed_bytes", ev.Type)
	}

	if p.InstallerType != "" {
		if allowed&fieldInstallerType == 0 {
			return fmt.Errorf("event %q: installer_type not allowed", ev.Type)
		}
		if !installerTypes[p.InstallerType] {
			return fmt.Errorf("event %q: invalid installer_type %q", ev.Type, p.InstallerType)
		}
	}

	if p.ErrorCode != "" {
		if allowed&fieldErrorCode == 0 {
			return fmt.Errorf("event %q: error_code not allowed", ev.Type)
		}
		if !errorCodePattern.MatchString(p.ErrorCode) {
			return fmt.Errorf("event %q: invalid error_code %q", ev.Type, p.ErrorCode)
		}
	}

	return nil
}
