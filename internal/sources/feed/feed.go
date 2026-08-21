package feed

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

var (
	ErrInvalidJSON        = errors.New("некорректный JSON фида")
	ErrUnsupportedVersion = errors.New("неподдерживаемая версия фида")
	ErrEmptyFeed          = errors.New("фид не содержит записей")
)

type Entry struct {
	Title      string
	URIs       []string
	UploadedAt *time.Time
	Size       int64
}

type Feed struct {
	Name        string
	Version     int
	Entries     []Entry
	Fingerprint string
	Invalid     int
	Warnings    []string
}

type rawFeed struct {
	Name      string     `json:"name"`
	Version   *int       `json:"version"`
	Downloads []rawEntry `json:"downloads"`
	Entries   []rawEntry `json:"entries"`
	Items     []rawEntry `json:"items"`
}

type rawEntry struct {
	Title      string          `json:"title"`
	URIs       []string        `json:"uris"`
	URI        string          `json:"uri"`
	Magnet     string          `json:"magnet"`
	UploadDate json.RawMessage `json:"uploadDate"`
	UploadedAt json.RawMessage `json:"uploadedAt"`
	Date       json.RawMessage `json:"date"`
	FileSize   json.RawMessage `json:"fileSize"`
}

type warningCounter struct {
	entriesTruncated bool
	invalidTitle     int
	noURI            int
	nonMagnetDropped int
	tooLongURI       int
	uriListTruncated int
	negativeSize     int
	badSize          int
	badDate          int
	duplicates       int
}

func (w warningCounter) build() []string {
	var out []string
	if w.entriesTruncated {
		out = append(out, fmt.Sprintf("фид обрезан до %d записей (превышен лимит)", MaxEntries))
	}
	if w.invalidTitle > 0 {
		out = append(out, fmt.Sprintf("%d записей пропущено из-за некорректного заголовка", w.invalidTitle))
	}
	if w.noURI > 0 {
		out = append(out, fmt.Sprintf("%d записей пропущено: нет валидного URI", w.noURI))
	}
	if w.nonMagnetDropped > 0 {
		out = append(out, fmt.Sprintf("%d URI пропущено: поддерживается только magnet", w.nonMagnetDropped))
	}
	if w.tooLongURI > 0 {
		out = append(out, fmt.Sprintf("%d URI пропущено: превышена максимальная длина", w.tooLongURI))
	}
	if w.uriListTruncated > 0 {
		out = append(out, fmt.Sprintf("%d записей: список URI обрезан до лимита %d", w.uriListTruncated, MaxURIsPerEntry))
	}
	if w.negativeSize > 0 {
		out = append(out, fmt.Sprintf("%d записей: отрицательный размер файла обнулён", w.negativeSize))
	}
	if w.badSize > 0 {
		out = append(out, fmt.Sprintf("%d записей: не удалось разобрать размер файла", w.badSize))
	}
	if w.badDate > 0 {
		out = append(out, fmt.Sprintf("%d записей: не удалось разобрать дату публикации", w.badDate))
	}
	if w.duplicates > 0 {
		out = append(out, fmt.Sprintf("%d дублирующихся записей объединено", w.duplicates))
	}
	if len(out) > MaxWarnings {
		out = out[:MaxWarnings]
	}
	return out
}

func rawURIs(re rawEntry) []string {
	if len(re.URIs) > 0 {
		return re.URIs
	}
	if re.URI != "" {
		return []string{re.URI}
	}
	if re.Magnet != "" {
		return []string{re.Magnet}
	}
	return nil
}

func firstNonEmptyRaw(candidates ...json.RawMessage) json.RawMessage {
	for _, c := range candidates {
		if len(c) > 0 {
			return c
		}
	}
	return nil
}

