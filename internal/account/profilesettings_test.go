package account

import (
	"encoding/json"
	"fmt"
	"testing"
)

func TestProfileAppearanceDefaultsAreIndependentOfShowcase(t *testing.T) {
	for _, showcase := range []string{"", `"showcase":[],`} {
		for _, tc := range []struct {
			name                                 string
			dim, position, wantDim, wantPosition int
		}{
			{"below range", -1, -20, 35, 50},
			{"above range", 101, 200, 35, 50},
			{"zero", 0, 0, 0, 0},
			{"upper bound", 100, 100, 100, 100},
		} {
			t.Run(showcase+tc.name, func(t *testing.T) {
				var user CurrentUser
				data := fmt.Sprintf(`{"profile":{%s"appearance":{"theme":"orbital","coverUrl":"https://cdn.test/cover.webp","coverDim":%d,"coverPosition":%d}}}`, showcase, tc.dim, tc.position)
				if err := json.Unmarshal([]byte(data), &user); err != nil {
					t.Fatal(err)
				}
				got := withProfileDefaults(user).Profile.Appearance
				if got.CoverDim != tc.wantDim || got.CoverPosition != tc.wantPosition || got.Theme != "orbital" || got.CoverURL != "https://cdn.test/cover.webp" || got.Accent != "#67d8ef" {
					t.Fatalf("appearance = %+v", got)
				}
			})
		}
	}
}

func TestProfileAppearanceKeepsCustomColours(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   string
		want ProfileAppearance
	}{
		{"colours and pure set", `{"theme":"custom","nameColor":"#ff8800","frameColor":"#070606","customPure":true}`,
			ProfileAppearance{Theme: "custom", NameColor: "#ff8800", FrameColor: "#070606", CustomPure: true}},
		{"colours absent", `{"theme":"custom"}`, ProfileAppearance{Theme: "custom"}},
		{"colours cleared", `{"theme":"custom","nameColor":"","frameColor":"","customPure":false}`, ProfileAppearance{Theme: "custom"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var got ProfileAppearance
			if err := json.Unmarshal([]byte(tc.in), &got); err != nil {
				t.Fatal(err)
			}
			if got.NameColor != tc.want.NameColor || got.FrameColor != tc.want.FrameColor || got.CustomPure != tc.want.CustomPure || got.Theme != tc.want.Theme {
				t.Fatalf("appearance = %+v, want colours %+v", got, tc.want)
			}
			again, err := json.Marshal(got)
			if err != nil {
				t.Fatal(err)
			}
			var back ProfileAppearance
			if err := json.Unmarshal(again, &back); err != nil {
				t.Fatal(err)
			}
			if back != got {
				t.Fatalf("round trip changed appearance: %+v -> %+v", got, back)
			}
		})
	}
}
