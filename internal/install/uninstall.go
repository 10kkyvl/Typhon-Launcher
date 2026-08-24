package install

import (
	"strings"
	"unicode"

	"typhon/internal/library"
)

type uninstallEntry struct {
	Key             string
	DisplayName     string
	Command         string
	QuietCommand    string
	InstallLocation string
	ProductCode     string
	SystemComponent bool
}

func (e uninstallEntry) usable() bool {
	return e.Command != "" || e.QuietCommand != "" || e.ProductCode != ""
}

func newEntries(before, after map[string]uninstallEntry) []uninstallEntry {
	out := make([]uninstallEntry, 0, 4)
	for key, entry := range after {
		if _, known := before[key]; known {
			continue
		}
		if entry.SystemComponent || !entry.usable() {
			continue
		}
		out = append(out, entry)
	}
	return out
}

func pickUninstall(before, after map[string]uninstallEntry, destination, name string) (library.Uninstall, bool) {
	entries := newEntries(before, after)
	if len(entries) == 0 {
		return library.Uninstall{}, false
	}
	if destination != "" {
		matched := make([]uninstallEntry, 0, 2)
		for _, entry := range entries {
			if entry.InstallLocation == "" {
				continue
			}
			if samePath(entry.InstallLocation, destination) || inside(entry.InstallLocation, destination) {
				matched = append(matched, entry)
			}
		}
		if len(matched) == 1 {
			return uninstallOf(matched[0]), true
		}
	}
	if key := titleKey(name); key != "" {
		matched := make([]uninstallEntry, 0, 2)
		for _, entry := range entries {
			display := titleKey(entry.DisplayName)
			if display == "" {
				continue
			}
			if strings.Contains(display, key) || strings.Contains(key, display) {
				matched = append(matched, entry)
			}
		}
		if len(matched) == 1 {
			return uninstallOf(matched[0]), true
		}
	}
	if len(entries) == 1 {
		return uninstallOf(entries[0]), true
	}
	return library.Uninstall{}, false
}

func uninstallOf(e uninstallEntry) library.Uninstall {
	return library.Uninstall{
		Key:          e.Key,
		Command:      e.Command,
		QuietCommand: e.QuietCommand,
		ProductCode:  e.ProductCode,
	}
}

func titleKey(name string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(name) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}
