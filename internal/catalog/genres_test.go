package catalog

import (
	"reflect"
	"testing"
)

func TestCanonicalGenresUnifyProvidersAndSavedLanguages(t *testing.T) {
	got := canonicalGenres([]string{"RPG", "Role-playing (RPG)", "Ролевые игры", "Simulation", "Sports", "Экшены", "Приключенческие игры", "Early Access", "Utilities", " Visual Novel "})
	want := []string{"Role-playing (RPG)", "Simulator", "Sport", "Action", "Adventure", "Visual Novel"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestRecommendationGenreChipsAndLegacyProfile(t *testing.T) {
	for _, genre := range []string{"Indie", "Adventure", "Visual Novel", "Simulator", "Role-playing (RPG)", "Shooter", "Strategy", "Fighting"} {
		game := Game{ID: "candidate", Title: "Candidate", Genres: []string{genre}}
		got := rankGames([]Game{game}, RecommendationProfile{}, nil, RecommendationPreferences{}, false, nil, GameQuery{Genre: genre})
		if len(got) != 1 {
			t.Errorf("chip %s removed its candidate", genre)
		}
	}
	games := []Game{{ID: "owned", Genres: []string{"Ролевые игры", "RPG"}}}
	evidence := buildEvidence(games, []RecommendationLibraryItem{{CanonicalGameID: "owned", Favorite: true}})
	if len(evidence.Genres) != 1 || evidence.Genres["Role-playing (RPG)"] != 3 {
		t.Fatalf("legacy evidence=%+v", evidence)
	}
	profile := profileFromEvidence(evidence, RecommendationPreferences{})
	ranked := rankGames([]Game{{ID: "candidate", Genres: []string{"Role-playing (RPG)"}}}, profile, nil, RecommendationPreferences{}, false, nil, GameQuery{Genre: "RPG"})
	if len(ranked) != 1 || ranked[0].Reason != "genre" {
		t.Fatalf("legacy affinity=%+v", ranked)
	}
}
