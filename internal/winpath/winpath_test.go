package winpath

import "testing"

func TestReserved(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{"NUL", true},
		{"con", true},
		{"aux.txt", true},
		{"LPT9.log", true},
		{"COM1", true},
		{"com¹.exe", true},
		{"CONOUT$", true},
		{"prn .txt", true},
		{"setup.exe.", true},
		{"setup.exe ", true},
		{"COM10", false},
		{"LPTX", false},
		{"auxiliary.txt", false},
		{"console.exe", false},
		{"Game.exe", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := Reserved(tc.name); got != tc.want {
			t.Errorf("Reserved(%q) = %v, want %v", tc.name, got, tc.want)
		}
	}
}
