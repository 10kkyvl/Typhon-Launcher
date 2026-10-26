package typhonapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"typhon/internal/account"
	"typhon/internal/catalog"
	"typhon/internal/metadata"
)

const (
	maxRelatedItems = 50
	maxRelatedTitle = 200
	maxRelatedID    = 64
	maxRelatedType  = 64
	maxRelatedCover = 1024
	maxPlatforms    = 40
	maxPlatformName = 80
)

var _ catalog.RelatedCatalog = (*Client)(nil)

type relatedItem struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	GameType    string   `json:"gameType"`
	Relation    string   `json:"relation"`
	ReleaseYear *int     `json:"releaseYear"`
	Platforms   []string `json:"platforms"`
	CoverURL    string   `json:"coverUrl"`
}

type relatedResponse struct {
	Items []relatedItem `json:"items"`
}

// Related returns the server's related records for a catalog game. A game the
// server does not know (404 with code not_found) is catalog.ErrRelatedNotFound,
// not an empty list; any other 404 is an upstream error.
func (c *Client) Related(ctx context.Context, serverID string) ([]catalog.RelatedGame, error) {
	if serverID == "" || len(serverID) > maxRelatedID {
		return nil, fmt.Errorf("%w: некорректный id игры", ErrBadRequest)
	}
	var payload relatedResponse
	var err error
	for attempt := 0; ; attempt++ {
		err = c.get(ctx, account.APIPrefix+"/catalog/games/"+url.PathEscape(serverID)+"/related", &payload)
		var limit *metadata.RateLimitError
		if !errors.As(err, &limit) || attempt >= 2 {
			break
		}
		timer := time.NewTimer(max(time.Second, limit.RetryAfter))
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
	var statusErr *httpStatusError
	if errors.As(err, &statusErr) && statusErr.status == http.StatusNotFound {
		if statusErr.code == "not_found" {
			return nil, fmt.Errorf("%w: %w", catalog.ErrRelatedNotFound, err)
		}
		// a 404 without the server error code comes from a proxy or an undeployed route, not from the catalog
		return nil, fmt.Errorf("%w: маршрут связанных игр недоступен: %w", ErrUpstream, err)
	}
	if err != nil {
		return nil, err
	}
	out := make([]catalog.RelatedGame, 0, len(payload.Items))
	seen := make(map[string]bool, len(payload.Items))
	for _, it := range payload.Items {
		game, ok := cleanRelated(it)
		if !ok || seen[game.ID] || game.ID == serverID {
			continue
		}
		seen[game.ID] = true
		out = append(out, game)
		if len(out) == maxRelatedItems {
			break
		}
	}
	return out, nil
}

func cleanRelated(it relatedItem) (catalog.RelatedGame, bool) {
	id := strings.TrimSpace(it.ID)
	title := cleanLabel(it.Title, maxRelatedTitle)
	if id == "" || len(id) > maxRelatedID || title == "" || !catalog.ValidRelation(it.Relation) {
		return catalog.RelatedGame{}, false
	}
	game := catalog.RelatedGame{
		ID:       id,
		Title:    title,
		GameType: cleanLabel(it.GameType, maxRelatedType),
		Relation: it.Relation,
	}
	if it.ReleaseYear != nil && *it.ReleaseYear >= 1950 && *it.ReleaseYear <= 2200 {
		year := *it.ReleaseYear
		game.ReleaseYear = &year
	}
	for _, p := range it.Platforms {
		if name := cleanLabel(p, maxPlatformName); name != "" && len(game.Platforms) < maxPlatforms {
			game.Platforms = append(game.Platforms, name)
		}
	}
	if cover := strings.TrimSpace(it.CoverURL); len(cover) <= maxRelatedCover {
		if u, err := url.Parse(cover); err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Host != "" {
			game.CoverURL = cover
		}
	}
	return game, true
}

func cleanLabel(s string, limit int) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || r == utf8.RuneError {
			return -1
		}
		return r
	}, s)
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) <= limit {
		return s
	}
	return strings.TrimSpace(string([]rune(s)[:limit]))
}
