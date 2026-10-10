package metadata

import (
	"errors"
	"fmt"
	"strings"

	"typhon/internal/catalog"
	"typhon/internal/uierr"

	"github.com/wailsapp/wails/v3/pkg/application"
)

var (
	errStoreLinkMissing = uierr.New("metadata.store_link_missing", "у игры нет ссылки на этот магазин")
	errStoreLinkInvalid = uierr.New("metadata.store_link_invalid", "сохранённая ссылка на магазин не прошла проверку")
	errNoApp            = errors.New("приложение не запущено")
)

func validStoreLinks(in map[string]string) map[string]string {
	out := catalog.SanitizeStoreLinks(in)
	if len(out) == 0 {
		return nil
	}
	return out
}

func (s *Service) OpenStoreLink(gameID, store string) error {
	gameID = strings.TrimSpace(gameID)
	if gameID == "" {
		return errNoGameID
	}
	game, err := s.catalog.GetGame(gameID)
	if err != nil {
		return err
	}
	raw, ok := game.StoreLinks[store]
	if !ok || raw == "" {
		return errStoreLinkMissing
	}
	link, ok := catalog.ValidStoreLink(store, raw)
	if !ok {
		return errStoreLinkInvalid
	}
	if err := s.openURL(link); err != nil {
		return fmt.Errorf("open store link: %w", err)
	}
	return nil
}

func openSystemBrowser(link string) error {
	app := application.Get()
	if app == nil {
		return errNoApp
	}
	return app.Browser.OpenURL(link)
}
