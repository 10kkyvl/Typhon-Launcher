package metadata

import (
	"errors"
	"testing"
)

func TestPickRefusesAmbiguousCandidates(t *testing.T) {
	cases := []struct {
		name       string
		title      string
		year       int
		candidates []Candidate
		want       string
		wantErr    error
	}{
		{
			name:  "prey without year stays unresolved",
			title: "Prey",
			candidates: []Candidate{
				{ProviderID: "2657", Title: "Prey", ReleaseYear: 2017},
				{ProviderID: "7", Title: "Prey", ReleaseYear: 2006},
			},
			wantErr: ErrAmbiguous,
		},
		{
			name:  "prey with year picks the matching release",
			title: "Prey",
			year:  2006,
			candidates: []Candidate{
				{ProviderID: "2657", Title: "Prey", ReleaseYear: 2017},
				{ProviderID: "7", Title: "Prey", ReleaseYear: 2006},
			},
			want: "7",
		},
		{
			name:  "doom series without year stays unresolved",
			title: "DOOM",
			candidates: []Candidate{
				{ProviderID: "1", Title: "Doom", ReleaseYear: 1993},
				{ProviderID: "2", Title: "DOOM", ReleaseYear: 2016},
				{ProviderID: "3", Title: "Doom Eternal", ReleaseYear: 2020},
			},
			wantErr: ErrAmbiguous,
		},
		{
			name:  "doom with year picks the reboot",
			title: "DOOM",
			year:  2016,
			candidates: []Candidate{
				{ProviderID: "1", Title: "Doom", ReleaseYear: 1993},
				{ProviderID: "2", Title: "DOOM", ReleaseYear: 2016},
				{ProviderID: "3", Title: "Doom Eternal", ReleaseYear: 2020},
			},
			want: "2",
		},
		{
			name:  "single exact title matches",
			title: "Dishonored 2",
			candidates: []Candidate{
				{ProviderID: "11", Title: "Dishonored 2", ReleaseYear: 2016},
			},
			want: "11",
		},
		{
			name:  "loose title stays unresolved",
			title: "Prey",
			candidates: []Candidate{
				{ProviderID: "9", Title: "Prey for the Gods", ReleaseYear: 2021},
			},
			wantErr: ErrAmbiguous,
		},
		{
			name:       "no candidates",
			title:      "Prey",
			candidates: nil,
			wantErr:    ErrNoMatch,
		},
		{
			name:  "empty canonical title never auto matches",
			title: "",
			candidates: []Candidate{
				{ProviderID: "1", Title: "Prey", ReleaseYear: 2017},
			},
			wantErr: ErrAmbiguous,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := pick(tc.title, tc.year, tc.candidates)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err = %v, want %v", err, tc.wantErr)
				}
				if got.ProviderID != "" {
					t.Fatalf("candidate %q returned together with an error", got.ProviderID)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.ProviderID != tc.want {
				t.Fatalf("provider id = %q, want %q", got.ProviderID, tc.want)
			}
		})
	}
}

func TestRankOrdersByConfidence(t *testing.T) {
	ranked := rank("Prey", 2017, []Candidate{
		{ProviderID: "7", Title: "Prey", ReleaseYear: 2006},
		{ProviderID: "9", Title: "Prey for the Gods", ReleaseYear: 2021},
		{ProviderID: "2657", Title: "Prey", ReleaseYear: 2017},
	})
	if len(ranked) != 3 {
		t.Fatalf("ranked = %d, want 3", len(ranked))
	}
	if ranked[0].ProviderID != "2657" {
		t.Fatalf("top = %q, want 2657", ranked[0].ProviderID)
	}
	for i := 1; i < len(ranked); i++ {
		if ranked[i-1].Confidence < ranked[i].Confidence {
			t.Fatalf("candidates are not sorted by confidence: %+v", ranked)
		}
	}
	if ranked[0].Confidence <= ranked[1].Confidence {
		t.Fatalf("matching year did not win: %+v", ranked)
	}
}

func TestConfidenceStaysInRange(t *testing.T) {
	cases := []Candidate{
		{Title: "Prey", ReleaseYear: 2017},
		{Title: "Prey", ReleaseYear: 1990},
		{Title: "", ReleaseYear: 0},
		{Title: "совершенно другая игра", ReleaseYear: 2017},
	}
	for _, c := range cases {
		got := confidence("prey", 2017, c)
		if got < 0 || got > 1 {
			t.Fatalf("confidence(%q) = %v, out of range", c.Title, got)
		}
	}
}
