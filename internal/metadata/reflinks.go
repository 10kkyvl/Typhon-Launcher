package metadata

import (
	"fmt"
	"net/url"
	"strings"

	"typhon/internal/uierr"
)

const (
	LinkPCGamingWiki  = "pcgamingwiki"
	LinkHowLongToBeat = "howlongtobeat"
	LinkSteamDLC      = "steamdlc"

	hltbSearchPrefix = "https://howlongtobeat.com/?q="
	maxHLTBTitle     = 200
)

var (
	errLinkKind     = uierr.New("metadata.link_kind_unknown", "неизвестный вид ссылки")
	errLinkNoData   = uierr.New("metadata.link_no_data", "для этой ссылки нет данных об игре")
	errLinkRejected = uierr.New("metadata.link_rejected", "ссылка не прошла проверку")
)

// allowedReferenceURL — закрытый список справочных адресов, которые лаунчер
// открывает в браузере. Проверяется итоговая строка, а не только то, как её
// собрали: открытие ссылки не должно зависеть от аккуратности сборщика.
func allowedReferenceURL(link string) bool {
	u, err := url.Parse(link)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Opaque != "" || u.Fragment != "" {
		return false
	}
	switch u.Host {
	case pcgwHost:
		_, ok := validPageURL(link)
		return ok
	case "howlongtobeat.com":
		return u.Path == "/" && strings.HasPrefix(link, hltbSearchPrefix) && u.Query().Get("q") != ""
	case "store.steampowered.com":
		return u.RawQuery == "" && steamStoreURL.MatchString(u.Path)
	default:
		return false
	}
}

// OpenReferenceLink открывает справочную страницу игры. Адрес фронт не
// передаёт: он называет вид ссылки, а адрес берётся из данных, которые
// лаунчер уже проверил.
func (s *Service) OpenReferenceLink(gameID, kind string, appID int64) error {
	gameID = strings.TrimSpace(gameID)
	if gameID == "" {
		return errNoGameID
	}
	game, err := s.catalog.GetGame(gameID)
	if err != nil {
		return err
	}
	var link string
	switch kind {
	case LinkPCGamingWiki:
		key := providerKey(game)
		if key == "" {
			return errLinkNoData
		}
		entry, ok := s.saves.peek(key)
		if !ok || entry.Missing || entry.Paths == nil {
			return errLinkNoData
		}
		link = entry.Paths.PageURL
	case LinkHowLongToBeat:
		title := cleanText(game.Title, maxHLTBTitle)
		if title == "" {
			return errLinkNoData
		}
		// HowLongToBeat keeps q percent-encoded twice in its own search URLs and
		// rewrites any other form into that, so the link uses it directly.
		once := strings.ReplaceAll(url.QueryEscape(title), "+", "%20")
		link = hltbSearchPrefix + url.QueryEscape(once)
	case LinkSteamDLC:
		info := s.steam.info(game.ID)
		if info == nil {
			return errLinkNoData
		}
		found := false
		for _, d := range info.DLC {
			if d.AppID == appID {
				found = true
				break
			}
		}
		if !found {
			return errLinkNoData
		}
		link = SteamStoreURL(appID)
	default:
		return fmt.Errorf("%w: %q", errLinkKind, kind)
	}
	if !allowedReferenceURL(link) {
		return errLinkRejected
	}
	if err := s.openURL(link); err != nil {
		return fmt.Errorf("open reference link: %w", err)
	}
	return nil
}
