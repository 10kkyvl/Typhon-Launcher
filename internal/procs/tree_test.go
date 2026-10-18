package procs

import (
	"slices"
	"testing"
)

func TestAncestors(t *testing.T) {
	cases := []struct {
		name    string
		parents map[uint32]uint32
		pid     uint32
		want    []uint32
	}{
		{"chain", map[uint32]uint32{30: 20, 20: 10, 10: 4, 4: 0}, 30, []uint32{20, 10, 4}},
		{"unknown pid", map[uint32]uint32{20: 10}, 99, nil},
		{"orphan parent missing", map[uint32]uint32{30: 20}, 30, []uint32{20}},
		{"self parent", map[uint32]uint32{5: 5}, 5, nil},
		{"loop", map[uint32]uint32{1: 2, 2: 3, 3: 1}, 1, []uint32{2, 3}},
		{"empty", nil, 7, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Ancestors(c.parents, c.pid)
			if !slices.Equal(got, c.want) {
				t.Fatalf("Ancestors = %v, want %v", got, c.want)
			}
		})
	}
}

func TestAncestorsDepthLimit(t *testing.T) {
	parents := map[uint32]uint32{}
	for pid := uint32(1); pid <= 100; pid++ {
		parents[pid] = pid + 1
	}
	if got := Ancestors(parents, 1); len(got) != MaxAncestorDepth {
		t.Fatalf("len(Ancestors) = %d, want %d", len(got), MaxAncestorDepth)
	}
}

func TestDescendants(t *testing.T) {
	cases := []struct {
		name    string
		parents map[uint32]uint32
		root    uint32
		want    []uint32
	}{
		{"deepest first", map[uint32]uint32{20: 10, 30: 20, 40: 30, 99: 7}, 10, []uint32{40, 30, 20}},
		{"siblings", map[uint32]uint32{20: 10, 21: 10, 30: 20}, 10, []uint32{30, 20, 21}},
		{"none", map[uint32]uint32{20: 10}, 99, nil},
		{"loop through root", map[uint32]uint32{10: 20, 20: 10}, 10, []uint32{20}},
		{"pid zero ignored", map[uint32]uint32{0: 10}, 10, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Descendants(c.parents, c.root)
			if !slices.Equal(got, c.want) {
				t.Fatalf("Descendants = %v, want %v", got, c.want)
			}
		})
	}
}
