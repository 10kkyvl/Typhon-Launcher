package jumplist

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"unicode"

	"typhon/internal/shortcut"
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

func OpenArgs(route string) string {
	return "--open " + route
}

// recentEntries skips removed destinations: the shell fails AppendCategory
// with E_ACCESSDENIED for the whole category if one is put back. A game whose
// id cannot be a command line argument is skipped too and reported, so one bad
// record does not cost the user the whole list.
func recentEntries(cfg Config, games []Game, removed map[string]struct{}) (entries []Entry, skipped []error) {
	entries = make([]Entry, 0, len(games))
	for _, game := range games {
		args, err := shortcut.PlayArgs(game.ID, "")
		if err != nil {
			skipped = append(skipped, err)
			continue
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
	return entries, skipped
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
