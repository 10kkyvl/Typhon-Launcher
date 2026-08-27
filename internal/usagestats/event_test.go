package usagestats

import (
	"strings"
	"testing"
	"time"
)

func TestValidateAcceptsAllowedEvents(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name string
		ev   Event
	}{
		{"launcher_session_started", Event{Type: TypeLauncherSessionStarted, Timestamp: now}},
		{"launcher_session_stopped", Event{Type: TypeLauncherSessionStopped, Timestamp: now, Properties: Properties{DurationSeconds: 10}}},
		{"game_started", Event{Type: TypeGameStarted, Timestamp: now, Properties: Properties{GameID: "123"}}},
		{"game_stopped", Event{Type: TypeGameStopped, Timestamp: now, Properties: Properties{GameID: "123", DurationSeconds: 5}}},
		{"download_started", Event{Type: TypeDownloadStarted, Timestamp: now, Properties: Properties{GameID: "1"}}},
		{"download_completed", Event{Type: TypeDownloadCompleted, Timestamp: now, Properties: Properties{GameID: "1", DurationSeconds: 1, BytesTotal: 2, AverageSpeedBytes: 3}}},
		{"download_failed", Event{Type: TypeDownloadFailed, Timestamp: now, Properties: Properties{GameID: "1", DurationSeconds: 1, BytesTotal: 2, ErrorCode: "network"}}},
		{"download_cancelled", Event{Type: TypeDownloadCancelled, Timestamp: now, Properties: Properties{GameID: "1", DurationSeconds: 1, BytesTotal: 2}}},
		{"install_started", Event{Type: TypeInstallStarted, Timestamp: now, Properties: Properties{GameID: "1", InstallerType: "msi_installer"}}},
		{"install_completed", Event{Type: TypeInstallCompleted, Timestamp: now, Properties: Properties{GameID: "1", InstallerType: "portable", DurationSeconds: 4}}},
		{"install_failed", Event{Type: TypeInstallFailed, Timestamp: now, Properties: Properties{GameID: "1", InstallerType: "unknown", DurationSeconds: 4, ErrorCode: "timeout"}}},
		{"update_started", Event{Type: TypeUpdateStarted, Timestamp: now, Properties: Properties{GameID: "1"}}},
		{"update_completed", Event{Type: TypeUpdateCompleted, Timestamp: now, Properties: Properties{GameID: "1", DurationSeconds: 1}}},
		{"update_failed", Event{Type: TypeUpdateFailed, Timestamp: now, Properties: Properties{GameID: "1", DurationSeconds: 1, ErrorCode: "disk_full"}}},
		{"verify_started", Event{Type: TypeVerifyStarted, Timestamp: now, Properties: Properties{GameID: "1"}}},
		{"verify_completed", Event{Type: TypeVerifyCompleted, Timestamp: now, Properties: Properties{GameID: "1", DurationSeconds: 1}}},
		{"verify_failed", Event{Type: TypeVerifyFailed, Timestamp: now, Properties: Properties{GameID: "1", DurationSeconds: 1, ErrorCode: "not_found"}}},
		{"repair_started", Event{Type: TypeRepairStarted, Timestamp: now, Properties: Properties{GameID: "1"}}},
		{"repair_completed", Event{Type: TypeRepairCompleted, Timestamp: now, Properties: Properties{GameID: "1", DurationSeconds: 1}}},
		{"repair_failed", Event{Type: TypeRepairFailed, Timestamp: now, Properties: Properties{GameID: "1", DurationSeconds: 1, ErrorCode: "unknown"}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := validate(tc.ev); err != nil {
				t.Fatalf("expected valid event, got error: %v", err)
			}
		})
	}
}

func TestValidateRejectsBadEvents(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name string
		ev   Event
	}{
		{"unknown_type", Event{Type: "not_a_real_event", Timestamp: now}},
		{"missing_timestamp", Event{Type: TypeGameStarted, Properties: Properties{GameID: "1"}}},
		{"letters_in_game_id", Event{Type: TypeGameStarted, Timestamp: now, Properties: Properties{GameID: "abc"}}},
		{"too_long_game_id", Event{Type: TypeGameStarted, Timestamp: now, Properties: Properties{GameID: strings.Repeat("1", 21)}}},
		{"zero_game_id", Event{Type: TypeGameStarted, Timestamp: now, Properties: Properties{GameID: "0"}}},
		{"negative_duration", Event{Type: TypeGameStopped, Timestamp: now, Properties: Properties{GameID: "1", DurationSeconds: -1}}},
		{"field_not_allowed_for_type", Event{Type: TypeLauncherSessionStarted, Timestamp: now, Properties: Properties{GameID: "1"}}},
		{"duration_not_allowed", Event{Type: TypeGameStarted, Timestamp: now, Properties: Properties{GameID: "1", DurationSeconds: 5}}},
		{"error_code_with_space", Event{Type: TypeDownloadFailed, Timestamp: now, Properties: Properties{GameID: "1", ErrorCode: "bad code"}}},
		{"error_code_uppercase", Event{Type: TypeDownloadFailed, Timestamp: now, Properties: Properties{GameID: "1", ErrorCode: "BAD"}}},
		{"invalid_installer_type", Event{Type: TypeInstallStarted, Timestamp: now, Properties: Properties{GameID: "1", InstallerType: "totally_custom"}}},
		{"installer_type_not_allowed", Event{Type: TypeGameStarted, Timestamp: now, Properties: Properties{GameID: "1", InstallerType: "portable"}}},
		{"negative_bytes_total", Event{Type: TypeDownloadCompleted, Timestamp: now, Properties: Properties{GameID: "1", BytesTotal: -1}}},
		{"negative_average_speed", Event{Type: TypeDownloadCompleted, Timestamp: now, Properties: Properties{GameID: "1", AverageSpeedBytes: -1}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := validate(tc.ev); err == nil {
				t.Fatal("expected validation error, got nil")
			}
		})
	}
}

func TestGameIDPatternRejectsZeroAndLeadingZero(t *testing.T) {
	for _, id := range []string{"0", "00", "0123"} {
		if gameIDPattern.MatchString(id) {
			t.Fatalf("expected %q to be rejected as a game id", id)
		}
	}
	if !gameIDPattern.MatchString("123") {
		t.Fatal("expected a plain positive integer to be accepted")
	}
}
