package sources

import (
	"context"
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
	raw := r.RawTitle
	if r.GameHint != "" {
		raw = r.GameHint
	}
	return strings.Join(titles.MatchNames(raw), "\x00") + "|" + strconv.Itoa(r.Year)
}

func remoteReleaseQueries(list []*Release) ([]string, []catalog.ReleaseQuery) {
	keys := []string{}
	queries := []catalog.ReleaseQuery{}
	seen := map[string]bool{}
	for _, r := range list {
		if r.Locked || r.Ignored || r.Availability == AvailabilityRemoved {
			continue
		}
		key := remoteQueryKey(r)
		if seen[key] {
			continue
		}
		seen[key] = true
		raw := r.RawTitle
		if r.GameHint != "" {
			raw = r.GameHint
		}
		names := titles.MatchNames(raw)
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
