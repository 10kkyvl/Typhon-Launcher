package account

import (
	"encoding/json"
	"slices"
	"testing"
)

func TestUpdateProfileSendsFeedHiddenOnlyWhenSet(t *testing.T) {
	tests := []struct {
		name    string
		profile ProfileSettings
		want    string
	}{
		{"nil list is not sent", ProfileSettings{Visibility: VisibilityFriends}, ""},
		{"empty list is not sent", ProfileSettings{Visibility: VisibilityFriends, FeedHidden: []string{}}, ""},
		{"kinds are sent", ProfileSettings{Visibility: VisibilityFriends, FeedHidden: []string{"played", "reviewed"}}, `["played","reviewed"]`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sent := sentProfile(t, Patch{Profile: &tt.profile})
			got, ok := sent["feedHidden"]
			if tt.want == "" {
				if ok {
					t.Fatalf("feedHidden sent as %s", got)
				}
				return
			}
			if string(got) != tt.want {
				t.Fatalf("feedHidden = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestDefaultProfileSettingsHaveNoHiddenKinds(t *testing.T) {
	if got := DefaultProfileSettings().FeedHidden; len(got) != 0 {
		t.Fatalf("default FeedHidden = %v", got)
	}
}

func TestProfileFeedHiddenSurvivesDefaults(t *testing.T) {
	for _, data := range []string{
		`{"profile":{"feedHidden":["played"]}}`,
		`{"profile":{"showcase":["favorites"],"visibility":"friends","feedHidden":["played"]}}`,
	} {
		var user CurrentUser
		if err := json.Unmarshal([]byte(data), &user); err != nil {
			t.Fatal(err)
		}
		if got := withProfileDefaults(user).Profile.FeedHidden; !slices.Equal(got, []string{"played"}) {
			t.Fatalf("%s: FeedHidden = %v", data, got)
		}
	}
}
