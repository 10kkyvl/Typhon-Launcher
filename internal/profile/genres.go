package profile

import (
	"math"
	"sort"
	"strings"
)

const maxGenres = 5

type GenreShare struct {
	Name  string  `json:"name"`
	Share float64 `json:"share"`
}

type GenreBreakdown struct {
	Genres  []GenreShare `json:"genres"`
	Other   float64      `json:"other"`
	Unknown float64      `json:"unknown"`
}

type PlayedGenres struct {
	Seconds int64
	Genres  []string
}

// shareKey makes shares that differ only by float error compare equal, so ties fall back to the name
// with a transitive ordering.
func shareKey(v float64) int64 { return int64(math.Round(v * 1e9)) }

func round3(v float64) float64 { return math.Round(v*1000) / 1000 }

// ComputeGenres mirrors the backend's BreakdownGenres; both run the same table test, so the algorithm must not drift.
func ComputeGenres(played []PlayedGenres) GenreBreakdown {
	out := GenreBreakdown{Genres: []GenreShare{}}
	var total, unknown float64
	seconds := map[string]float64{}
	for _, p := range played {
		if p.Seconds <= 0 {
			continue
		}
		total += float64(p.Seconds)
		names := distinctGenres(p.Genres)
		if len(names) == 0 {
			unknown += float64(p.Seconds)
			continue
		}
		part := float64(p.Seconds) / float64(len(names))
		for _, name := range names {
			seconds[name] += part
		}
	}
	if total == 0 {
		return out
	}

	ranked := make([]GenreShare, 0, len(seconds))
	for name, sec := range seconds {
		ranked = append(ranked, GenreShare{Name: name, Share: sec / total})
	}
	sort.Slice(ranked, func(i, j int) bool {
		a, b := shareKey(ranked[i].Share), shareKey(ranked[j].Share)
		if a != b {
			return a > b
		}
		return ranked[i].Name < ranked[j].Name
	})

	var other float64
	for i, g := range ranked {
		if i < maxGenres {
			out.Genres = append(out.Genres, GenreShare{Name: g.Name, Share: round3(g.Share)})
			continue
		}
		other += g.Share
	}
	out.Other = round3(other)
	out.Unknown = round3(unknown / total)
	return out
}

func distinctGenres(names []string) []string {
	seen := make(map[string]bool, len(names))
	out := make([]string, 0, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	return out
}
