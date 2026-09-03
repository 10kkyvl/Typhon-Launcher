package sources

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"typhon/internal/catalog"
	"typhon/internal/sources/feed"
	"typhon/internal/titles"
)

type matcher interface {
	ResolveAll(queries []catalog.Query) []catalog.Match
	Provision(queries []catalog.Query) (map[string]catalog.Game, error)
}

func parseEntries(sourceID string, entries []feed.Entry, now time.Time) []*Release {
	out := make([]*Release, 0, len(entries))
	seen := make(map[string]bool, len(entries))
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
			Repacker:        repackerOf(parsed.Tags),
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
		key := r.identity()
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, r)
	}
	return out
}

var repackerPriority = []string{"fitgirl", "dodi", "elamigos", "xatab", "kaoskrew", "masquerade"}

func repackerOf(tags []string) string {
	set := make(map[string]bool, len(tags))
	for _, t := range tags {
		set[t] = true
	}
	for _, p := range repackerPriority {
		if set[p] {
			return p
		}
	}
	return ""
}

func merge(existing, incoming []*Release, now time.Time, initial bool) ([]*Release, Summary) {
	var summary Summary
	index := make(map[string]*Release, len(existing))
	for _, r := range existing {
		index[r.identity()] = r
	}

	present := make(map[string]bool, len(incoming))
	merged := append([]*Release(nil), existing...)
	for _, next := range incoming {
		key := next.identity()
		present[key] = true
		current, ok := index[key]
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
		}
		current.RawTitle = next.RawTitle
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
	return merged, summary
}

func changed(current, next *Release) bool {
	return current.RawTitle != next.RawTitle ||
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

func applyMatches(m matcher, list []*Release) error {
	if m == nil {
		return nil
	}
	targets := make([]*Release, 0, len(list))
	for _, r := range list {
		if r.Locked || r.Ignored {
			continue
		}
		if r.MatchStatus == catalog.StatusMatched && r.CanonicalGameID != nil && stableMatch(r.MatchMethod) {
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
	}

	pending := make([]catalog.Query, 0)
	pendingSeen := map[string]bool{}
	for _, r := range targets {
		if r.MatchStatus != catalog.StatusUnmatched || r.NormalizedTitle == "" {
			continue
		}
		if pendingSeen[r.NormalizedTitle] {
			continue
		}
		pendingSeen[r.NormalizedTitle] = true
		pending = append(pending, catalog.Query{Title: r.Title, Normalized: r.NormalizedTitle, Year: r.Year})
	}
	if len(pending) == 0 {
		return nil
	}
	provisioned, err := m.Provision(pending)
	if err != nil {
		return fmt.Errorf("provision matched games: %w", err)
	}
	for _, r := range targets {
		if r.MatchStatus != catalog.StatusUnmatched {
			continue
		}
		game, ok := provisioned[r.NormalizedTitle]
		if !ok {
			continue
		}
		id := game.ID
		r.CanonicalGameID = &id
		r.MatchStatus = catalog.StatusMatched
		r.MatchConfidence = 1
		r.MatchMethod = string(catalog.MethodProvisional)
	}
	return nil
}

// Матч по псевдониму или похожести держится на данных каталога, а они
// меняются: псевдоним могут выбросить, игру — переименовать. Такой матч
// пересчитывается на каждом обновлении, точный и ручной — нет.
func stableMatch(method string) bool {
	switch catalog.Method(method) {
	case catalog.MethodExactTitle, catalog.MethodExternalID, catalog.MethodOverride, catalog.MethodProvisional:
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
