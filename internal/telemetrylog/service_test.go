package telemetrylog

import (
	"strings"
	"testing"
)

func TestServiceListFormatsValidJSON(t *testing.T) {
	log := New(4)
	log.Record(KindDiagnostics, "/e", []byte(`{"a":1,"b":[2,3]}`))
	s := &Service{log: log}

	entries := s.List()
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}
	e := entries[0]
	if !e.Formatted {
		t.Fatalf("expected Formatted=true for valid JSON, got entry: %+v", e)
	}
	if !strings.Contains(e.Payload, "\n") {
		t.Fatalf("expected indented JSON with newlines, got %q", e.Payload)
	}
	if !strings.Contains(e.Payload, "\"a\": 1") {
		t.Fatalf("expected indented field, got %q", e.Payload)
	}
}

func TestServiceListReturnsRawOnMalformedJSON(t *testing.T) {
	log := New(4)
	raw := []byte(`{"a":1,"b":[2,3`)
	log.Record(KindDiagnostics, "/e", raw)
	s := &Service{log: log}

	entries := s.List()
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}
	e := entries[0]
	if e.Formatted {
		t.Fatalf("expected Formatted=false for malformed JSON, got entry: %+v", e)
	}
	if e.Payload != string(raw) {
		t.Fatalf("got payload %q, want raw payload %q", e.Payload, raw)
	}
}

func TestServiceListReturnsRawOnTruncatedPayload(t *testing.T) {
	log := New(4)
	big := make([]byte, maxPayloadBytes+1)
	for i := range big {
		big[i] = '{'
	}
	log.Record(KindDiagnostics, "/e", big)
	s := &Service{log: log}

	entries := s.List()
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}
	e := entries[0]
	if e.Formatted {
		t.Fatal("expected Formatted=false for a truncated payload that no longer parses as JSON")
	}
	if !strings.HasSuffix(e.Payload, string(truncationMarker)) {
		t.Fatalf("expected raw payload to retain truncation marker, got tail %q", e.Payload[len(e.Payload)-40:])
	}
}

func TestNewServiceUsesStdLog(t *testing.T) {
	s := NewService()
	if s.log != std {
		t.Fatal("NewService should back onto the package-wide std log")
	}
}
