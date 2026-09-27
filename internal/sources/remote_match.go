package sources

import (
	"context"
	"regexp"
	"strconv"
	"strings"
	"time"

	"typhon/internal/catalog"
	"typhon/internal/sources/feed"
	"typhon/internal/titles"
)

func cloneReleases(list []*Release) []*Release {
	out := make([]*Release, len(list))
	for i, r := range list {
		copy := *r
		out[i] = &copy
	}
	return out
}

// Reparse cached rows too: HTTP 304 only says the feed bytes did not change.
func reparseRelease(r *Release) {
	p := titles.Parse(r.RawTitle)
	r.Title, r.NormalizedTitle = p.Base, p.Normalized
	if r.GameHint != "" {
		hint := titles.Parse(r.GameHint)
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

func (s *Service) resolveRemoteReleases(ctx context.Context, list []*Release) (map[string]catalog.Match, error) {
	if s.catalog == nil || !s.catalog.HasReleaseMatcher() {
		return nil, nil
	}
	keys, queries := remoteReleaseQueries(list)
	if len(queries) == 0 {
		return nil, nil
	}
	matches, err := s.catalog.ResolveSourceQueries(ctx, queries)
	if err != nil {
		return nil, err
	}
	out := make(map[string]catalog.Match, len(keys))
	for i, key := range keys {
		out[key] = matches[i]
	}
	return out, nil
}
func remoteQueryKey(r *Release) string {
	return strings.Join(remoteReleaseNames(r), "\x00") + "|" + strconv.Itoa(r.Year)
}

var singleReleaseChapter = regexp.MustCompile(`^(?:chapter|episode) (?:[0-9]+|[ivxlcdm]+)$`)

// A source's stable game hint can omit the chapter or edition that identifies
// this particular release. Use the more specific title only when it extends
// the same base name; unrelated download labels must not overrule the hint.
func remoteReleaseNames(r *Release) []string {
	if r.GameHint == "" {
		return titles.MatchNames(r.RawTitle)
	}
	hinted := titles.MatchNames(r.GameHint)
	raw, hint := titles.Parse(r.RawTitle), titles.Parse(r.GameHint)
	base, hintedBase := titles.Normalize(raw.Base), titles.Normalize(hint.Base)
	if base == hintedBase && raw.Edition != "" && hint.Edition == "" {
		return titles.MatchNames(r.RawTitle)
	}
	tail, extends := strings.CutPrefix(base, hintedBase+" ")
	if hintedBase == "" || !extends || hint.Edition != "" {
		return hinted
	}
	// These suffixes distinguish a different release. MatchNames deliberately
	// avoids falling back from a remaster to the original game.
	if tail == "remaster" || tail == "remastered" {
		return titles.MatchNames(r.RawTitle)
	}
	if tail != "enhanced" && !singleReleaseChapter.MatchString(tail) {
		return hinted
	}
	// Episodic games can have either separate chapter cards or one series card.
	// Bare Enhanced is also used for a base game's update. Prefer a specific
	// known card while retaining the author's explicit hint in these cases.
	names := titles.MatchNames(r.RawTitle)
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

func remoteReleaseQueries(list []*Release) ([]string, []catalog.ReleaseQuery) {
	keys := []string{}
	queries := []catalog.ReleaseQuery{}
	seen := map[string]bool{}
	for _, r := range list {
		if r.Locked || r.Ignored || r.Availability == AvailabilityRemoved {
			continue
		}
		names := remoteReleaseNames(r)
		key := strings.Join(names, "\x00") + "|" + strconv.Itoa(r.Year)
		if seen[key] {
			continue
		}
		seen[key] = true
		valid := names[:0]
		for _, name := range names {
			if len([]rune(name)) <= 200 && strings.TrimSpace(name) != "" {
				valid = append(valid, name)
			}
		}
		if len(valid) == 0 {
			continue
		}
		keys = append(keys, key)
		queries = append(queries, catalog.ReleaseQuery{Titles: valid, Year: r.Year})
	}
	return keys, queries
}

func (s *Service) previewWithCoverage(ctx context.Context, kind Type, location string, result feed.Result) (Preview, error) {
	preview := s.preview(kind, location, result)
	if s.catalog == nil || !s.catalog.HasReleaseMatcher() {
		return preview, nil
	}
	_, queries := remoteReleaseQueries(parseEntries("", result.Feed.Entries, time.Now()))
	matches, err := s.catalog.PreviewSourceQueries(ctx, queries)
	if err != nil {
		return Preview{}, err
	}
	preview.Games = len(queries)
	preview.Known = 0
	for _, m := range matches {
		if m.Game != nil {
			preview.Known++
		}
	}
	preview.Unknown = preview.Games - preview.Known
	return preview, nil
}
