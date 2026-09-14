package diagnostics

import (
	"context"
	"fmt"
	"log/slog"
	"runtime"
	"runtime/debug"
	"strings"
	"time"

	"typhon/internal/usagestats"
)

type capturedLog struct {
	at                                         time.Time
	details                                    *Details
	epoch                                      uint64
	component, operation, message, stack, code string
}

// NewLogHandler preserves local logging and captures ordinary Go errors.
// Capture runs on a bounded worker: logging while holding a settings/service
// lock must never synchronously re-enter those services to read consent.
func NewLogHandler(next slog.Handler, service *Service) slog.Handler {
	return &diagnosticLogHandler{next: next, service: service}
}

type diagnosticLogHandler struct {
	next    slog.Handler
	service *Service
	attrs   []slog.Attr
}

func (h *diagnosticLogHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}
func (h *diagnosticLogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	all := append(append([]slog.Attr(nil), h.attrs...), attrs...)
	return &diagnosticLogHandler{next: h.next.WithAttrs(attrs), service: h.service, attrs: all}
}
func (h *diagnosticLogHandler) WithGroup(name string) slog.Handler {
	return &diagnosticLogHandler{next: h.next.WithGroup(name), service: h.service, attrs: h.attrs}
}
func (h *diagnosticLogHandler) Handle(ctx context.Context, r slog.Record) error {
	err := h.next.Handle(ctx, r)
	if r.Level < slog.LevelInfo {
		return err
	}
	e := capturedLog{at: r.Time, component: "launcher", operation: r.Message, message: r.Message, code: usagestats.CodeUnknown}
	if r.PC != 0 {
		// Resolve return PCs and inlined callers just like slog source attribution.
		frame, _ := runtime.CallersFrames([]uintptr{r.PC}).Next()
		if tail, ok := strings.CutPrefix(frame.Function, "typhon/internal/"); ok {
			e.component = strings.SplitN(tail, ".", 2)[0]
		}
	}
	if e.at.IsZero() {
		e.at = time.Now()
	}
	attrs := append([]slog.Attr(nil), h.attrs...)
	r.Attrs(func(a slog.Attr) bool { attrs = append(attrs, a); return true })
	fields := contextFields(attrs)
	var read func(slog.Attr)
	read = func(a slog.Attr) {
		a.Value = a.Value.Resolve()
		if a.Value.Kind() == slog.KindGroup {
			for _, nested := range a.Value.Group() {
				read(nested)
			}
			return
		}
		switch a.Key {
		case "component":
			e.component = a.Value.String()
		case "operation":
			e.operation = a.Value.String()
		case "err", "error":
			if cause, ok := a.Value.Any().(error); ok {
				e.code = diagnosticCode(cause)
				for key, value := range errorContext(cause) {
					fields[key] = value
				}
			}
			e.message = r.Message + ": " + fmt.Sprint(a.Value.Any())
		}
	}
	for _, a := range attrs {
		read(a)
	}
	if code := fields["error_code"]; validDiagnosticCode(code) {
		e.code = code
	}
	// Delivery failures must not recursively generate their own deliveries.
	if e.component == "diagnostics" || strings.HasPrefix(r.Message, "diagnostics") {
		return err
	}
	e.message = capText(e.message, 16<<10)
	if r.Level >= slog.LevelError {
		e.stack = capText(string(debug.Stack()), 16<<10)
	}

	h.service.mu.Lock()
	defer h.service.mu.Unlock()
	if h.service.disabled {
		return err
	}
	e.epoch = h.service.consentEpoch
	if r.Level >= slog.LevelError {
		e.details = h.service.detailsLocked(e.component, e.at, fields)
		select {
		case h.service.logEvents <- e:
		default:
		}
	}
	// Store only a bounded in-memory window. No settings callbacks or filesystem
	// work is allowed here: callers may be logging while holding service locks.
	level := "INFO"
	if r.Level >= slog.LevelError {
		level = "ERROR"
	} else if r.Level >= slog.LevelWarn {
		level = "WARN"
	}
	if len(h.service.breadcrumbs) >= breadcrumbCapacity {
		copy(h.service.breadcrumbs, h.service.breadcrumbs[1:])
		h.service.breadcrumbs = h.service.breadcrumbs[:breadcrumbCapacity-1]
	}
	h.service.breadcrumbs = append(h.service.breadcrumbs, Breadcrumb{Timestamp: e.at, Level: level, Component: e.component, Message: e.message, Context: fields})

	return err
}

func (s *Service) captureLogs(ctx context.Context) {
	defer s.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case e := <-s.logEvents:
			if s.consentCurrent(e.epoch) {
				s.captureEvent(e, false)
			}
		}
	}
}
