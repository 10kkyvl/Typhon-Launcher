// Package jumplist keeps the taskbar Jump List of the launcher current: the
// recently played games and two shortcuts into the window. Building the
// entries is plain data; talking to the shell is Windows-only.
package jumplist

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"unicode"
)

const (
	maxAppIDLen = 128

	RouteLibrary   = "library"
	RouteDownloads = "downloads"
)

var (
	ErrNoAppID      = errors.New("jumplist: пустой AppUserModelID")
	ErrBadAppID     = errors.New("jumplist: недопустимый AppUserModelID")
	ErrNoLauncher   = errors.New("jumplist: путь к лаунчеру пуст")
	ErrRelLauncher  = errors.New("jumplist: путь к лаунчеру должен быть абсолютным")
	ErrBadGameID    = errors.New("jumplist: идентификатор игры нельзя передать в командной строке")
	ErrUnsupported  = errors.New("jumplist: Jump List есть только в Windows")
	errNoCategory   = errors.New("jumplist: нет названия категории недавних игр")
	errNoTaskLabels = errors.New("jumplist: нет названий задач")
)

type Game struct {
	ID         string
	Title      string
	Executable string
}

type Labels struct {
	Recent    string
	Library   string
	Downloads string
}

type Config struct {
	AppID    string
	Launcher string
	Labels   Labels
}

type Entry struct {
	Title string
	Args  string
	Icon  string
}

func (c Config) validate() error {
	if c.AppID == "" {
		return ErrNoAppID
	}
	if len(c.AppID) > maxAppIDLen || strings.ContainsFunc(c.AppID, unicode.IsSpace) {
		return fmt.Errorf("%w: %q", ErrBadAppID, c.AppID)
	}
	if c.Launcher == "" {
		return ErrNoLauncher
	}
	if !filepath.IsAbs(c.Launcher) {
		return fmt.Errorf("%w: %q", ErrRelLauncher, c.Launcher)
	}
	if c.Labels.Recent == "" {
		return errNoCategory
	}
	if c.Labels.Library == "" || c.Labels.Downloads == "" {
		return errNoTaskLabels
	}
	return nil
}

// PlayArgs is the command line the launcher already understands from game
// shortcuts; the id travels unquoted exactly as there, so one that would not
// survive a command line is refused instead of being split in two arguments.
func PlayArgs(id string) (string, error) {
	if id == "" || strings.ContainsFunc(id, func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsControl(r) || r == '"'
	}) {
		return "", fmt.Errorf("%w: %q", ErrBadGameID, id)
	}
	return "--play " + id, nil
}

func OpenArgs(route string) string {
	return "--open " + route
}

// recentEntries skips what the user removed from the list: the shell answers
// AppendCategory with E_ACCESSDENIED for the whole category if a removed
// destination is put back.
func recentEntries(cfg Config, games []Game, removed map[string]struct{}) ([]Entry, error) {
	entries := make([]Entry, 0, len(games))
	for _, game := range games {
		args, err := PlayArgs(game.ID)
		if err != nil {
			return nil, err
		}
		if _, gone := removed[args]; gone {
			continue
		}
		icon := cfg.Launcher
		if game.Executable != "" && filepath.IsAbs(game.Executable) {
			icon = game.Executable
		}
		entries = append(entries, Entry{Title: entryTitle(game), Args: args, Icon: icon})
	}
	return entries, nil
}

func taskEntries(cfg Config) []Entry {
	return []Entry{
		{Title: cfg.Labels.Library, Args: OpenArgs(RouteLibrary), Icon: cfg.Launcher},
		{Title: cfg.Labels.Downloads, Args: OpenArgs(RouteDownloads), Icon: cfg.Launcher},
	}
}

func entryTitle(game Game) string {
	title := strings.Join(strings.FieldsFunc(game.Title, func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsControl(r)
	}), " ")
	if title == "" {
		return game.ID
	}
	return title
}
