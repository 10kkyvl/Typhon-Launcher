// Package deeplink reads typhon:// links. A link only ever names a game to show
// or a title to search the catalog for: nothing a web page can put into one
// reaches an install or a launch.
package deeplink

import (
	"errors"
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	Scheme = "typhon"

	ProviderSteam = "steam"
	ProviderIGDB  = "igdb"

	hostGame   = "game"
	hostSearch = "search"

	maxLinkLen    = 256
	maxIDLen      = 12
	maxQueryRunes = 120
	// typhon-backend refuses a longer query in bytes: maxSearchQueryLen in
	// internal/httpapi/metadata.go and the Search check in catalog/browse.go.
	maxQueryBytes = 200

	// The longest query has every byte written as %XX.
	maxSearchLinkLen = len("typhon://search/?q=") + maxQueryBytes*3
)

var (
	ErrEmpty        = errors.New("ссылка пустая")
	ErrTooLong      = errors.New("ссылка слишком длинная")
	ErrMalformed    = errors.New("ссылка не разбирается как URL")
	ErrScheme       = errors.New("в ссылке другая схема")
	ErrHost         = errors.New("ссылка ведёт не на игру")
	ErrUserinfo     = errors.New("в ссылке есть данные пользователя")
	ErrPath         = errors.New("в ссылке есть лишний путь")
	ErrFragment     = errors.New("в ссылке есть фрагмент")
	ErrParams       = errors.New("в ссылке нужен ровно один параметр steam или igdb")
	ErrProvider     = errors.New("в ссылке неизвестный источник")
	ErrID           = errors.New("идентификатор в ссылке — не от 1 до 12 цифр")
	ErrZeroID       = errors.New("идентификатор в ссылке нулевой")
	ErrSearchParams = errors.New("в ссылке поиска нужен ровно один параметр q")
	ErrQueryEscape  = errors.New("запрос в ссылке закодирован неверно")
	ErrQueryUTF8    = errors.New("запрос в ссылке — не UTF-8")
	ErrQueryControl = errors.New("в запросе из ссылки есть управляющие символы")
	ErrQueryEmpty   = errors.New("запрос в ссылке пустой")
	ErrQueryTooLong = errors.New("запрос в ссылке слишком длинный")
)

// Link is what a typhon:// link asks for: a game in Target or, when Query is
// set, text to look up by title. Neither starts an install or a launch.
type Link struct {
	Target Target
	Query  string
}

// Target is the game a link names. ID is in canonical form: digits without
// leading zeros, the spelling the catalog indexes it under.
type Target struct {
	Provider string `json:"provider"`
	ID       string `json:"id"`
}

func (t Target) Validate() error {
	if t.Provider != ProviderSteam && t.Provider != ProviderIGDB {
		return ErrProvider
	}
	return validateID(t.ID)
}

// providerID is the identifier the metadata API takes for this target.
func (t Target) providerID() string {
	if t.Provider == ProviderSteam {
		return "steam:" + t.ID
	}
	return t.ID
}

func validateID(id string) error {
	if id == "" || len(id) > maxIDLen {
		return ErrID
	}
	for i := range len(id) {
		if id[i] < '0' || id[i] > '9' {
			return ErrID
		}
	}
	if strings.Trim(id, "0") == "" {
		return ErrZeroID
	}
	if id[0] == '0' {
		return ErrID
	}
	return nil
}

// IsLink tells a link from any other launch argument. The scheme of a URL is
// case-insensitive, so a malformed TYPHON: link still counts as an attempt.
func IsLink(arg string) bool {
	prefix := Scheme + ":"
	return len(arg) >= len(prefix) && strings.EqualFold(arg[:len(prefix)], prefix)
}

// Parse accepts typhon://game?steam=<appid>, typhon://game?igdb=<id> and
// typhon://search?q=<percent-encoded UTF-8>. The slash before the query is
// allowed because browsers add it to a URL with an empty path. The query is read
// by hand: url.Values would accept percent escapes in a game link, a plus and
// repeated keys that have no place in a link.
func Parse(raw string) (Link, error) {
	if raw == "" {
		return Link{}, ErrEmpty
	}
	if len(raw) > maxSearchLinkLen {
		return Link{}, ErrTooLong
	}
	u, err := url.Parse(raw)
	if err != nil {
		// url.Error quotes the input, and a link is text from an untrusted page
		// that would end up in the log.
		return Link{}, ErrMalformed
	}
	if u.Scheme != Scheme {
		return Link{}, ErrScheme
	}
	if u.Opaque != "" || (u.Host != hostGame && u.Host != hostSearch) {
		return Link{}, ErrHost
	}
	if u.Host == hostGame && len(raw) > maxLinkLen {
		return Link{}, ErrTooLong
	}
	if u.User != nil {
		return Link{}, ErrUserinfo
	}
	if u.Path != "" && u.Path != "/" {
		return Link{}, ErrPath
	}
	if strings.Contains(raw, "#") {
		return Link{}, ErrFragment
	}
	if u.Host == hostSearch {
		query, err := parseQuery(u.RawQuery)
		if err != nil {
			return Link{}, err
		}
		return Link{Query: query}, nil
	}
	provider, id, found := strings.Cut(u.RawQuery, "=")
	if !found || provider == "" || strings.ContainsAny(u.RawQuery, "&;") || strings.ContainsRune(id, '=') {
		return Link{}, ErrParams
	}
	target := Target{Provider: provider, ID: id}
	if err := target.Validate(); err != nil {
		return Link{}, err
	}
	return Link{Target: target}, nil
}

// parseQuery reads the one q parameter of a search link. Like the game query it
// is read by hand: url.ParseQuery would turn a plus into a space and accept
// repeated keys. Its errors never wrap url.EscapeError, which quotes the input.
func parseQuery(rawQuery string) (string, error) {
	key, value, found := strings.Cut(rawQuery, "=")
	if !found || key != "q" || strings.ContainsAny(value, "&;=") {
		return "", ErrSearchParams
	}
	for i := range len(value) {
		if !isQueryByte(value[i]) {
			return "", ErrQueryEscape
		}
	}
	decoded, err := url.PathUnescape(value)
	if err != nil {
		return "", ErrQueryEscape
	}
	return validateQuery(decoded)
}

// validateQuery checks decoded search text and returns it trimmed.
func validateQuery(decoded string) (string, error) {
	if !utf8.ValidString(decoded) {
		return "", ErrQueryUTF8
	}
	for _, r := range decoded {
		if unicode.IsControl(r) || unicode.In(r, unicode.Zl, unicode.Zp, unicode.Bidi_Control) {
			return "", ErrQueryControl
		}
	}
	query := strings.TrimSpace(decoded)
	if !strings.ContainsFunc(query, isVisible) {
		return "", ErrQueryEmpty
	}
	if len(query) > maxQueryBytes || utf8.RuneCountInString(query) > maxQueryRunes {
		return "", ErrQueryTooLong
	}
	return query, nil
}

// isVisible tells a character that shows from one that only joins or shapes its
// neighbours: a zero width joiner is fine inside an emoji, not as a whole query.
func isVisible(r rune) bool {
	return !unicode.IsSpace(r) && !unicode.In(r, unicode.Cf, unicode.Variation_Selector, unicode.Other_Default_Ignorable_Code_Point)
}

// isQueryByte is what RFC 3986 allows unescaped in a query. Anything else, a
// raw quote above all, must arrive percent-encoded.
func isQueryByte(c byte) bool {
	switch {
	case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		return true
	}
	return strings.IndexByte("-._~!$'()*+,:@/?%", c) >= 0
}
