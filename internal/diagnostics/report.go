package diagnostics

import "time"

// Report is a single sanitized diagnostics event ready to leave the
// process. It never carries account identity; InstallationID and SessionID
// travel alongside it in reportPayload, the same split usagestats uses
// between Event and batchPayload.
type Report struct {
	ErrorID    string
	AppVersion string
	OS         string
	Arch       string
	Component  string
	Operation  string
	ErrorCode  string
	Message    string
	Stack      string
	Timestamp  time.Time
	Fatal      bool
}

// The wire shape mirrors the telemetry batch: identity is carried once on the
// envelope, not repeated on every report. The fingerprint is deliberately not
// sent — the server recomputes it and refuses unknown fields, so a
// client-supplied one would both be ignored and reject the whole batch.
type reportPayload struct {
	ErrorID    string    `json:"error_id"`
	AppVersion string    `json:"app_version"`
	OS         string    `json:"os"`
	Arch       string    `json:"arch"`
	Component  string    `json:"component"`
	Operation  string    `json:"operation"`
	ErrorCode  string    `json:"error_code"`
	Message    string    `json:"message"`
	Stack      string    `json:"stack"`
	Timestamp  time.Time `json:"timestamp"`
	Fatal      bool      `json:"fatal"`
}

type batchPayload struct {
	InstallationID string          `json:"installation_id"`
	SessionID      string          `json:"session_id"`
	AppVersion     string          `json:"app_version"`
	OS             string          `json:"os"`
	Arch           string          `json:"arch"`
	Reports        []reportPayload `json:"reports"`
}

// The conversion is the point: it compiles only while the domain report and
// the wire report carry the same fields, so adding a field to Report that has
// no business leaving the machine breaks the build instead of shipping.
func toPayload(r Report) reportPayload {
	return reportPayload(r)
}
