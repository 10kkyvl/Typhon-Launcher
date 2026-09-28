package sources

import (
	"context"
	"errors"
	"hash/crc32"
	"regexp"
	"strconv"
	"strings"
	"time"

	"typhon/internal/catalog"
	"typhon/internal/sources/feed"
	"typhon/internal/titles"
)

const (
	remoteMatchTTL       = 24 * time.Hour
	remoteMatchJitterMax = 12 * time.Hour
)

// Stable per key, so a large feed does not expire in a single refresh.
func remoteMatchJitter(key string) time.Duration {
	seconds := crc32.ChecksumIEEE([]byte(key)) % uint32(remoteMatchJitterMax/time.Second)
	return time.Duration(seconds) * time.Second
}

func remoteMatchFresh(rm *RemoteMatch, key string, now time.Time) bool {
	if rm == nil || rm.Key != key {
		return false
	}
	age := now.Sub(rm.At)
	return age >= 0 && age < remoteMatchTTL+remoteMatchJitter(key)
}

type resolvedRemoteMatch struct {
	catalog.Match
	fresh bool
}

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

func carryRemoteMatchForward(previous, incoming []*Release) {
	byIdentity := make(map[string]*Release, len(previous))
	for _, r := range previous {
		byIdentity[r.identity()] = r
	}
	for _, r := range incoming {
		if prev, ok := byIdentity[r.identity()]; ok {
			r.RemoteMatch = prev.RemoteMatch
		}
	}
}

func (s *Service) resolveRemoteReleases(ctx context.Context, list []*Release) (map[string]resolvedRemoteMatch, *catalog.PartialMatchError, error) {
	if s.catalog == nil || !s.catalog.HasReleaseMatcher() {
		return nil, nil, nil
	}
	now := time.Now()
	out := make(map[string]resolvedRemoteMatch)
	byKey := map[string]*Release{}
	cached := map[string]*RemoteMatch{}
	for _, r := range list {
		if r.Locked || r.Ignored || r.Availability == AvailabilityRemoved {
			continue
		}
		key := remoteQueryKey(r)
		if _, ok := byKey[key]; !ok {
			byKey[key] = r
		}
		if _, ok := cached[key]; !ok && remoteMatchFresh(r.RemoteMatch, key, now) {
			cached[key] = r.RemoteMatch
		}
	}

	verified := map[string]bool{}
	for _, rm := range cached {
		if rm.GameID == "" {
			continue
		}
		if _, checked := verified[rm.GameID]; !checked {
			verified[rm.GameID] = s.catalog.HasGame(rm.GameID)
		}
	}

	needsQuery := make([]*Release, 0, len(byKey))
	for key, r := range byKey {
		rm, isCached := cached[key]
		if isCached && (rm.GameID == "" || verified[rm.GameID]) {
			out[key] = resolvedRemoteMatch{Match: catalog.Match{
				Status:     rm.Status,
				GameID:     rm.GameID,
				Confidence: rm.Confidence,
				Method:     catalog.Method(rm.Method),
			}}
			continue
		}
		needsQuery = append(needsQuery, r)
	}

	keys, queries := remoteReleaseQueries(needsQuery)
	if len(queries) == 0 {
		return out, nil, nil
	}
	matches, err := s.catalog.ResolveSourceQueries(ctx, queries)
	var partial *catalog.PartialMatchError
	switch {
	case err == nil:
	case errors.As(err, &partial):
	default:
		return nil, nil, err
	}
	failed := map[int]bool{}
	if partial != nil {
		for _, i := range partial.Failed {
			failed[i] = true
		}
	}
	for i, key := range keys {
		if failed[i] {
			continue
		}
		out[key] = resolvedRemoteMatch{Match: matches[i], fresh: true}
	}
	return out, partial, nil
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
