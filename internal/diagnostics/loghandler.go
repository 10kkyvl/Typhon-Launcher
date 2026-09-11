package diagnostics

import (
	"context"
	"fmt"
	"log/slog"
	"runtime"
	"runtime/debug"
	"strings"

	"typhon/internal/usagestats"
)

type capturedLog struct {
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
	if r.Level < slog.LevelError {
		return err
	}
	e := capturedLog{component: "launcher", operation: r.Message, message: r.Message, code: usagestats.CodeUnknown}
	if fn := runtime.FuncForPC(r.PC); fn != nil {
		if tail, ok := strings.CutPrefix(fn.Name(), "typhon/internal/"); ok {
			e.component = strings.SplitN(tail, ".", 2)[0]
		}
	}
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
			}
			e.message = r.Message + ": " + fmt.Sprint(a.Value.Any())
		}
	}
	for _, a := range h.attrs {
		read(a)
	}
	r.Attrs(func(a slog.Attr) bool { read(a); return true })
	// Delivery failures must not recursively generate their own deliveries.
	if e.component == "diagnostics" || strings.HasPrefix(r.Message, "diagnostics") {
		return err
	}
	e.message = capText(e.message, 16<<10)
	e.stack = capText(string(debug.Stack()), 16<<10)
	h.service.mu.Lock()
	e.epoch = h.service.consentEpoch
	if !h.service.disabled {
		select {
		case h.service.logEvents <- e:
		default:
		}
	}
	h.service.mu.Unlock()
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
				s.capture(e.component, e.operation, e.message, e.stack, e.code, false)
			}
		}
	}
}
