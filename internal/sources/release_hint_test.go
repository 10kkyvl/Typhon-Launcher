package sources

import (
	"reflect"
	"testing"
)

func TestReleaseHintsKeepExplicitChaptersAndEditions(t *testing.T) {
	for _, tc := range []struct {
		raw, hint string
		want      []string
	}{
		{"Deathbloom — Chapter 1", "Deathbloom", []string{"Deathbloom — Chapter 1", "Deathbloom"}},
		{"Deathbloom — Chapter 2", "Deathbloom", []string{"Deathbloom — Chapter 2", "Deathbloom"}},
		{"The Dark Inside Me — Chapter 2", "The Dark Inside Me", []string{"The Dark Inside Me — Chapter 2", "The Dark Inside Me"}},
		{"The Persistence — Enhanced", "The Persistence", []string{"The Persistence — Enhanced", "The Persistence"}},
		{"Super Seducer 2 — Enhanced", "Super Seducer 2", []string{"Super Seducer 2 — Enhanced", "Super Seducer 2"}},
		{"Example: Remastered v1.2", "Example", []string{"Example Remastered"}},
		{"Example — v1.2 - Deluxe Edition", "Example", []string{"Example Deluxe Edition", "Example"}},
		{"Example — [Episode 1]", "Example", []string{"Example — [Episode 1]", "Example"}},
		{"Example — Chapter 1-4", "Example", []string{"Example"}},
		{"Example — Chapter One", "Example", []string{"Example"}},
		{"Different Download Label v1.2", "Example", []string{"Example"}},
		{"Example + Other Game", "Example", []string{"Example"}},
		{"Example — build notes", "Example", []string{"Example"}},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			r := &Release{RawTitle: tc.raw, GameHint: tc.hint}
			reparseRelease(r)
			if got := remoteReleaseNames(r); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("names=%q, want %q", got, tc.want)
			}
			keys, queries := remoteReleaseQueries([]*Release{r})
			if len(keys) != 1 || keys[0] != remoteQueryKey(r) || !reflect.DeepEqual(queries[0].Titles, tc.want) {
				t.Fatalf("lookup key/query diverged: %q %+v", keys, queries)
			}
		})
	}
}
