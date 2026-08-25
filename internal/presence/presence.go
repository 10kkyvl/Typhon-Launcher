package presence

import (
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"typhon/internal/discord"
	"typhon/internal/library"
	"typhon/internal/settings"
)

const (
	assetKey  = "typhon-icon"
	appName   = "Typhon"
	maxText   = 128
	minText   = 2
	untitled  = "Игра"
	hourText  = "Всего %d ч"
	minutText = "Всего %d мин"
)

var (
	errNoTarget = errors.New("presence target не задан")
	errNoCover  = errors.New("resolver обложки не задан")
	errScheme   = errors.New("обложка доступна только по https")
)

type Target interface {
	Show(discord.Presence)
	Clear()
	SetEnabled(bool)
}

type CoverFunc func(canonicalGameID string) (string, error)

type Watcher struct {
	target Target
	cover  CoverFunc
	now    func() time.Time
}

func New(target Target, cover CoverFunc) (*Watcher, error) {
	if target == nil {
		return nil, errNoTarget
	}
	if cover == nil {
		return nil, errNoCover
	}
	return &Watcher{target: target, cover: cover, now: time.Now}, nil
}

func (w *Watcher) Apply(s settings.Settings) {
	w.target.SetEnabled(s.DiscordRichPresence)
}

func (w *Watcher) SessionStarted(game library.Game) {
	w.target.Show(w.build(game))
}

func (w *Watcher) SessionStopped(string) {
	w.target.Clear()
}

func (w *Watcher) build(game library.Game) discord.Presence {
	name := title(game)
	p := discord.Presence{
		Details:    name,
		State:      playtime(game.PlaytimeSeconds),
		StartedAt:  w.now(),
		LargeImage: assetKey,
		LargeText:  appName,
	}
	cover, err := w.coverURL(game.CanonicalGameID)
	if err != nil {
		slog.Warn("discord cover unavailable", "id", game.ID, "error", err)
		return p
	}
	if cover == "" {
		return p
	}
	p.LargeImage = cover
	p.LargeText = name
	p.SmallImage = assetKey
	p.SmallText = appName
	return p
}

func (w *Watcher) coverURL(canonicalGameID string) (string, error) {
	if strings.TrimSpace(canonicalGameID) == "" {
		return "", nil
	}
	raw, err := w.cover(canonicalGameID)
	if err != nil {
		return "", err
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("разбор обложки %s: %w", raw, err)
	}
	if parsed.Scheme != "https" || parsed.Host == "" {
		return "", fmt.Errorf("%w: %s", errScheme, raw)
	}
	return raw, nil
}

func title(game library.Game) string {
	name := clamp(game.Title)
	if name == "" {
		return untitled
	}
	return name
}

func clamp(text string) string {
	text = strings.TrimSpace(text)
	runes := []rune(text)
	if len(runes) > maxText {
		return strings.TrimSpace(string(runes[:maxText]))
	}
	if len(runes) < minText {
		return ""
	}
	return text
}

func playtime(seconds int64) string {
	if seconds < 60 {
		return ""
	}
	if seconds < 3600 {
		return fmt.Sprintf(minutText, seconds/60)
	}
	return fmt.Sprintf(hourText, seconds/3600)
}
