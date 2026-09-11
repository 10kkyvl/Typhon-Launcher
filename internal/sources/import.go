package sources

import (
	"sort"
	"strconv"
	"strings"
	"time"

	"typhon/internal/catalog"
	"typhon/internal/sources/feed"
	"typhon/internal/titles"
)

// maxRemovedReleases bounds how many vanished-from-the-feed releases a
// source keeps around in s.releases. See evictStaleRemoved for why they
// cannot simply be dropped on removal.
const maxRemovedReleases = 5000

type matcher interface {
	ResolveAll(queries []catalog.Query) []catalog.Match
	Epoch() uint64
}

func parseEntries(sourceID string, entries []feed.Entry, now time.Time) []*Release {
	parsedEntries := make([]*Release, 0, len(entries))
	for _, e := range entries {
		parsed := titles.Parse(e.Title)
		kind := KindRelease
		if e.Type == feed.TypePatch {
			kind = KindPatch
		}
		base, normalized := parsed.Base, parsed.Normalized
		if e.Game != "" {
			hint := titles.Parse(e.Game)
			base, normalized = hint.Base, hint.Normalized
		}
		version := parsed.Version
		if e.ToVersion != "" {
			version = e.ToVersion
		}
		r := &Release{
			SourceID:        sourceID,
			DistributionID:  e.DistributionID,
			Kind:            kind,
			RawTitle:        e.Title,
			Title:           base,
			NormalizedTitle: normalized,
			Version:         version,
			RawVersion:      parsed.RawVersion,
			FromVersion:     e.FromVersion,
			ToVersion:       e.ToVersion,
			Sequence:        e.Sequence,
			Edition:         parsed.Edition,
			Languages:       parsed.Languages,
			Year:            parsed.Year,
			Tags:            parsed.Tags,
			Repacker:        titles.Repacker(parsed.Tags),
			DLCCount:        parsed.DLCCount,
			Size:            e.Size,
			SizeUnknown:     e.SizeUnknown,
			UploadedAt:      e.UploadedAt,
			URIs:            e.URIs,
			Availability:    AvailabilityAvailable,
			MatchStatus:     catalog.StatusUnmatched,
			FirstSeenAt:     now,
			LastSeenAt:      now,
			CreatedAt:       now,
		}
		for _, uri := range e.URIs {
			if hash, ok := feed.MagnetInfoHash(uri); ok {
				r.InfoHash = hash
				break
			}
		}
		parsedEntries = append(parsedEntries, r)
	}

	// A line identifier must point to one current full release (or one exact
	// patch transition). If a feed assigns it to competing entries, treating
	// either one as the installed distribution would be an unsafe guess.
	distributionCounts := make(map[string]int, len(parsedEntries))
	for _, r := range parsedEntries {
		if r.DistributionID != "" {
			distributionCounts[r.identity()]++
		}
	}
	for _, r := range parsedEntries {
		if r.DistributionID != "" && distributionCounts[r.identity()] > 1 {
			r.DistributionID = ""
		}
	}

	out := make([]*Release, 0, len(parsedEntries))
	seen := make(map[string]bool, len(parsedEntries))
	for _, r := range parsedEntries {
		key := r.identity()
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, r)
	}
	return out
}

