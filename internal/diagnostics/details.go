package diagnostics

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	MaxContextFields        = 24
	MaxContextValueLen      = 256
	MaxBreadcrumbs          = 12
	MaxBreadcrumbMessageLen = 512
	MaxDetailsBytes         = 16 << 10
)

// Details is optional so reports from older launchers remain valid. Only
// diagnostic scalars belong here: never configuration, credentials or payloads.
type Details struct {
	Context     map[string]string `json:"context,omitempty"`
	Breadcrumbs []Breadcrumb      `json:"breadcrumbs,omitempty"`
}

type Breadcrumb struct {
	Timestamp time.Time         `json:"timestamp"`
	Level     string            `json:"level"`
	Component string            `json:"component"`
	Message   string            `json:"message"`
	Context   map[string]string `json:"context,omitempty"`
}

// Keep this allowlist aligned with the server. Unknown slog attributes are
// deliberately excluded even when their values appear harmless.
func contextKeyAllowed(key string) bool {
	switch key {
	case "source_id", "download_id", "install_id", "game_id", "operation_id", "request_id",
		"source_type", "stage", "duration_ms", "timeout_ms", "scheduled", "retry_attempt",
		"http_status", "http_timeout_ms", "bytes", "entries", "status", "type", "kind", "reason_code", "error_code",
		"attempt", "version", "exit_code", "mode", "enabled", "count", "added", "removed",
		"uptime_ms", "go_version", "goroutines", "error_type", "route":
		return true
	}
	return false
}

const breadcrumbCapacity = 64

var processStarted = time.Now()

func contextFields(attrs []slog.Attr) map[string]string {
	fields := make(map[string]string)
	var read func(slog.Attr)
	read = func(a slog.Attr) {
		a.Value = a.Value.Resolve()
		if a.Value.Kind() == slog.KindGroup {
			for _, nested := range a.Value.Group() {
				read(nested)
			}
			return
		}
		key := a.Key
		// These aliases are IDs in the affected services, not display names.
		if key == "game" {
			key = "game_id"
		}
		if !contextKeyAllowed(key) {
			return
		}
		var value string
		switch a.Value.Kind() {
		case slog.KindString:
			value = a.Value.String()
		case slog.KindBool:
			value = strconv.FormatBool(a.Value.Bool())
		case slog.KindInt64:
			value = strconv.FormatInt(a.Value.Int64(), 10)
		case slog.KindUint64:
			value = strconv.FormatUint(a.Value.Uint64(), 10)
		case slog.KindFloat64:
			value = strconv.FormatFloat(a.Value.Float64(), 'g', -1, 64)
		case slog.KindDuration:
			value = strconv.FormatInt(a.Value.Duration().Milliseconds(), 10)
		default:
			return // never stringify arbitrary objects, settings or request bodies
		}
		if len(fields) < MaxContextFields || fields[key] != "" {
			fields[key] = capText(value, 4096)
		}
	}
	for _, a := range attrs {
		read(a)
	}
	return fields
}

func cleanContext(fields map[string]string) map[string]string {
	out := make(map[string]string)
	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if !contextKeyAllowed(key) || len(out) >= MaxContextFields {
			continue
		}
		value, err := sanitizeText(strings.ToValidUTF8(fields[key], ""))
		if err != nil {
			continue
		}
		out[key] = capText(stripControl(value), MaxContextValueLen)
	}
	return out
}

func sanitizeDetails(d *Details) *Details {
	if d == nil {
		return nil
	}
	out := &Details{Context: cleanContext(d.Context)}
	for _, b := range d.Breadcrumbs {
		component, err := sanitizeText(strings.ToValidUTF8(b.Component, ""))
		if err != nil {
			continue
		}
		message, err := sanitizeText(strings.ToValidUTF8(b.Message, ""))
		if err != nil {
			continue
		}
		b.Component = capText(stripControl(component), maxComponentLen)
		b.Message = capText(stripControl(message), MaxBreadcrumbMessageLen)
		b.Context = cleanContext(b.Context)
		out.Breadcrumbs = append(out.Breadcrumbs, b)
	}
	if len(out.Breadcrumbs) > MaxBreadcrumbs {
		out.Breadcrumbs = out.Breadcrumbs[len(out.Breadcrumbs)-MaxBreadcrumbs:]
	}
	// JSON escaping can multiply the byte size, so bound the actual wire form.
	for {
		data, err := json.Marshal(out)
		if err != nil {
			return nil
		}
		if len(data) <= MaxDetailsBytes {
			return out
		}
		if len(out.Breadcrumbs) == 0 {
			return nil
		}
		out.Breadcrumbs = out.Breadcrumbs[1:]
	}
}

func (s *Service) detailsLocked(component string, at time.Time, fields map[string]string) *Details {
	d := &Details{Context: make(map[string]string)}
	for key, value := range fields {
		d.Context[key] = value
	}
	d.Context["uptime_ms"] = strconv.FormatInt(max(0, time.Since(processStarted).Milliseconds()), 10)
	d.Context["go_version"] = runtime.Version()
	d.Context["goroutines"] = strconv.Itoa(runtime.NumGoroutine())
	for _, b := range s.breadcrumbs {
		if (b.Component == component || b.Component == "launcher") && !b.Timestamp.Before(at.Add(-5*time.Minute)) && !b.Timestamp.After(at) {
			d.Breadcrumbs = append(d.Breadcrumbs, b)
		}
	}
	sort.SliceStable(d.Breadcrumbs, func(i, j int) bool { return d.Breadcrumbs[i].Timestamp.Before(d.Breadcrumbs[j].Timestamp) })
	if len(d.Breadcrumbs) > MaxBreadcrumbs {
		d.Breadcrumbs = d.Breadcrumbs[len(d.Breadcrumbs)-MaxBreadcrumbs:]
	}
	return d
}

func validDiagnosticCode(value string) bool {
	if value == "" || len(value) > 64 || !utf8.ValidString(value) {
		return false
	}
	for _, c := range value {
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '_' || c == '.') {
			return false
		}
	}
	return true
}

func errorType(err error) string {
	var types []string
	for err != nil && len(types) < 6 {
		types = append(types, fmt.Sprintf("%T", err))
		err = errors.Unwrap(err)
	}
	return strings.Join(types, " → ")
}

func errorContext(err error) map[string]string {
	fields := map[string]string{"error_type": errorType(err)}
	// Components can contribute safe facts without depending on diagnostics.
	var contextual interface{ DiagnosticContext() map[string]string }
	if errors.As(err, &contextual) {
		for key, value := range contextual.DiagnosticContext() {
			if contextKeyAllowed(key) {
				fields[key] = capText(value, 4096)
			}
		}
	}
	return fields
}
