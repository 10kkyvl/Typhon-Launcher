package feed

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"typhon/internal/uierr"
)

var (
	ErrInvalidJSON        = uierr.New("sources.feed_invalid_json", "некорректный JSON фида")
	ErrUnsupportedVersion = uierr.New("sources.feed_unsupported_version", "неподдерживаемая версия фида")
	ErrEmptyFeed          = uierr.New("sources.feed_empty", "фид не содержит записей")
)

type Entry struct {
	Title       string
	Game        string
	Type        string
	FromVersion string
	ToVersion   string
	Sequence    int
	URIs        []string
	UploadedAt  *time.Time
	Size        int64
	SizeUnknown bool
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
	Title       string `json:"title"`
	Game        string `json:"game"`
	Type        string `json:"type"`
	FromVersion string `json:"fromVersion"`
	ToVersion   string `json:"toVersion"`
	Sequence    *int   `json:"sequence"`
	Order       *int   `json:"order"`

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
	httpOnlyDropped  int
	tooLongURI       int
	uriListTruncated int
	negativeSize     int
	badSize          int
	badDate          int
	duplicates       int
	badPatch         int
	unknownType      int
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
	if w.httpOnlyDropped > 0 {
		out = append(out, fmt.Sprintf("%d раздач доступны только по прямой ссылке — прямые ссылки пока не поддерживаются", w.httpOnlyDropped))
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
	if w.badPatch > 0 {
		out = append(out, fmt.Sprintf("%d патчей пропущено: не указаны fromVersion/toVersion", w.badPatch))
	}
	if w.unknownType > 0 {
		out = append(out, fmt.Sprintf("%d записей с неизвестным типом обработано как обычный релиз", w.unknownType))
	}
	if len(out) > MaxWarnings {
		out = out[:MaxWarnings]
	}
	return out
}

func normalizeType(raw string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", TypeRelease:
		return TypeRelease, true
	case TypePatch, "update":
		return TypePatch, true
	default:
		return TypeRelease, false
	}
}

func trimVersion(raw string) string {
	v := strings.Join(strings.Fields(raw), " ")
	if utf8.RuneCountInString(v) > MaxVersionLen {
		return ""
	}
	return v
}

func trimGame(raw string) string {
	name := strings.Join(strings.Fields(raw), " ")
	if utf8.RuneCountInString(name) > MaxTitleLen {
		return ""
	}
	return name
}

func sequenceOf(re rawEntry) int {
	switch {
	case re.Sequence != nil:
		return *re.Sequence
	case re.Order != nil:
		return *re.Order
	default:
		return 0
	}
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
		consideredURIs := 0
		httpURIs := 0
		for _, u := range candidates {
			u = strings.TrimSpace(u)
			if u == "" {
				continue
			}
			if utf8.RuneCountInString(u) > MaxURILen {
				wc.tooLongURI++
				continue
			}
			consideredURIs++
			lower := strings.ToLower(u)
			if !strings.HasPrefix(lower, "magnet:") {
				wc.nonMagnetDropped++
				if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
					httpURIs++
				}
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
			if consideredURIs > 0 && httpURIs == consideredURIs {
				wc.httpOnlyDropped++
			} else {
				wc.noURI++
			}
			continue
		}

		size, sState := parseSize(re.FileSize)
		unknownSize := false
		switch sState {
		case sizeBad:
			wc.badSize++
			size = 0
		case sizeUnknown:
			unknownSize = true
			size = 0
		default:
			if size < 0 {
				wc.negativeSize++
				size = 0
			}
		}

		dateRaw := firstNonEmptyRaw(re.UploadDate, re.UploadedAt, re.Date)
		uploadedAt, dateOK := parseDate(dateRaw)
		if !dateOK {
			wc.badDate++
		}

		entryType, known := normalizeType(re.Type)
		if !known {
			wc.unknownType++
		}
		from, to := trimVersion(re.FromVersion), trimVersion(re.ToVersion)
		if entryType == TypePatch && (from == "" || to == "") {
			invalid++
			wc.badPatch++
			continue
		}

		sortedURIs := append([]string(nil), uris...)
		sort.Strings(sortedURIs)
		key := entryType + "\x1f" + title + "\x1f" + strings.Join(sortedURIs, "\x1f")
		if seen[key] {
			wc.duplicates++
			continue
		}
		seen[key] = true

		entries = append(entries, Entry{
			Title:       title,
			Game:        trimGame(re.Game),
			Type:        entryType,
			FromVersion: from,
			ToVersion:   to,
			Sequence:    sequenceOf(re),
			URIs:        uris,
			UploadedAt:  uploadedAt,
			Size:        size,
			SizeUnknown: unknownSize,
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
		parts = append(parts, strings.Join([]string{
			e.Title,
			e.Game,
			e.Type,
			e.FromVersion,
			e.ToVersion,
			strconv.Itoa(e.Sequence),
			strings.Join(uris, "\x1e"),
			strconv.FormatInt(e.Size, 10),
			strconv.FormatBool(e.SizeUnknown),
		}, "\x1f"))
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

type sizeState int

const (
	sizeOK sizeState = iota
	sizeUnknown
	sizeBad
)

var sizeUnits = map[string]float64{
	"b": 1, "byte": 1, "bytes": 1,
	"kb": 1000, "mb": 1e6, "gb": 1e9, "tb": 1e12,
	"kib": 1024, "mib": 1024 * 1024, "gib": 1024 * 1024 * 1024, "tib": 1024 * 1024 * 1024 * 1024,
	"б": 1, "кб": 1000, "мб": 1e6, "гб": 1e9, "тб": 1e12,
}

var sizeUnknownTokens = map[string]bool{
	"":           true,
	"-":          true,
	"—":          true,
	"?":          true,
	"n/a":        true,
	"na":         true,
	"unknown":    true,
	"неизвестно": true,
	"null":       true,
}

var sizePrefixes = []string{"~", "≈", ">", "<", "approx.", "starting from", "from", "от"}

func stripSizePrefix(s string) string {
	s = strings.TrimSpace(s)
	for {
		stripped := false
		lower := strings.ToLower(s)
		for _, p := range sizePrefixes {
			if strings.HasPrefix(lower, strings.ToLower(p)) {
				s = strings.TrimSpace(s[len(p):])
				stripped = true
				break
			}
		}
		if !stripped {
			return s
		}
	}
}

func splitSizePair(s string) (string, string, bool) {
	if strings.Count(s, "/") != 1 {
		return "", "", false
	}
	idx := strings.IndexByte(s, '/')
	left := strings.TrimSpace(s[:idx])
	right := strings.TrimSpace(s[idx+1:])
	if left == "" || right == "" {
		return "", "", false
	}
	return left, right, true
}

func splitNumberUnit(s string) (string, string, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", "", false
	}
	runes := []rune(s)
	i := 0
	for i < len(runes) {
		r := runes[i]
		if (r >= '0' && r <= '9') || r == '.' || r == ',' || r == ' ' {
			i++
			continue
		}
		break
	}
	numPart := strings.TrimSpace(string(runes[:i]))
	unitPart := strings.TrimSpace(strings.TrimSuffix(string(runes[i:]), "."))
	if numPart == "" {
		return "", "", false
	}
	return numPart, unitPart, true
}

func normalizeSizeNumber(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}
	hasDot := strings.Contains(raw, ".")
	runes := []rune(raw)
	var b strings.Builder
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		switch {
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '.':
			b.WriteRune(r)
		case r == ',' || r == ' ':
			j := i + 1
			for j < len(runes) && runes[j] >= '0' && runes[j] <= '9' {
				j++
			}
			digitsFollowing := j - (i + 1)
			switch {
			case digitsFollowing == 3 && !hasDot:
			case r == ',' && digitsFollowing >= 1 && digitsFollowing <= 2:
				b.WriteByte('.')
			default:
				return "", false
			}
		default:
			return "", false
		}
	}
	out := b.String()
	if out == "" {
		return "", false
	}
	return out, true
}

