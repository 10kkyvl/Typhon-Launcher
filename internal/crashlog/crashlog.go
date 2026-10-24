package crashlog

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

var ErrUnsupported = errors.New("crash journal is not available on this platform")

var (
	ErrNoExe          = errors.New("crash query has no executable name")
	ErrEventTooLarge  = errors.New("crash event XML is too large")
	ErrNotCrashEvent  = errors.New("event is not an application crash")
	ErrMalformedEvent = errors.New("crash event is malformed")
)

const (
	SourceAppError = "Application Error"
	SourceWER      = "Windows Error Reporting"
)

type Fault struct {
	Source        string
	App           string
	Module        string
	ModuleVersion string
	ModulePath    string
	ExceptionCode uint32
	HasException  bool
	Time          time.Time
}

type Query struct {
	Exe  string
	From time.Time
	To   time.Time
}

type Reader func(ctx context.Context, q Query) ([]Fault, error)

type Clock interface {
	Now() time.Time
	After(d time.Duration) <-chan time.Time
}

type Options struct {
	Interval time.Duration
	MaxWait  time.Duration
}

var DefaultOptions = Options{Interval: time.Second, MaxWait: 5 * time.Second}

type systemClock struct{}

func (systemClock) Now() time.Time                         { return time.Now() }
func (systemClock) After(d time.Duration) <-chan time.Time { return time.After(d) }

func Lookup(ctx context.Context, q Query, opt Options) (Fault, bool, error) {
	return Await(ctx, read, systemClock{}, q, opt)
}

// Await polls because WER writes its event some seconds after the process
// dies; a single query right after the exit would miss it.
func Await(ctx context.Context, src Reader, clock Clock, q Query, opt Options) (Fault, bool, error) {
	if strings.TrimSpace(q.Exe) == "" {
		return Fault{}, false, ErrNoExe
	}
	start := clock.Now()
	var lastErr error
	for {
		if err := ctx.Err(); err != nil {
			return Fault{}, false, fmt.Errorf("wait for crash event: %w", err)
		}
		faults, err := src(ctx, q)
		if f, ok := Pick(faults, q.Exe); ok {
			return f, true, nil
		}
		if err != nil {
			if errors.Is(err, ErrUnsupported) {
				return Fault{}, false, err
			}
			lastErr = err
		}
		if clock.Now().Sub(start)+opt.Interval > opt.MaxWait {
			return Fault{}, false, lastErr
		}
		select {
		case <-ctx.Done():
			return Fault{}, false, fmt.Errorf("wait for crash event: %w", ctx.Err())
		case <-clock.After(opt.Interval):
		}
	}
}

// Pick prefers Application Error over WER: it carries the module path, which
// the antivirus check needs. Within a source the latest event wins.
func Pick(faults []Fault, exe string) (Fault, bool) {
	var best Fault
	found := false
	for _, f := range faults {
		if !strings.EqualFold(f.App, exe) {
			continue
		}
		if !found || rank(f) > rank(best) || (rank(f) == rank(best) && f.Time.After(best.Time)) {
			best = f
			found = true
		}
	}
	return best, found
}

func rank(f Fault) int {
	if f.Source == SourceAppError {
		return 1
	}
	return 0
}
