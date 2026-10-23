package tray

import (
	"strings"
	"unicode"
)

const (
	MaxRecent      = 5
	maxTitleRunes  = 40
	titleEllipsis  = "…"
	playingDivider = " — "
)

type Kind int

const (
	KindHeader Kind = iota
	KindGame
	KindStop
	KindSeparator
	KindOpen
	KindQuit
)

type Game struct {
	ID      string
	Title   string
	Running bool
}

type Labels struct {
	Recent  string
	Playing string
	Stop    string
	Open    string
	Quit    string
}

type Item struct {
	Kind     Kind
	Label    string
	GameID   string
	Disabled bool
}

func BuildItems(labels Labels, games []Game) []Item {
	var items []Item
	shown := games
	if len(shown) > MaxRecent {
		shown = shown[:MaxRecent]
	}
	if len(shown) > 0 {
		items = append(items, Item{Kind: KindHeader, Label: labels.Recent})
		for _, game := range shown {
			title := cleanTitle(game.Title)
			if title == "" {
				title = game.ID
			}
			if game.Running {
				items = append(items,
					Item{Kind: KindGame, Label: title + playingDivider + labels.Playing, GameID: game.ID, Disabled: true},
					Item{Kind: KindStop, Label: labels.Stop, GameID: game.ID},
				)
				continue
			}
			items = append(items, Item{Kind: KindGame, Label: title, GameID: game.ID})
		}
		items = append(items, Item{Kind: KindSeparator})
	}
	return append(items,
		Item{Kind: KindOpen, Label: labels.Open},
		Item{Kind: KindSeparator},
		Item{Kind: KindQuit, Label: labels.Quit},
	)
}

// A tab in a native menu label starts the accelerator column, a newline cuts the item.
func cleanTitle(title string) string {
	var b strings.Builder
	space := false
	for _, r := range strings.TrimSpace(title) {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			space = true
			continue
		}
		if space && b.Len() > 0 {
			b.WriteRune(' ')
		}
		space = false
		b.WriteRune(r)
	}
	runes := []rune(b.String())
	if len(runes) > maxTitleRunes {
		return string(runes[:maxTitleRunes-1]) + titleEllipsis
	}
	return string(runes)
}

// On Windows a single ampersand marks a mnemonic and swallows the next letter.
func NativeLabel(label string, windows bool) string {
	if !windows {
		return label
	}
	return strings.ReplaceAll(label, "&", "&&")
}
