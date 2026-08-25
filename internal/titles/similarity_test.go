package titles

import "testing"

func TestSimilarityExactMatch(t *testing.T) {
	if s := Similarity("prey", "prey"); s != 1 {
		t.Errorf("Similarity(prey, prey) = %v, want 1", s)
	}
	if s := Similarity("cyberpunk 2077", "cyberpunk 2077"); s != 1 {
		t.Errorf("Similarity(cyberpunk 2077, cyberpunk 2077) = %v, want 1", s)
	}
}

func TestSimilaritySubsetTitles(t *testing.T) {
	s := Similarity("cyberpunk 2077", "cyberpunk 2077 phantom liberty")
	if s <= 0.6 || s >= 1.0 {
		t.Errorf("Similarity(cyberpunk 2077, cyberpunk 2077 phantom liberty) = %v, want in (0.6, 1.0)", s)
	}
}

func TestSimilarityDoomEternal(t *testing.T) {
	s := Similarity("doom", "doom eternal")
	if s >= 0.9 {
		t.Errorf("Similarity(doom, doom eternal) = %v, want < 0.9", s)
	}
	if s <= 0 {
		t.Errorf("Similarity(doom, doom eternal) = %v, want > 0", s)
	}
}

func TestSimilarityUnrelated(t *testing.T) {
	s := Similarity("prey", "doom")
	if s >= 0.5 {
		t.Errorf("Similarity(prey, doom) = %v, want < 0.5", s)
	}
}

func TestSimilarityNoPanicOnEmpty(t *testing.T) {
	cases := [][2]string{
		{"", ""},
		{"", "prey"},
		{"prey", ""},
		{"   ", "   "},
	}
	for _, c := range cases {
		_ = Similarity(c[0], c[1])
	}
}

func TestSimilarityBounds(t *testing.T) {
	inputs := []string{
		"Cyberpunk 2077",
		"cyberpunk 2077 ultimate edition",
		"The Witcher 3 Wild Hunt",
		"a completely different title with many words",
		"",
	}
	for _, a := range inputs {
		for _, b := range inputs {
			s := Similarity(a, b)
			if s < 0 || s > 1 {
				t.Errorf("Similarity(%q, %q) = %v out of bounds", a, b, s)
			}
		}
	}
}

func TestTokenSet(t *testing.T) {
	got := TokenSet("cyberpunk 2077 ultimate edition")
	want := []string{"2077", "cyberpunk", "edition", "ultimate"}
	if len(got) != len(want) {
		t.Fatalf("TokenSet length = %d, want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("TokenSet[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestTokenSetEmpty(t *testing.T) {
	if got := TokenSet(""); got != nil {
		t.Errorf("TokenSet(\"\") = %v, want nil", got)
	}
}

func TestLevenshteinNoPanicOnLongInput(t *testing.T) {
	long := make([]byte, 5000)
	for i := range long {
		long[i] = byte('a' + i%26)
	}
	_ = Similarity(string(long), "short title")
}