func parseSizeComponent(s string) (float64, string, bool) {
	numPart, unitPart, ok := splitNumberUnit(s)
	if !ok {
		return 0, "", false
	}
	norm, ok := normalizeSizeNumber(numPart)
	if !ok {
		return 0, "", false
	}
	val, err := strconv.ParseFloat(norm, 64)
	if err != nil {
		return 0, "", false
	}
	return val, strings.ToLower(unitPart), true
}

func parseSizeAtom(s string) (int64, sizeState) {
	num, unit, ok := parseSizeComponent(s)
	if !ok {
		return 0, sizeBad
	}
	if unit == "" {
		return int64(num), sizeOK
	}
	mult, ok := sizeUnits[unit]
	if !ok {
		return 0, sizeBad
	}
	return int64(num * mult), sizeOK
}

func parseSizePair(left, right string) (int64, sizeState) {
	lnum, lunit, lok := parseSizeComponent(left)
	rnum, runit, rok := parseSizeComponent(right)
	if !lok && !rok {
		return 0, sizeBad
	}

	var leftBytes, rightBytes int64
	haveLeft, haveRight := false, false

	if lok && lunit != "" {
		mult, ok := sizeUnits[lunit]
		if !ok {
			return 0, sizeBad
		}
		leftBytes = int64(lnum * mult)
		haveLeft = true
	}
	if rok && runit != "" {
		mult, ok := sizeUnits[runit]
		if !ok {
			return 0, sizeBad
		}
		rightBytes = int64(rnum * mult)
		haveRight = true
	}
	if !haveLeft && lok && runit != "" {
		leftBytes = int64(lnum * sizeUnits[runit])
		haveLeft = true
	}
	if !haveRight && rok && lunit != "" {
		rightBytes = int64(rnum * sizeUnits[lunit])
		haveRight = true
	}

	switch {
	case haveLeft && haveRight:
		if leftBytes > rightBytes {
			return leftBytes, sizeOK
		}
		return rightBytes, sizeOK
	case haveLeft:
		return leftBytes, sizeOK
	case haveRight:
		return rightBytes, sizeOK
	default:
		return 0, sizeBad
	}
}

func parseSizeString(s string) (int64, sizeState) {
	s = strings.TrimSpace(s)
	s = stripSizePrefix(s)
	if sizeUnknownTokens[strings.ToLower(s)] {
		return 0, sizeUnknown
	}
	if left, right, ok := splitSizePair(s); ok {
		return parseSizePair(left, right)
	}
	return parseSizeAtom(s)
}

func parseSize(raw json.RawMessage) (int64, sizeState) {
	if len(raw) == 0 {
		return 0, sizeUnknown
	}
	trimmed := bytes.TrimSpace(raw)
	if string(trimmed) == "null" {
		return 0, sizeUnknown
	}
	if trimmed[0] == '"' {
		var s string
		if err := json.Unmarshal(trimmed, &s); err != nil {
			return 0, sizeBad
		}
		return parseSizeString(s)
	}
	var f float64
	if err := json.Unmarshal(trimmed, &f); err != nil {
		return 0, sizeBad
	}
	return int64(f), sizeOK
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
