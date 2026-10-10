package profile

import (
	"fmt"
	"reflect"
	"testing"
)

func TestComputeGenres(t *testing.T) {
	many := func(n int) []PlayedGenres {
		var out []PlayedGenres
		for i := 0; i < n; i++ {
			out = append(out, PlayedGenres{Seconds: int64(100 * (n - i)), Genres: []string{fmt.Sprintf("G%d", i)}})
		}
		return out
	}
	tests := []struct {
		name   string
		played []PlayedGenres
		want   GenreBreakdown
	}{
		{
			name: "shared fixture",
			played: []PlayedGenres{
				{Seconds: 3600, Genres: []string{"RPG", "Adventure"}},
				{Seconds: 1800, Genres: []string{"Shooter"}},
				{Seconds: 1800},
				{Seconds: 0, Genres: []string{"Puzzle"}},
			},
			want: GenreBreakdown{
				Genres:  []GenreShare{{"Adventure", 0.25}, {"RPG", 0.25}, {"Shooter", 0.25}},
				Unknown: 0.25,
			},
		},
		{name: "nothing played", played: nil, want: GenreBreakdown{Genres: []GenreShare{}}},
		{name: "only zero playtime", played: []PlayedGenres{{Seconds: 0, Genres: []string{"RPG"}}}, want: GenreBreakdown{Genres: []GenreShare{}}},
		{name: "negative playtime is ignored", played: []PlayedGenres{{Seconds: -5, Genres: []string{"RPG"}}, {Seconds: 10, Genres: []string{"Puzzle"}}}, want: GenreBreakdown{Genres: []GenreShare{{"Puzzle", 1}}}},
		{name: "only unknown", played: []PlayedGenres{{Seconds: 60}}, want: GenreBreakdown{Genres: []GenreShare{}, Unknown: 1}},
		{
			name:   "duplicate and empty genre names do not skew the split",
			played: []PlayedGenres{{Seconds: 100, Genres: []string{"RPG", "RPG", ""}}},
			want:   GenreBreakdown{Genres: []GenreShare{{"RPG", 1}}},
		},
		{
			name:   "only empty names count as unknown",
			played: []PlayedGenres{{Seconds: 100, Genres: []string{"", ""}}},
			want:   GenreBreakdown{Genres: []GenreShare{}, Unknown: 1},
		},
		{
			name: "rounding happens once at the end",
			played: []PlayedGenres{
				{Seconds: 1, Genres: []string{"A"}},
				{Seconds: 1, Genres: []string{"B"}},
				{Seconds: 1, Genres: []string{"C"}},
			},
			want: GenreBreakdown{Genres: []GenreShare{{"A", 0.333}, {"B", 0.333}, {"C", 0.333}}},
		},
		{
			name:   "top five and the rest folds into other",
			played: many(7),
			// totals 100*(7+6+5+4+3+2+1)=2800; G5 and G6 are 200 and 100.
			want: GenreBreakdown{
				Genres: []GenreShare{{"G0", 0.25}, {"G1", 0.214}, {"G2", 0.179}, {"G3", 0.143}, {"G4", 0.107}},
				Other:  0.107,
			},
		},
		{
			name: "ties break by name when the cut falls between equals",
			played: []PlayedGenres{
				{Seconds: 100, Genres: []string{"F"}}, {Seconds: 100, Genres: []string{"E"}}, {Seconds: 100, Genres: []string{"D"}},
				{Seconds: 100, Genres: []string{"C"}}, {Seconds: 100, Genres: []string{"B"}}, {Seconds: 100, Genres: []string{"A"}},
			},
			want: GenreBreakdown{
				Genres: []GenreShare{{"A", 0.167}, {"B", 0.167}, {"C", 0.167}, {"D", 0.167}, {"E", 0.167}},
				Other:  0.167,
			},
		},
		{
			name: "float noise from uneven splits does not break a tie",
			played: []PlayedGenres{
				{Seconds: 100, Genres: []string{"Z", "Y", "X"}},
				{Seconds: 100, Genres: []string{"W"}},
			},
			want: GenreBreakdown{Genres: []GenreShare{{"W", 0.5}, {"X", 0.167}, {"Y", 0.167}, {"Z", 0.167}}},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ComputeGenres(tc.played)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}
