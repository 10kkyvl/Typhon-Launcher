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
