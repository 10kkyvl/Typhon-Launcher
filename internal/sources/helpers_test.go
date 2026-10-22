package sources

import (
	"context"
	"time"

	"typhon/internal/sources/feed"
)

func parseEntries(sourceID string, entries []feed.Entry, now time.Time) []*Release {
	list, err := parseFeedEntries(context.Background(), sourceID, entries, now, newTitleMemo(), nil)
	if err != nil {
		panic(err)
	}
	return list
}

func merge(existing, incoming []*Release, now time.Time, initial bool) ([]*Release, Summary) {
	list, summary, err := mergeReleases(context.Background(), existing, incoming, now, initial)
	if err != nil {
		panic(err)
	}
	return list, summary
}

func reparseRelease(r *Release) { newTitleMemo().reparse(r) }

func remoteReleaseNames(r *Release) []string { return newTitleMemo().releaseNames(r) }

func remoteQueryKey(r *Release) string { return newTitleMemo().queryKey(r) }
