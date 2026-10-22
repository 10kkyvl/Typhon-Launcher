package achievements

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

const (
	maxFileBytes = 1 << 20
	maxKeyLen    = 128

	minUnix        = 946684800  // 2000-01-01: anything earlier is a counter, not a moment
	maxUnix        = 4102444800 // 2100-01-01
	millisBoundary = 100_000_000_000
)

var iniServiceSections = map[string]struct{}{
	"steamachievements": {},
	"steam":             {},
}

func validKey(key string) bool {
	if key == "" || len(key) > maxKeyLen || strings.Contains(key, "..") {
		return false
	}
	for _, r := range key {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '.', r == '-':
		default:
			return false
		}
	}
	return true
}

func readLimited(r io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxFileBytes+1))
	if err != nil {
		return nil, fmt.Errorf("чтение файла прогресса: %w", err)
	}
	if len(data) > maxFileBytes {
		return nil, ErrFileTooBig
	}
	return bytes.TrimPrefix(data, []byte("\xef\xbb\xbf")), nil
}

func mergeKey(dst map[string]time.Time, key string, at time.Time) {
	prev, ok := dst[key]
	switch {
	case !ok:
		dst[key] = at
	case at.IsZero():
	case prev.IsZero() || at.Before(prev):
		dst[key] = at
	}
}

func mergeUnlocks(dst, src map[string]time.Time) {
	for key, at := range src {
		mergeKey(dst, key, at)
	}
}

func unixTime(v int64) time.Time {
	if v >= millisBoundary {
		v /= 1000
	}
	if v < minUnix || v > maxUnix {
		return time.Time{}
	}
	return time.Unix(v, 0).UTC()
}

// ParseJSON reads a progress file shaped as {"KEY": {"earned": true,
// "earned_time": 1700000000}}. A top-level array is a list of definitions, not
// progress, and is reported as ErrNotProgress.
func ParseJSON(r io.Reader) (map[string]time.Time, error) {
	data, err := readLimited(r)
	if err != nil {
		return nil, err
	}
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return nil, fmt.Errorf("%w: пустой файл", ErrBadJSON)
	}
	if data[0] == '[' {
		if !json.Valid(data) {
			return nil, fmt.Errorf("%w: оборванный массив", ErrBadJSON)
		}
		return nil, ErrNotProgress
	}
	var entries map[string]json.RawMessage
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrBadJSON, err)
	}
	out := make(map[string]time.Time, len(entries))
	for key, raw := range entries {
		if !validKey(key) {
			continue
		}
		var fields map[string]json.RawMessage
		if json.Unmarshal(raw, &fields) != nil {
			continue // a value that is not an object carries no progress of this shape
		}
		lower := make(map[string]json.RawMessage, len(fields))
		for name, value := range fields {
			lower[strings.ToLower(name)] = value
		}
		if !jsonUnlocked(lower) {
			continue
		}
		mergeKey(out, key, jsonTime(lower))
	}
	return out, nil
}

func jsonUnlocked(fields map[string]json.RawMessage) bool {
	for _, name := range []string{"earned", "unlocked", "achieved"} {
		if raw, ok := fields[name]; ok && rawFlag(raw) {
			return true
		}
	}
	return false
}

func rawFlag(raw json.RawMessage) bool {
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return parseFlag(text)
	}
	var flag bool
	if json.Unmarshal(raw, &flag) == nil {
		return flag
	}
	var num float64
	if json.Unmarshal(raw, &num) == nil {
		return num == 1
	}
	return false
}

func jsonTime(fields map[string]json.RawMessage) time.Time {
	for _, name := range []string{"earned_time", "unlock_time", "unlocktime", "time"} {
		raw, ok := fields[name]
		if !ok {
			continue
		}
		var num float64
		if json.Unmarshal(raw, &num) == nil {
			if at := unixTime(int64(num)); !at.IsZero() {
				return at
			}
			continue
		}
		var text string
		if json.Unmarshal(raw, &text) == nil {
			if at := parseTime(text); !at.IsZero() {
				return at
			}
		}
	}
	return time.Time{}
}

func parseTime(value string) time.Time {
	n, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil {
		return time.Time{}
	}
	return unixTime(n)
}

// parseFlag reads "1", "true" and the packed form some writers use for the
// state: hex bytes whose first byte has its lowest bit set when unlocked.
func parseFlag(value string) bool {
	v := strings.ToLower(strings.Trim(strings.TrimSpace(value), `"'`))
	switch v {
	case "1", "true", "yes":
		return true
	case "", "0", "false", "no":
		return false
	}
	if len(v) < 2 || len(v)%2 != 0 {
		return false
	}
	first, err := hex.DecodeString(v[:2])
	if err != nil {
		return false
	}
	return first[0]&1 == 1
}

// ParseINI reads progress shaped as [KEY] sections with an unlocked flag and an
// optional unlock time. Sections that hold the layer's own settings and keys
// the format does not define are skipped.
func ParseINI(r io.Reader) (map[string]time.Time, error) {
	data, err := readLimited(r)
	if err != nil {
		return nil, err
	}
	out := map[string]time.Time{}
	var cur *iniSection
	flush := func() {
		if cur != nil && cur.unlocked {
			mergeKey(out, cur.name, cur.time())
		}
		cur = nil
	}
	for n, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line[0] == ';' || line[0] == '#' {
			continue
		}
		if line[0] == '[' {
			end := strings.IndexByte(line, ']')
			if end < 0 {
				return nil, fmt.Errorf("%w: строка %d: секция не закрыта", ErrBadINI, n+1)
			}
			flush()
			name := strings.TrimSpace(line[1:end])
			if _, service := iniServiceSections[strings.ToLower(name)]; service || !validKey(name) {
				continue
			}
			cur = &iniSection{name: name, values: map[string]string{}}
			continue
		}
		name, value, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("%w: строка %d: нет знака равенства", ErrBadINI, n+1)
		}
		if cur == nil {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(name))
		switch key {
		case "achieved", "unlocked", "state":
			if parseFlag(value) {
				cur.unlocked = true
			}
		case "unlocktime", "time", "curprogress":
			cur.values[key] = value
		}
	}
	flush()
	return out, nil
}

type iniSection struct {
	name     string
	unlocked bool
	values   map[string]string
}

func (s *iniSection) time() time.Time {
	for _, key := range []string{"unlocktime", "time", "curprogress"} {
		if at := parseTime(s.values[key]); !at.IsZero() {
			return at
		}
	}
	return time.Time{}
}