func merge(existing, incoming []*Release, now time.Time, initial bool) ([]*Release, Summary) {
	var summary Summary
	index := make(map[string]*Release, len(existing))
	legacyIndex := make(map[string]*Release, len(existing))
	ambiguousLegacy := make(map[string]bool)
	incomingLegacyCounts := make(map[string]int, len(incoming))
	for _, r := range incoming {
		incomingLegacyCounts[r.legacyIdentity()]++
	}
	for _, r := range existing {
		index[r.identity()] = r
		if r.DistributionID != "" {
			continue
		}
		key := r.legacyIdentity()
		if legacyIndex[key] != nil {
			ambiguousLegacy[key] = true
		} else {
			legacyIndex[key] = r
		}
	}

	present := make(map[string]bool, len(incoming))
	merged := append([]*Release(nil), existing...)
	for _, next := range incoming {
		key := next.identity()
		present[key] = true
		current, ok := index[key]
		// A source can add distributionId to an existing entry without
		// changing its torrent. The exact former identity is the only safe
		// migration path; title/game/repacker similarities are deliberately
		// not used here.
		if !ok && next.DistributionID != "" && incomingLegacyCounts[next.legacyIdentity()] == 1 &&
			!ambiguousLegacy[next.legacyIdentity()] {
			current = legacyIndex[next.legacyIdentity()]
			ok = current != nil
		}
		if !ok {
			next.ID = catalog.NewID()
			next.New = !initial
			if next.New {
				summary.New++
			}
			merged = append(merged, next)
			index[key] = next
			summary.Added++
			continue
		}
		if current.Availability == AvailabilityRemoved {
			summary.Restored++
		}
		if changed(current, next) {
			summary.Updated++
			// Изменившийся заголовок или версия — другой запрос к каталогу,
			// поэтому прошлый результат матчинга больше не действителен.
			current.MatchEpoch = 0
			if !current.Locked && current.NormalizedTitle != next.NormalizedTitle {
				current.CanonicalGameID = nil
				current.MatchStatus = ""
				current.MatchMethod = ""
			}
		}
		current.RawTitle = next.RawTitle
		current.DistributionID = next.DistributionID
		current.Kind = next.Kind
		current.Title = next.Title
		current.NormalizedTitle = next.NormalizedTitle
		current.Version = next.Version
		current.RawVersion = next.RawVersion
		current.FromVersion = next.FromVersion
		current.ToVersion = next.ToVersion
		current.Sequence = next.Sequence
		current.Edition = next.Edition
		current.Languages = next.Languages
		current.Year = next.Year
		current.Tags = next.Tags
		current.Repacker = next.Repacker
		current.DLCCount = next.DLCCount
		current.Size = next.Size
		current.SizeUnknown = next.SizeUnknown
		current.UploadedAt = next.UploadedAt
		current.URIs = next.URIs
		current.InfoHash = next.InfoHash
		current.Availability = AvailabilityAvailable
		current.LastSeenAt = now
	}

	for _, r := range merged {
		if present[r.identity()] || r.Availability == AvailabilityRemoved {
			continue
		}
		r.Availability = AvailabilityRemoved
		summary.Removed++
	}
	return evictStaleRemoved(merged), summary
}

// evictStaleRemoved bounds how many AvailabilityRemoved releases a source
// keeps once they pass maxRemovedReleases. They cannot be dropped the
// moment a release goes missing from the feed: GetSourceDetails counts them
// separately and SourceDetailsModal has a "removed" tab so a user can see
// what disappeared from a source. But nothing ever deleted them either, and
// feed.MaxEntries only caps a single parse pass — a source with churn
// (releases leaving and returning over months) grew this list without any
// upper bound. Evicting the oldest-by-LastSeenAt removed releases first
// keeps the useful case (recent disappearances stay visible) while putting
// a ceiling on memory; releases still available in the feed are never
// touched by this, no matter how many removed ones pile up around them.
func evictStaleRemoved(list []*Release) []*Release {
	var removedIdx []int
	for i, r := range list {
		if r.Availability == AvailabilityRemoved {
			removedIdx = append(removedIdx, i)
		}
	}
	if len(removedIdx) <= maxRemovedReleases {
		return list
	}
	sort.Slice(removedIdx, func(a, b int) bool {
		return list[removedIdx[a]].LastSeenAt.Before(list[removedIdx[b]].LastSeenAt)
	})
	evict := make(map[int]bool, len(removedIdx)-maxRemovedReleases)
	for _, idx := range removedIdx[:len(removedIdx)-maxRemovedReleases] {
		evict[idx] = true
	}
	out := make([]*Release, 0, len(list)-len(evict))
	for i, r := range list {
		if evict[i] {
			continue
		}
		out = append(out, r)
	}
	return out
}

