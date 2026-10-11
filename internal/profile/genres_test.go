package profile

import (
	"encoding/json"
	"reflect"
	"strconv"
	"testing"
)

// Copied from the backend's TestBreakdownGenres; the backend runs the same cases against BreakdownGenres.
// Change a case here and the backend table must change with it.
func TestComputeGenres(t *testing.T) {
	g := func(name string, share float64) GenreShare { return GenreShare{Name: name, Share: share} }
	tests := []struct {
		name  string
		games []PlayedGenres
		want  GenreBreakdown
	}{
		{
			// A splits 1800/1800 over RPG and Adventure, B is Shooter, C has no
			// genre, D has no playtime and is left out of the total.
			name: "contract fixture",
			games: []PlayedGenres{
				{Seconds: 3600, Genres: []string{"RPG", "Adventure"}},
				{Seconds: 1800, Genres: []string{"Shooter"}},
				{Seconds: 1800, Genres: nil},
				{Seconds: 0, Genres: []string{"Puzzle"}},
			},
			want: GenreBreakdown{Genres: []GenreShare{g("Adventure", 0.25), g("RPG", 0.25), g("Shooter", 0.25)}, Other: 0, Unknown: 0.25},
		},
		{
			name:  "no games",
			games: nil,
			want:  GenreBreakdown{Genres: []GenreShare{}},
		},
		{
			name:  "only games without playtime",
			games: []PlayedGenres{{Seconds: 0, Genres: []string{"RPG"}}, {Seconds: -5, Genres: []string{"RPG"}}},
			want:  GenreBreakdown{Genres: []GenreShare{}},
		},
		{
			name:  "everything unknown",
			games: []PlayedGenres{{Seconds: 100, Genres: nil}, {Seconds: 300, Genres: []string{}}},
			want:  GenreBreakdown{Genres: []GenreShare{}, Unknown: 1},
		},
		{
			name:  "a single genre takes everything",
			games: []PlayedGenres{{Seconds: 10, Genres: []string{"Strategy"}}, {Seconds: 30, Genres: []string{"Strategy"}}},
			want:  GenreBreakdown{Genres: []GenreShare{g("Strategy", 1)}},
		},
		{
			// Seven genres: the five biggest are listed, Racing and Sport sum
			// into Other. 1000+900+800+700+600+500+400 = 4900.
			name: "top five and the rest as other",
			games: []PlayedGenres{
				{Seconds: 1000, Genres: []string{"Action"}},
				{Seconds: 900, Genres: []string{"Adventure"}},
				{Seconds: 800, Genres: []string{"Puzzle"}},
				{Seconds: 700, Genres: []string{"Shooter"}},
				{Seconds: 600, Genres: []string{"Strategy"}},
				{Seconds: 500, Genres: []string{"Racing"}},
				{Seconds: 400, Genres: []string{"Sport"}},
			},
			want: GenreBreakdown{
				Genres: []GenreShare{g("Action", 0.204), g("Adventure", 0.184), g("Puzzle", 0.163), g("Shooter", 0.143), g("Strategy", 0.122)},
				Other:  0.184,
			},
		},
		{
			// Equal shares are ordered by name, and the cut falls inside the
			// tie: Alpha..Echo are listed, Foxtrot goes to Other.
			name: "ties are ordered by name before the cut",
			games: []PlayedGenres{
				{Seconds: 100, Genres: []string{"Foxtrot"}},
				{Seconds: 100, Genres: []string{"Echo"}},
				{Seconds: 100, Genres: []string{"Delta"}},
				{Seconds: 100, Genres: []string{"Charlie"}},
				{Seconds: 100, Genres: []string{"Bravo"}},
				{Seconds: 100, Genres: []string{"Alpha"}},
			},
			want: GenreBreakdown{
				Genres: []GenreShare{g("Alpha", 0.167), g("Bravo", 0.167), g("Charlie", 0.167), g("Delta", 0.167), g("Echo", 0.167)},
				Other:  0.167,
			},
		},
		{
			// Three genres at 1/3 each cannot be told apart by their float sums
			// (0.1+0.2 != 0.3 style), so the order must still be by name.
			name: "ties that differ only by float error",
			games: []PlayedGenres{
				{Seconds: 1, Genres: []string{"Zed", "Yak", "Xen"}},
				{Seconds: 10, Genres: []string{"Zed"}},
				{Seconds: 10, Genres: []string{"Yak"}},
				{Seconds: 10, Genres: []string{"Xen"}},
			},
			want: GenreBreakdown{Genres: []GenreShare{g("Xen", 0.333), g("Yak", 0.333), g("Zed", 0.333)}},
		},
		{
			name: "rounding happens once at the end",
			games: []PlayedGenres{
				{Seconds: 1, Genres: []string{"A"}},
				{Seconds: 1, Genres: []string{"B"}},
				{Seconds: 1, Genres: []string{"C"}},
				{Seconds: 1, Genres: []string{"D"}},
				{Seconds: 1, Genres: []string{"E"}},
				{Seconds: 1, Genres: []string{"F"}},
				{Seconds: 1, Genres: nil},
			},
			want: GenreBreakdown{
				Genres:  []GenreShare{g("A", 0.143), g("B", 0.143), g("C", 0.143), g("D", 0.143), g("E", 0.143)},
				Other:   0.143,
				Unknown: 0.143,
			},
		},
		{
			name: "a repeated or blank genre on one game counts once",
			games: []PlayedGenres{
				{Seconds: 100, Genres: []string{"RPG", "RPG", " ", "Indie"}},
			},
			want: GenreBreakdown{Genres: []GenreShare{g("Indie", 0.5), g("RPG", 0.5)}},
		},
		{
			name:  "a game whose only genres are blank is unknown",
			games: []PlayedGenres{{Seconds: 100, Genres: []string{"", "  "}}},
			want:  GenreBreakdown{Genres: []GenreShare{}, Unknown: 1},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ComputeGenres(tt.games)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("ComputeGenres() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestComputeGenresIsIndependentOfGameOrder(t *testing.T) {
	games := []PlayedGenres{
		{Seconds: 700, Genres: []string{"A", "B"}},
		{Seconds: 330, Genres: []string{"C"}},
		{Seconds: 110, Genres: []string{"B", "C", "D"}},
		{Seconds: 55, Genres: nil},
		{Seconds: 90, Genres: []string{"E"}},
		{Seconds: 91, Genres: []string{"F"}},
	}
	want := ComputeGenres(games)
	for shift := 1; shift < len(games); shift++ {
		rotated := append(append([]PlayedGenres{}, games[shift:]...), games[:shift]...)
		if got := ComputeGenres(rotated); !reflect.DeepEqual(got, want) {
			t.Fatalf("rotated by %d: %+v, want %+v", shift, got, want)
		}
	}
}

func TestComputeGenresJSON(t *testing.T) {
	encoded, err := json.Marshal(ComputeGenres(nil))
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != `{"genres":[],"other":0,"unknown":0}` {
		t.Fatalf("empty breakdown JSON = %s", encoded)
	}
	encoded, err = json.Marshal(ComputeGenres([]PlayedGenres{{Seconds: 1, Genres: []string{"RPG"}}}))
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != `{"genres":[{"name":"RPG","share":1}],"other":0,"unknown":0}` {
		t.Fatalf("breakdown JSON = %s", encoded)
	}
}

func TestComputeGenresSharesAddUp(t *testing.T) {
	games := make([]PlayedGenres, 0, 40)
	for i := 1; i <= 40; i++ {
		games = append(games, PlayedGenres{Seconds: int64(i * 37), Genres: []string{"G" + strconv.Itoa(i%9), "H" + strconv.Itoa(i%4)}})
	}
	got := ComputeGenres(games)
	sum := got.Other + got.Unknown
	for _, g := range got.Genres {
		sum += g.Share
	}
	if sum < 0.995 || sum > 1.005 {
		t.Fatalf("shares sum to %v, want 1 within rounding", sum)
	}
	if len(got.Genres) != maxGenres {
		t.Fatalf("listed genres = %d, want %d", len(got.Genres), maxGenres)
	}
}
