package sources

import (
	"context"
	"errors"
	"hash/crc32"
	"strings"
	"time"

	"typhon/internal/catalog"
	"typhon/internal/matchprogress"
	"typhon/internal/sources/feed"
)

const (
	remoteMatchTTL       = 24 * time.Hour
	remoteMatchJitterMax = 12 * time.Hour

	// A found game rarely changes its answer, so asking again every day would
	// spend a request per release for nothing. The jitter is wider than one
	// refresh cycle, so a large source still does not expire in a single pass.
	remoteMatchKnownTTL       = 7 * 24 * time.Hour
	remoteMatchKnownJitterMax = 24 * time.Hour
)

// remoteMatchRev is bumped when a match answer starts to carry more than it did
// (rev 1: the catalog record's parentId and gameType). A found match saved under
// an older rev is asked again, in the ordinary batches, instead of waiting out
// remoteMatchKnownTTL.
const remoteMatchRev = 1

// Stable per key, so a large feed does not expire in a single refresh.
func remoteMatchJitter(key string, span time.Duration) time.Duration {
	seconds := crc32.ChecksumIEEE([]byte(key)) % uint32(span.Seconds())
	return time.Duration(seconds) * time.Second
}

// A release the server did not know stays on the short lifetime: the server
// may learn it any day.
func remoteMatchLifetime(rm *RemoteMatch, key string) time.Duration {
	if rm.GameID != "" {
		return remoteMatchKnownTTL + remoteMatchJitter(key, remoteMatchKnownJitterMax)
	}
	return remoteMatchTTL + remoteMatchJitter(key, remoteMatchJitterMax)
}

func remoteMatchFresh(rm *RemoteMatch, key string, now time.Time) bool {
	if rm == nil || rm.Key != key {
		return false
	}
	if rm.GameID != "" && rm.Rev < remoteMatchRev {
		return false
	}
	age := now.Sub(rm.At)
	return age >= 0 && age < remoteMatchLifetime(rm, key)
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

func (s *Service) resolveRemoteReleases(ctx context.Context, list []*Release, run *refreshRun) (map[string]resolvedRemoteMatch, *catalog.PartialMatchError, error) {
	if s.catalog == nil || !s.catalog.HasReleaseMatcher() {
		return nil, nil, nil
	}
	defer run.begin(stageMatch)()
	now := s.now()
	out := make(map[string]resolvedRemoteMatch)
	byKey := map[string]*Release{}
	cached := map[string]*RemoteMatch{}
	for i, r := range list {
		if err := interrupted(ctx, i); err != nil {
			return nil, nil, err
		}
		if r.Locked || r.Ignored || r.Availability == AvailabilityRemoved {
			continue
		}
		key := run.memo.queryKey(r)
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

	keys, queries, err := remoteReleaseQueries(ctx, run.memo, needsQuery)
	if err != nil {
		return nil, nil, err
	}
	run.progress(stageMatch, 0, len(queries))
	if len(queries) == 0 {
		return out, nil, nil
	}
	matchCtx := matchprogress.With(ctx, func(done, total int) { run.progress(stageMatch, done, total) })
	matches, err := s.catalog.ResolveSourceQueries(matchCtx, queries)
	var partial *catalog.PartialMatchError
	switch {
	case err == nil:
	case errors.As(err, &partial):
	default:
		return nil, nil, err
	}
	run.finish(stageMatch, len(queries))
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

func remoteReleaseQueries(ctx context.Context, memo *titleMemo, list []*Release) ([]string, []catalog.ReleaseQuery, error) {
	keys := []string{}
	queries := []catalog.ReleaseQuery{}
	seen := map[string]bool{}
	for i, r := range list {
		if err := interrupted(ctx, i); err != nil {
			return nil, nil, err
		}
		if r.Locked || r.Ignored || r.Availability == AvailabilityRemoved {
			continue
		}
		names := memo.releaseNames(r)
		key := memo.queryKey(r)
		if seen[key] {
			continue
		}
		seen[key] = true
		valid := make([]string, 0, len(names))
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
	return keys, queries, nil
}

func (s *Service) previewWithCoverage(ctx context.Context, kind Type, location string, result feed.Result) (Preview, error) {
	preview := s.preview(kind, location, result)
	if s.catalog == nil || !s.catalog.HasReleaseMatcher() {
		return preview, nil
	}
	memo := s.newMemo()
	list, err := parseFeedEntries(ctx, "", result.Feed.Entries, s.now(), memo, nil)
	if err != nil {
		return Preview{}, err
	}
	_, queries, err := remoteReleaseQueries(ctx, memo, list)
	if err != nil {
		return Preview{}, err
	}
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
