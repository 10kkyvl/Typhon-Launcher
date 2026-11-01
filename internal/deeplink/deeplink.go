// Package deeplink reads typhon:// links. A link only ever names a game to
// show: nothing a web page can put into one reaches an install or a launch.
package deeplink

import (
	"errors"
	"net/url"
	"strings"
)

const (
	Scheme = "typhon"

	ProviderSteam = "steam"
	ProviderIGDB  = "igdb"

	hostGame = "game"

	maxLinkLen = 256
	maxIDLen   = 12
)

var (
	ErrEmpty     = errors.New("ссылка пустая")
	ErrTooLong   = errors.New("ссылка слишком длинная")
	ErrMalformed = errors.New("ссылка не разбирается как URL")
	ErrScheme    = errors.New("в ссылке другая схема")
	ErrHost      = errors.New("ссылка ведёт не на игру")
	ErrUserinfo  = errors.New("в ссылке есть данные пользователя")
	ErrPath      = errors.New("в ссылке есть лишний путь")
	ErrFragment  = errors.New("в ссылке есть фрагмент")
	ErrParams    = errors.New("в ссылке нужен ровно один параметр steam или igdb")
	ErrProvider  = errors.New("в ссылке неизвестный источник")
	ErrID        = errors.New("идентификатор в ссылке — не от 1 до 12 цифр")
	ErrZeroID    = errors.New("идентификатор в ссылке нулевой")
)

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

// Parse accepts typhon://game?steam=<appid> and typhon://game?igdb=<id>. The
// slash before the query is allowed because browsers add it to a URL with an
// empty path. The query is read by hand: url.Values would accept percent
// escapes, a plus and repeated keys that have no place in a link.
func Parse(raw string) (Target, error) {
	if raw == "" {
		return Target{}, ErrEmpty
	}
	if len(raw) > maxLinkLen {
		return Target{}, ErrTooLong
	}
	u, err := url.Parse(raw)
	if err != nil {
		// url.Error quotes the input, and a link is text from an untrusted page
		// that would end up in the log.
		return Target{}, ErrMalformed
	}
	if u.Scheme != Scheme {
		return Target{}, ErrScheme
	}
	if u.Opaque != "" || u.Host != hostGame {
		return Target{}, ErrHost
	}
	if u.User != nil {
		return Target{}, ErrUserinfo
	}
	if u.Path != "" && u.Path != "/" {
		return Target{}, ErrPath
	}
	if strings.Contains(raw, "#") {
		return Target{}, ErrFragment
	}
	provider, id, found := strings.Cut(u.RawQuery, "=")
	if !found || provider == "" || strings.ContainsAny(u.RawQuery, "&;") || strings.ContainsRune(id, '=') {
		return Target{}, ErrParams
	}
	target := Target{Provider: provider, ID: id}
	if err := target.Validate(); err != nil {
		return Target{}, err
	}
	return target, nil
}
