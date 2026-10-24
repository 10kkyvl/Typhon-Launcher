package crashlog

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"
)

const (
	maxEventXML   = 64 << 10
	maxNameLength = 128
)

type eventXML struct {
	System struct {
		Provider struct {
			Name string `xml:"Name,attr"`
		} `xml:"Provider"`
		EventID     int `xml:"EventID"`
		TimeCreated struct {
			SystemTime string `xml:"SystemTime,attr"`
		} `xml:"TimeCreated"`
	} `xml:"System"`
	Data []string `xml:"EventData>Data"`
}

func ParseEvent(raw []byte) (Fault, error) {
	if len(raw) > maxEventXML {
		return Fault{}, ErrEventTooLarge
	}
	var ev eventXML
	dec := xml.NewDecoder(bytes.NewReader(raw))
	if err := dec.Decode(&ev); err != nil {
		return Fault{}, fmt.Errorf("%w: %w", ErrMalformedEvent, err)
	}
	var f Fault
	switch {
	case ev.System.Provider.Name == SourceAppError && ev.System.EventID == 1000:
		if len(ev.Data) < 7 {
			return Fault{}, fmt.Errorf("%w: %d data fields", ErrMalformedEvent, len(ev.Data))
		}
		f = Fault{Source: SourceAppError, App: ev.Data[0], Module: ev.Data[3], ModuleVersion: ev.Data[4]}
		f.ExceptionCode, f.HasException = parseException(ev.Data[6])
		if len(ev.Data) > 8 {
			f.PID, f.HasPID = parseException(ev.Data[8])
		}
		if len(ev.Data) > 11 {
			f.ModulePath = strings.TrimSpace(ev.Data[11])
		}
	case ev.System.Provider.Name == SourceWER && ev.System.EventID == 1001:
		if len(ev.Data) < 3 || !strings.EqualFold(strings.TrimSpace(ev.Data[2]), "APPCRASH") {
			return Fault{}, ErrNotCrashEvent
		}
		if len(ev.Data) < 12 {
			return Fault{}, fmt.Errorf("%w: %d data fields", ErrMalformedEvent, len(ev.Data))
		}
		f = Fault{Source: SourceWER, App: ev.Data[5], Module: ev.Data[8], ModuleVersion: ev.Data[9]}
		f.ExceptionCode, f.HasException = parseException(ev.Data[11])
	default:
		return Fault{}, ErrNotCrashEvent
	}
	f.App = cleanName(f.App)
	f.Module = cleanName(f.Module)
	if strings.EqualFold(f.Module, "unknown") {
		f.Module = ""
	}
	f.ModuleVersion = cleanName(f.ModuleVersion)
	if ts := ev.System.TimeCreated.SystemTime; ts != "" {
		t, err := time.Parse(time.RFC3339Nano, ts)
		if err != nil {
			return Fault{}, fmt.Errorf("%w: time %q: %w", ErrMalformedEvent, ts, err)
		}
		f.Time = t
	}
	return f, nil
}

func parseException(s string) (uint32, bool) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(strings.TrimPrefix(s, "0x"), "0X")
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return 0, false
	}
	return uint32(v), true
}

// cleanName keeps only the file name: the value is shown in the UI and
// written to the log, and neither should carry a path or control characters.
func cleanName(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.LastIndexAny(s, `\/`); i >= 0 {
		s = s[i+1:]
	}
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
	if r := []rune(s); len(r) > maxNameLength {
		s = string(r[:maxNameLength])
	}
	return s
}
