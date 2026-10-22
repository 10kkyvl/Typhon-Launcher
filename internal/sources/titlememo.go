package sources

import (
	"regexp"
	"slices"
	"strconv"
	"strings"

	"typhon/internal/titles"
)

// titleMemo parses each distinct title once per refresh: the same release is
// read by the import, the remote match and the settle step, and Parse and
// MatchNames are the expensive part of all three. It lives for one refresh
// goroutine and is not safe for concurrent use.
type titleMemo struct {
	parse      func(string) titles.Parsed
	matchNames func(string) []string
	parsed     map[string]titles.Parsed
	matched    map[string][]string
	named      map[[2]string][]string
}

func newTitleMemo() *titleMemo {
	return &titleMemo{
		parse:      titles.Parse,
		matchNames: titles.MatchNames,
		parsed:     map[string]titles.Parsed{},
		matched:    map[string][]string{},
		named:      map[[2]string][]string{},
	}
}

func (m *titleMemo) Parse(raw string) titles.Parsed {
	if p, ok := m.parsed[raw]; ok {
		return p
	}
	p := m.parse(raw)
	m.parsed[raw] = p
	return p
}

func (m *titleMemo) MatchNames(raw string) []string {
	if names, ok := m.matched[raw]; ok {
		return slices.Clone(names)
	}
	names := m.matchNames(raw)
	m.matched[raw] = names
	return slices.Clone(names)
}

// Reparse cached rows too: HTTP 304 only says the feed bytes did not change.
func (m *titleMemo) reparse(r *Release) {
	p := m.Parse(r.RawTitle)
	r.Title, r.NormalizedTitle = p.Base, p.Normalized
	if r.GameHint != "" {
		hint := m.Parse(r.GameHint)
		r.Title, r.NormalizedTitle = hint.Base, hint.Normalized
		if hint.Year != 0 {
			p.Year = hint.Year
		}
	}
	r.Version, r.RawVersion = p.Version, p.RawVersion
	if r.ToVersion != "" {
		r.Version = r.ToVersion
	}
	r.Edition, r.Languages, r.Year, r.Tags, r.DLCCount = p.Edition, p.Languages, p.Year, p.Tags, p.DLCCount
	r.Repacker = titles.Repacker(p.Tags)
}

func (m *titleMemo) queryKey(r *Release) string {
	return strings.Join(m.releaseNames(r), "\x00") + "|" + strconv.Itoa(r.Year)
}

// The result is shared between releases with the same title and hint: callers
// must not modify it.
func (m *titleMemo) releaseNames(r *Release) []string {
	id := [2]string{r.RawTitle, r.GameHint}
	if names, ok := m.named[id]; ok {
		return names
	}
	names := m.computeReleaseNames(r)
	m.named[id] = names
	return names
}

var singleReleaseChapter = regexp.MustCompile(`^(?:chapter|episode) (?:[0-9]+|[ivxlcdm]+)$`)

// A source's stable game hint can omit the chapter or edition that identifies
// this particular release. Use the more specific title only when it extends
// the same base name; unrelated download labels must not overrule the hint.
func (m *titleMemo) computeReleaseNames(r *Release) []string {
	if r.GameHint == "" {
		return m.MatchNames(r.RawTitle)
	}
	hinted := m.MatchNames(r.GameHint)
	raw, hint := m.Parse(r.RawTitle), m.Parse(r.GameHint)
	base, hintedBase := titles.Normalize(raw.Base), titles.Normalize(hint.Base)
	if base == hintedBase && raw.Edition != "" && hint.Edition == "" {
		return m.MatchNames(r.RawTitle)
	}
	tail, extends := strings.CutPrefix(base, hintedBase+" ")
	if hintedBase == "" || !extends || hint.Edition != "" {
		return hinted
	}
	// These suffixes distinguish a different release. MatchNames deliberately
	// avoids falling back from a remaster to the original game.
	if tail == "remaster" || tail == "remastered" {
		return m.MatchNames(r.RawTitle)
	}
	if tail != "enhanced" && !singleReleaseChapter.MatchString(tail) {
		return hinted
	}
	// Episodic games can have either separate chapter cards or one series card.
	// Bare Enhanced is also used for a base game's update. Prefer a specific
	// known card while retaining the author's explicit hint in these cases.
	names := m.MatchNames(r.RawTitle)
	seen := map[string]bool{}
	for _, name := range names {
		seen[titles.Normalize(name)] = true
	}
	for _, name := range hinted {
		if !seen[titles.Normalize(name)] && len(names) < 6 {
			names = append(names, name)
			seen[titles.Normalize(name)] = true
		}
	}
	return names
}
