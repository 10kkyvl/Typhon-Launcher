package telemetrylog

import (
	"bytes"
	"encoding/json"
	"time"
)

// EntryView is Entry with the payload rendered for display: pretty-printed
// JSON when the payload parses as JSON, or the raw bytes as a string
// otherwise (Formatted reports which happened, so a caller can tell a
// clipped or malformed payload from a normally formatted one instead of
// silently seeing an unindented body).
type EntryView struct {
	Kind      Kind
	Endpoint  string
	SentAt    time.Time
	Payload   string
	Formatted bool
}

// Service exposes the process-wide telemetry log to the frontend.
type Service struct {
	log *Log
}

// NewService builds a Service backed by the process-wide telemetry log.
func NewService() *Service {
	return &Service{log: std}
}

// List returns the buffered entries, newest first, with payloads
// formatted for display.
func (s *Service) List() []EntryView {
	entries := s.log.Entries()
	out := make([]EntryView, len(entries))
	for i, e := range entries {
		out[i] = formatEntry(e)
	}
	return out
}

func formatEntry(e Entry) EntryView {
	var buf bytes.Buffer
	if err := json.Indent(&buf, e.Payload, "", "  "); err != nil {
		return EntryView{
			Kind:      e.Kind,
			Endpoint:  e.Endpoint,
			SentAt:    e.SentAt,
			Payload:   string(e.Payload),
			Formatted: false,
		}
	}
	return EntryView{
		Kind:      e.Kind,
		Endpoint:  e.Endpoint,
		SentAt:    e.SentAt,
		Payload:   buf.String(),
		Formatted: true,
	}
}
