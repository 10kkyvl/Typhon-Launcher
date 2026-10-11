package catalog

import (
	"net/url"
	"strings"
)

const maxStoreLinkLen = 512

const (
	StoreSteam = "steam"
	StoreGOG   = "gog"
	StoreEpic  = "epic"
)

var storeHosts = map[string][]string{
	StoreSteam: {"store.steampowered.com"},
	StoreGOG:   {"gog.com", "www.gog.com"},
	StoreEpic:  {"store.epicgames.com", "www.epicgames.com"},
}

func ValidStoreLink(store, raw string) (string, bool) {
	hosts, ok := storeHosts[store]
	if !ok || raw == "" || len(raw) > maxStoreLinkLen || raw != strings.TrimSpace(raw) {
		return "", false
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Opaque != "" {
		return "", false
	}
	host := strings.ToLower(u.Hostname())
	if u.Port() != "" {
		return "", false
	}
	for _, h := range hosts {
		if host == h {
			return u.String(), true
		}
	}
	return "", false
}

func SanitizeStoreLinks(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for store, raw := range in {
		if link, ok := ValidStoreLink(store, raw); ok {
			out[store] = link
		}
	}
	return out
}
