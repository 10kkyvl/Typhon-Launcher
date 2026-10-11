package main

import (
	"testing"

	"typhon/internal/heartbeat"
	"typhon/internal/settings"
)

func TestHeartbeatPolicy(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*settings.Settings)
		want   heartbeat.Policy
	}{
		{"fresh install", func(*settings.Settings) {}, heartbeat.Policy{Enabled: true}},
		{"heartbeat off", func(s *settings.Settings) { s.SessionHeartbeat = false }, heartbeat.Policy{}},
		{"heartbeat off with current consent", func(s *settings.Settings) {
			s.SessionHeartbeat = false
			s.AnonymousUsageStats = true
			s.TelemetryConsentVersion = settings.CurrentTelemetryConsent
		}, heartbeat.Policy{Identified: true}},
		{"statistics answered on the previous prompt", func(s *settings.Settings) {
			s.AnonymousUsageStats = true
			s.TelemetryConsentVersion = settings.CurrentTelemetryConsent - 1
		}, heartbeat.Policy{Enabled: true}},
		{"statistics declined on the current prompt", func(s *settings.Settings) {
			s.TelemetryConsentVersion = settings.CurrentTelemetryConsent
		}, heartbeat.Policy{Enabled: true}},
		{"statistics on the current prompt", func(s *settings.Settings) {
			s.AnonymousUsageStats = true
			s.TelemetryConsentVersion = settings.CurrentTelemetryConsent
		}, heartbeat.Policy{Enabled: true, Identified: true}},
		{"preselection before any answer", func(s *settings.Settings) {
			s.AnonymousUsageStats = true
		}, heartbeat.Policy{Enabled: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := settings.Defaults()
			tt.mutate(&s)
			if got := heartbeatPolicy(s); got != tt.want {
				t.Fatalf("heartbeatPolicy = %+v, want %+v", got, tt.want)
			}
		})
	}
}
