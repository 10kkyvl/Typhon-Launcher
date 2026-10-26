package tray

import (
	"reflect"
	"strings"
	"testing"
)

var testLabels = Labels{Recent: "Recent", Playing: "playing", Stop: "Stop", Open: "Open", Quit: "Quit"}

func footer() []Item {
	return []Item{{Kind: KindOpen, Label: "Open"}, {Kind: KindSeparator}, {Kind: KindQuit, Label: "Quit"}}
}

func recent(items ...Item) []Item {
	out := []Item{{Kind: KindHeader, Label: "Recent"}}
	out = append(out, items...)
	out = append(out, Item{Kind: KindSeparator})
	return append(out, footer()...)
}

func TestBuildItems(t *testing.T) {
	cases := []struct {
		name  string
		games []Game
		want  []Item
	}{
		{"no games has no recent block", nil, footer()},
		{
			"idle games",
			[]Game{{ID: "a", Title: "Alpha"}, {ID: "b", Title: "Beta"}},
			recent(Item{Kind: KindGame, Label: "Alpha", GameID: "a"}, Item{Kind: KindGame, Label: "Beta", GameID: "b"}),
		},
		{
			"running game gets a status and a stop item right under it",
			[]Game{{ID: "a", Title: "Alpha", Running: true}, {ID: "b", Title: "Beta"}},
			recent(
				Item{Kind: KindGame, Label: "Alpha — playing", GameID: "a", Disabled: true},
				Item{Kind: KindStop, Label: "Stop", GameID: "a"},
				Item{Kind: KindGame, Label: "Beta", GameID: "b"},
			),
		},
		{
			"only the first five games are shown",
			[]Game{
				{ID: "1", Title: "1"}, {ID: "2", Title: "2"}, {ID: "3", Title: "3"},
				{ID: "4", Title: "4"}, {ID: "5", Title: "5"}, {ID: "6", Title: "6"},
			},
			recent(
				Item{Kind: KindGame, Label: "1", GameID: "1"}, Item{Kind: KindGame, Label: "2", GameID: "2"},
				Item{Kind: KindGame, Label: "3", GameID: "3"}, Item{Kind: KindGame, Label: "4", GameID: "4"},
				Item{Kind: KindGame, Label: "5", GameID: "5"},
			),
		},
		{
			"title with control characters and runs of spaces is flattened",
			[]Game{{ID: "a", Title: "  Dark\tSouls\r\n  III  "}},
			recent(Item{Kind: KindGame, Label: "Dark Souls III", GameID: "a"}),
		},
		{
			"long title is cut by runes",
			[]Game{{ID: "a", Title: strings.Repeat("я", 60)}},
			recent(Item{Kind: KindGame, Label: strings.Repeat("я", 39) + "…", GameID: "a"}),
		},
		{
			"blank title falls back to the id",
			[]Game{{ID: "a1", Title: " \t "}},
			recent(Item{Kind: KindGame, Label: "a1", GameID: "a1"}),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := BuildItems(testLabels, tc.games)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("BuildItems = %+v\nwant         %+v", got, tc.want)
			}
		})
	}
}

func TestNativeLabel(t *testing.T) {
	cases := []struct {
		name    string
		label   string
		windows bool
		want    string
	}{
		{"windows escapes the mnemonic marker", "Tom & Jerry", true, "Tom && Jerry"},
		{"windows leaves plain text", "Alpha", true, "Alpha"},
		{"other platforms show the ampersand as is", "Tom & Jerry", false, "Tom & Jerry"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := NativeLabel(tc.label, tc.windows); got != tc.want {
				t.Fatalf("NativeLabel(%q, %v) = %q, want %q", tc.label, tc.windows, got, tc.want)
			}
		})
	}
}