func changed(current, next *Release) bool {
	return current.DistributionID != next.DistributionID ||
		current.RawTitle != next.RawTitle ||
		current.Kind != next.Kind ||
		current.Version != next.Version ||
		current.FromVersion != next.FromVersion ||
		current.ToVersion != next.ToVersion ||
		current.Sequence != next.Sequence ||
		current.Size != next.Size ||
		current.SizeUnknown != next.SizeUnknown ||
		current.Repacker != next.Repacker ||
		current.DLCCount != next.DLCCount ||
		strings.Join(current.Tags, "|") != strings.Join(next.Tags, "|") ||
		strings.Join(current.URIs, "|") != strings.Join(next.URIs, "|") ||
		!sameTime(current.UploadedAt, next.UploadedAt)
}

func sameTime(a, b *time.Time) bool {
	switch {
	case a == nil && b == nil:
		return true
	case a == nil || b == nil:
		return false
	default:
		return a.Equal(*b)
	}
}

// applyMatches пропускает релиз, который уже матчился на текущей эпохе и с
// тех пор не менялся: каталог и словарь те же, значит и ответ будет тот же.
// Без этого каждый рефетч прогонял через fuzzy все нераспознанные записи —
// на большом фиде это десятки тысяч сравнений впустую.
func applyMatches(m matcher, list []*Release) error {
	if m == nil {
		return nil
	}
	epoch := m.Epoch()
	targets := make([]*Release, 0, len(list))
	for _, r := range list {
		if r.Locked || r.Ignored {
			continue
		}
		if r.MatchEpoch == epoch {
			continue
		}
		if r.MatchStatus == catalog.StatusMatched && r.CanonicalGameID != nil && stableMatch(r.MatchMethod) {
			r.MatchEpoch = epoch
			continue
		}
		targets = append(targets, r)
	}
	if len(targets) == 0 {
		return nil
	}

	keys := make([]string, len(targets))
	queries := make([]catalog.Query, 0, len(targets))
	position := make(map[string]int, len(targets))
	for i, r := range targets {
		key := queryKey(r)
		keys[i] = key
		if _, ok := position[key]; ok {
			continue
		}
		position[key] = len(queries)
		queries = append(queries, catalog.Query{Title: r.Title, Normalized: r.NormalizedTitle, Year: r.Year})
	}

	matches := m.ResolveAll(queries)
	for i, r := range targets {
		match := matches[position[keys[i]]]
		assign(r, match)
		r.MatchEpoch = epoch
	}

	return nil
}

// Матч по псевдониму или похожести держится на данных каталога, а они
// меняются: псевдоним могут выбросить, игру — переименовать. Такой матч
// пересчитывается на каждом обновлении, точный и ручной — нет.
func stableMatch(method string) bool {
	switch catalog.Method(method) {
	case catalog.MethodExternalID, catalog.MethodOverride:
		return true
	}
	return false
}

func queryKey(r *Release) string {
	if r.Year > 0 {
		return r.NormalizedTitle + "|" + strconv.Itoa(r.Year)
	}
	return r.NormalizedTitle
}

func assign(r *Release, match catalog.Match) {
	r.MatchStatus = match.Status
	r.MatchConfidence = match.Confidence
	r.MatchMethod = string(match.Method)
	if match.Status == catalog.StatusMatched && match.GameID != "" {
		id := match.GameID
		r.CanonicalGameID = &id
		return
	}
	r.CanonicalGameID = nil
}

func counts(list []*Release) (matched, review, unmatched int) {
	for _, r := range list {
		if r.Availability == AvailabilityRemoved || r.Ignored {
			continue
		}
		switch r.MatchStatus {
		case catalog.StatusMatched:
			matched++
		case catalog.StatusReview:
			review++
		default:
			unmatched++
		}
	}
	return
}