func Parse(data []byte) (Feed, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return Feed{}, ErrInvalidJSON
	}

	var rawEntries []rawEntry
	name := ""
	version := SupportedVersion

	switch trimmed[0] {
	case '[':
		if err := json.Unmarshal(trimmed, &rawEntries); err != nil {
			return Feed{}, ErrInvalidJSON
		}
	case '{':
		var rf rawFeed
		if err := json.Unmarshal(trimmed, &rf); err != nil {
			return Feed{}, ErrInvalidJSON
		}
		name = rf.Name
		if rf.Version != nil {
			version = *rf.Version
		}
		switch {
		case len(rf.Downloads) > 0:
			rawEntries = rf.Downloads
		case len(rf.Entries) > 0:
			rawEntries = rf.Entries
		case len(rf.Items) > 0:
			rawEntries = rf.Items
		}
	default:
		return Feed{}, ErrInvalidJSON
	}

	if version < 1 || version > SupportedVersion {
		return Feed{}, ErrUnsupportedVersion
	}

	if len(rawEntries) == 0 {
		return Feed{}, ErrEmptyFeed
	}

	var wc warningCounter
	if len(rawEntries) > MaxEntries {
		rawEntries = rawEntries[:MaxEntries]
		wc.entriesTruncated = true
	}

	entries := make([]Entry, 0, len(rawEntries))
	invalid := 0
	seen := make(map[string]bool, len(rawEntries))

	for _, re := range rawEntries {
		title := strings.Join(strings.Fields(re.Title), " ")
		titleLen := utf8.RuneCountInString(title)
		if titleLen < MinTitleLen || titleLen > MaxTitleLen {
			invalid++
			wc.invalidTitle++
			continue
		}

		candidates := rawURIs(re)
		uris := make([]string, 0, len(candidates))
		for _, u := range candidates {
			u = strings.TrimSpace(u)
			if u == "" {
				continue
			}
			if utf8.RuneCountInString(u) > MaxURILen {
				wc.tooLongURI++
				continue
			}
			if !strings.HasPrefix(strings.ToLower(u), "magnet:") {
				wc.nonMagnetDropped++
				continue
			}
			uris = append(uris, u)
		}
		if len(uris) > MaxURIsPerEntry {
			uris = uris[:MaxURIsPerEntry]
			wc.uriListTruncated++
		}
		if len(uris) == 0 {
			invalid++
			wc.noURI++
			continue
		}

		size, sizeOK := parseSize(re.FileSize)
		if !sizeOK {
			wc.badSize++
			size = 0
		} else if size < 0 {
			wc.negativeSize++
			size = 0
		}

		dateRaw := firstNonEmptyRaw(re.UploadDate, re.UploadedAt, re.Date)
		uploadedAt, dateOK := parseDate(dateRaw)
		if !dateOK {
			wc.badDate++
		}

		sortedURIs := append([]string(nil), uris...)
		sort.Strings(sortedURIs)
		key := title + "\x1f" + strings.Join(sortedURIs, "\x1f")
		if seen[key] {
			wc.duplicates++
			continue
		}
		seen[key] = true

		entries = append(entries, Entry{
			Title:      title,
			URIs:       uris,
			UploadedAt: uploadedAt,
			Size:       size,
		})
	}
	if len(entries) == 0 {
		entries = nil
	}

	feedName := strings.TrimSpace(name)
	if utf8.RuneCountInString(feedName) > 200 {
		r := []rune(feedName)
		feedName = string(r[:200])
	}

	f := Feed{
		Name:     feedName,
		Version:  version,
		Entries:  entries,
		Invalid:  invalid,
		Warnings: wc.build(),
	}
	f.Fingerprint = Fingerprint(f)

	slog.Info("feed parsed", "name", f.Name, "entries", len(f.Entries), "invalid", f.Invalid, "warnings", len(f.Warnings))

	return f, nil
}

func Fingerprint(f Feed) string {
	parts := make([]string, 0, len(f.Entries))
	for _, e := range f.Entries {
		uris := append([]string(nil), e.URIs...)
		sort.Strings(uris)
		parts = append(parts, e.Title+"\x1f"+strings.Join(uris, "\x1f")+"\x1f"+strconv.FormatInt(e.Size, 10))
	}
	sort.Strings(parts)

	h := sha256.New()
	h.Write([]byte(f.Name))
	h.Write([]byte{0})
	h.Write([]byte(strconv.Itoa(f.Version)))
	h.Write([]byte{0})
	for _, p := range parts {
		h.Write([]byte(p))
		h.Write([]byte{0})
	}
	sum := h.Sum(nil)
	return hex.EncodeToString(sum)[:32]
}

var sizePattern = regexp.MustCompile(`^([0-9]+(?:\.[0-9]+)?)\s*([A-Za-z]+)$`)

var sizeUnits = map[string]float64{
	"b": 1, "byte": 1, "bytes": 1,
	"kb": 1000, "mb": 1e6, "gb": 1e9, "tb": 1e12,
	"kib": 1024, "mib": 1024 * 1024, "gib": 1024 * 1024 * 1024, "tib": 1024 * 1024 * 1024 * 1024,
}

func parseSize(raw json.RawMessage) (int64, bool) {
	if len(raw) == 0 {
		return 0, true
	}
	trimmed := bytes.TrimSpace(raw)
	if string(trimmed) == "null" {
		return 0, true
	}
	if trimmed[0] == '"' {
		var s string
		if err := json.Unmarshal(trimmed, &s); err != nil {
			return 0, false
		}
		return parseSizeString(s)
	}
	var f float64
	if err := json.Unmarshal(trimmed, &f); err != nil {
		return 0, false
	}
	return int64(f), true
}

func parseSizeString(s string) (int64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		return n, true
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return int64(f), true
	}
	m := sizePattern.FindStringSubmatch(s)
	if m == nil {
		return 0, false
	}
	num, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return 0, false
	}
	unit, ok := sizeUnits[strings.ToLower(m[2])]
	if !ok {
		return 0, false
	}
	return int64(num * unit), true
}

var dateLayouts = []string{
	time.RFC3339,
	"2006-01-02T15:04:05",
	"2006-01-02",
}

func parseDate(raw json.RawMessage) (*time.Time, bool) {
	if len(raw) == 0 {
		return nil, true
	}
	trimmed := bytes.TrimSpace(raw)
	if string(trimmed) == "null" {
		return nil, true
	}
	if trimmed[0] == '"' {
		var s string
		if err := json.Unmarshal(trimmed, &s); err != nil {
			return nil, false
		}
		return parseDateString(s)
	}
	var f float64
	if err := json.Unmarshal(trimmed, &f); err != nil {
		return nil, false
	}
	return unixToTime(f), true
}

func unixToTime(f float64) *time.Time {
	var t time.Time
	if f > 1e12 {
		t = time.UnixMilli(int64(f)).UTC()
	} else {
		t = time.Unix(int64(f), 0).UTC()
	}
	return &t
}

func parseDateString(s string) (*time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, false
	}
	for _, layout := range dateLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			t = t.UTC()
			return &t, true
		}
	}
	if n, err := strconv.ParseFloat(s, 64); err == nil {
		return unixToTime(n), true
	}
	return nil, false
}
