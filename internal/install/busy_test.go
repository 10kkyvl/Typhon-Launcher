package install

import "testing"

func TestBusy(t *testing.T) {
	cases := []struct {
		name   string
		items  []*Installation
		gameID string
		want   bool
	}{
		{"no installations", nil, "g1", false},
		{"unrelated game", []*Installation{{GameID: "g2", Status: StatusInstalling}}, "g1", false},
		{"pending is active", []*Installation{{GameID: "g1", Status: StatusPending}}, "g1", true},
		{"preparing is active", []*Installation{{GameID: "g1", Status: StatusPreparing}}, "g1", true},
		{"installing is active", []*Installation{{GameID: "g1", Status: StatusInstalling}}, "g1", true},
		{"extracting is active", []*Installation{{GameID: "g1", Status: StatusExtracting}}, "g1", true},
		{"verifying is active", []*Installation{{GameID: "g1", Status: StatusVerifying}}, "g1", true},
		{"waiting for user is active", []*Installation{{GameID: "g1", Status: StatusWaitingForUser}}, "g1", true},
		{"completed is terminal", []*Installation{{GameID: "g1", Status: StatusCompleted}}, "g1", false},
		{"failed is terminal", []*Installation{{GameID: "g1", Status: StatusFailed}}, "g1", false},
		{"cancelled is terminal", []*Installation{{GameID: "g1", Status: StatusCancelled}}, "g1", false},
		{"interrupted is terminal", []*Installation{{GameID: "g1", Status: StatusInterrupted}}, "g1", false},
		{"empty gameID never busy", []*Installation{{GameID: "g1", Status: StatusInstalling}}, "", false},
		{"matches among several", []*Installation{
			{GameID: "g0", Status: StatusCompleted},
			{GameID: "g1", Status: StatusFailed},
			{GameID: "g1", Status: StatusExtracting},
		}, "g1", true},
	}
	for _, tc := range cases {
		s := &Service{items: tc.items}
		if got := s.Busy(tc.gameID); got != tc.want {
			t.Fatalf("%s: Busy(%q) = %v, want %v", tc.name, tc.gameID, got, tc.want)
		}
	}
}
