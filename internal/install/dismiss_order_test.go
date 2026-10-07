package install

import (
	"strings"
	"testing"
)

func TestDismissRollbackKeepsListOrder(t *testing.T) {
	cases := []struct {
		name    string
		dismiss string
	}{
		{"first record", "a"},
		{"middle record", "b"},
		{"last record", "c"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newRig(t)
			for _, id := range []string{"a", "b", "c"} {
				r.add(Installation{ID: id, Name: id, Status: StatusFailed, Error: "boom"})
			}
			unblock := r.blockStore()
			wantPersistError(t, r.s.Dismiss(tc.dismiss))
			var order []string
			for _, item := range r.s.List() {
				order = append(order, item.ID)
			}
			unblock()
			if strings.Join(order, ",") != "a,b,c" {
				t.Fatalf("list after a failed Dismiss of %s = %v, want [a b c]", tc.dismiss, order)
			}
		})
	}
}
