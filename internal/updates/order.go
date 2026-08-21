package updates

import (
	"sort"

	"typhon/internal/sources"
	"typhon/internal/version"
)

type Candidate struct {
	Release sources.Release
	Version version.Version
}

func candidatesOf(list []sources.Release) []Candidate {
	out := make([]Candidate, 0, len(list))
	for _, r := range list {
		out = append(out, Candidate{Release: r, Version: version.Parse(releaseVersion(r))})
	}
	return out
}

func releaseVersion(r sources.Release) string {
	if r.ToVersion != "" {
		return r.ToVersion
	}
	return r.Version
}

// newerThan reports whether a should be ordered before b, newest first.
func newerThan(a, b Candidate) bool {
	if a.Release.Sequence != 0 && b.Release.Sequence != 0 && a.Release.Sequence != b.Release.Sequence {
		return a.Release.Sequence > b.Release.Sequence
	}
	if cmp, ok := version.Compare(a.Version, b.Version); ok && cmp != 0 {
		return cmp > 0
	}
	switch {
	case a.Release.UploadedAt != nil && b.Release.UploadedAt != nil:
		if !a.Release.UploadedAt.Equal(*b.Release.UploadedAt) {
			return a.Release.UploadedAt.After(*b.Release.UploadedAt)
		}
	case a.Release.UploadedAt != nil:
		return true
	case b.Release.UploadedAt != nil:
		return false
	}
	if !a.Release.FirstSeenAt.Equal(b.Release.FirstSeenAt) {
		return a.Release.FirstSeenAt.After(b.Release.FirstSeenAt)
	}
	return a.Release.ID < b.Release.ID
}

func OrderReleases(list []sources.Release) []sources.Release {
	ordered := candidatesOf(list)
	sort.SliceStable(ordered, func(i, j int) bool { return newerThan(ordered[i], ordered[j]) })
	out := make([]sources.Release, 0, len(ordered))
	for _, c := range ordered {
		out = append(out, c.Release)
	}
	return out
}
