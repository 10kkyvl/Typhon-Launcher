package version

import "testing"

func TestParseKinds(t *testing.T) {
	cases := []struct {
		raw        string
		kind       Kind
		normalized string
		series     string
		comparable bool
	}{
		{"1.2", KindNumeric, "1.2", "", true},
		{"1.2.3", KindSemantic, "1.2.3", "", true},
		{"v1.2.3", KindSemantic, "1.2.3", "", true},
		{"Version 1.2.3", KindSemantic, "1.2.3", "", true},
		{"1.02", KindNumeric, "1.2", "", true},
		{"2.31", KindNumeric, "2.31", "", true},
		{"Build 123456", KindBuild, "build 123456", "", true},
		{"build-123456", KindBuild, "build 123456", "", true},
		{"Update 9", KindNumeric, "update 9", "update", true},
		{"Patch 4", KindNumeric, "patch 4", "patch", true},
		{"2026.08.20", KindDate, "2026.8.20", "", true},
		{"1.2.3-beta", KindSemantic, "1.2.3+beta", "", false},
		{"goty", KindUnknown, "goty", "", false},
		{"", KindUnknown, "", "", false},
	}
	for _, c := range cases {
		got := Parse(c.raw)
		if got.Kind != c.kind {
			t.Errorf("Parse(%q).Kind = %q, want %q", c.raw, got.Kind, c.kind)
		}
		if got.Normalized != c.normalized {
			t.Errorf("Parse(%q).Normalized = %q, want %q", c.raw, got.Normalized, c.normalized)
		}
		if got.Series != c.series {
			t.Errorf("Parse(%q).Series = %q, want %q", c.raw, got.Series, c.series)
		}
		if got.Comparable != c.comparable {
			t.Errorf("Parse(%q).Comparable = %v, want %v", c.raw, got.Comparable, c.comparable)
		}
	}
}

func TestCompareOrder(t *testing.T) {
	cases := []struct {
		left, right string
		want        int
	}{
		{"1.9", "1.10", -1},
		{"2.21", "2.31", -1},
		{"1.2.3", "1.2.4", -1},
		{"Build 12000", "Build 13000", -1},
		{"1.2", "1.2.0", 0},
		{"1.3", "1.2.9", 1},
		{"Update 9", "Update 10", -1},
		{"2026.08.20", "2026.09.01", -1},
	}
	for _, c := range cases {
		got, ok := Compare(Parse(c.left), Parse(c.right))
		if !ok {
			t.Fatalf("Compare(%q, %q) not comparable", c.left, c.right)
		}
		if got != c.want {
			t.Errorf("Compare(%q, %q) = %d, want %d", c.left, c.right, got, c.want)
		}
	}
}

func TestNotComparable(t *testing.T) {
	cases := [][2]string{
		{"unknownA", "unknownB"},
		{"Update 9", "1.9"},
		{"2026.08.20", "1.2.3"},
		{"Build 100", "1.0.0"},
		{"1.2.3-beta", "1.2.4"},
		{"", "1.0"},
		{"Patch 4", "Update 4"},
	}
	for _, c := range cases {
		if _, ok := Compare(Parse(c[0]), Parse(c[1])); ok {
			t.Errorf("Compare(%q, %q) unexpectedly comparable", c[0], c[1])
		}
	}
}

func TestNewer(t *testing.T) {
	newer, ok := Newer(Parse("2.31"), Parse("2.21"))
	if !ok || !newer {
		t.Fatalf("Newer(2.31, 2.21) = %v, %v", newer, ok)
	}
	newer, ok = Newer(Parse("2.21"), Parse("2.31"))
	if !ok || newer {
		t.Fatalf("Newer(2.21, 2.31) = %v, %v", newer, ok)
	}
	if _, ok := Newer(Parse("goty"), Parse("2.31")); ok {
		t.Fatal("Newer with unknown version must not be comparable")
	}
}
