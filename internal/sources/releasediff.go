package sources

import (
	"context"
	"slices"
)

// releasesChanged reports whether list would write a different releases file
// than previous. A refresh that changes nothing must not rewrite it, so the
// comparison covers every persisted field except LastSeenAt, which merge moves
// forward on every pass and which is only read to order removed entries.
func releasesChanged(ctx context.Context, previous, list []*Release) (bool, error) {
	if len(previous) != len(list) {
		return true, nil
	}
	for i := range list {
		if err := interrupted(ctx, i); err != nil {
			return false, err
		}
		if !sameRelease(previous[i], list[i]) {
			return true, nil
		}
	}
	return false, nil
}

func sameRelease(a, b *Release) bool {
	return a.GameHint == b.GameHint &&
		a.ID == b.ID &&
		a.SourceID == b.SourceID &&
		a.DistributionID == b.DistributionID &&
		a.Kind == b.Kind &&
		a.RawTitle == b.RawTitle &&
		a.Title == b.Title &&
		a.NormalizedTitle == b.NormalizedTitle &&
		sameString(a.CanonicalGameID, b.CanonicalGameID) &&
		a.Version == b.Version &&
		a.RawVersion == b.RawVersion &&
		a.FromVersion == b.FromVersion &&
		a.ToVersion == b.ToVersion &&
		a.Sequence == b.Sequence &&
		a.Edition == b.Edition &&
		slices.Equal(a.Languages, b.Languages) &&
		a.Year == b.Year &&
		slices.Equal(a.Tags, b.Tags) &&
		a.Repacker == b.Repacker &&
		a.DLCCount == b.DLCCount &&
		a.Size == b.Size &&
		a.SizeUnknown == b.SizeUnknown &&
		sameTime(a.UploadedAt, b.UploadedAt) &&
		slices.Equal(a.URIs, b.URIs) &&
		a.InfoHash == b.InfoHash &&
		a.MatchStatus == b.MatchStatus &&
		a.MatchConfidence == b.MatchConfidence &&
		a.MatchMethod == b.MatchMethod &&
		a.MatchEpoch == b.MatchEpoch &&
		a.Availability == b.Availability &&
		a.Locked == b.Locked &&
		a.Ignored == b.Ignored &&
		a.New == b.New &&
		a.FirstSeenAt.Equal(b.FirstSeenAt) &&
		a.CreatedAt.Equal(b.CreatedAt) &&
		sameRemoteMatch(a.RemoteMatch, b.RemoteMatch)
}

func sameString(a, b *string) bool {
	switch {
	case a == nil && b == nil:
		return true
	case a == nil || b == nil:
		return false
	default:
		return *a == *b
	}
}

func sameRemoteMatch(a, b *RemoteMatch) bool {
	switch {
	case a == nil && b == nil:
		return true
	case a == nil || b == nil:
		return false
	default:
		return a.Key == b.Key &&
			a.At.Equal(b.At) &&
			a.Status == b.Status &&
			a.GameID == b.GameID &&
			a.Confidence == b.Confidence &&
			a.Method == b.Method &&
			a.Rev == b.Rev &&
			a.RulesVersion == b.RulesVersion &&
			a.Reason == b.Reason &&
			slices.EqualFunc(a.Candidates, b.Candidates, func(x, y RemoteCandidate) bool {
				return x.ID == y.ID && x.Title == y.Title && x.Year == y.Year && slices.Equal(x.Platforms, y.Platforms)
			})
	}
}
