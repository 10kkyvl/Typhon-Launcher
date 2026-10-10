package profile

import (
	"math"
	"sort"
)

const (
	maxGenres = 5
	// Splitting playtime across genres accumulates float error far below one rounding step; ties must survive it.
	shareEpsilon = 1e-9
)

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

// ComputeGenres is mirrored by the backend and both are tested on the same fixture, so the algorithm must not drift.
func ComputeGenres(played []PlayedGenres) GenreBreakdown {
	bySeconds := map[string]float64{}
	var total, unknown float64
	for _, p := range played {
		if p.Seconds <= 0 {
			continue
		}
		seconds := float64(p.Seconds)
		total += seconds
		names := uniqueNames(p.Genres)
		if len(names) == 0 {
			unknown += seconds
			continue
		}
		part := seconds / float64(len(names))
		for _, name := range names {
			bySeconds[name] += part
		}
	}
	if total == 0 {
		return GenreBreakdown{Genres: []GenreShare{}}
	}

	shares := make([]GenreShare, 0, len(bySeconds))
	for name, seconds := range bySeconds {
		shares = append(shares, GenreShare{Name: name, Share: seconds / total})
	}
	sort.Slice(shares, func(i, j int) bool {
		if math.Abs(shares[i].Share-shares[j].Share) > shareEpsilon {
			return shares[i].Share > shares[j].Share
		}
		return shares[i].Name < shares[j].Name
	})

	var other float64
	if len(shares) > maxGenres {
		for _, rest := range shares[maxGenres:] {
			other += rest.Share
		}
		shares = shares[:maxGenres]
	}
	for i := range shares {
		shares[i].Share = round3(shares[i].Share)
	}
	return GenreBreakdown{Genres: shares, Other: round3(other), Unknown: round3(unknown / total)}
}

func uniqueNames(names []string) []string {
	seen := make(map[string]struct{}, len(names))
	out := make([]string, 0, len(names))
	for _, name := range names {
		if name == "" {
			continue
		}
		if _, dup := seen[name]; dup {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	return out
}

func round3(v float64) float64 {
	return math.Round(v*1000) / 1000
}
