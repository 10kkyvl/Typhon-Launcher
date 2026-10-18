package shortcut

import (
	"errors"
	"strings"
	"testing"
)

func TestGameAppID(t *testing.T) {
	long := strings.Repeat("a", 300)
	tests := []struct {
		name    string
		in      string
		want    string
		wantErr error
	}{
		{name: "plain", in: "abc-123_x", want: "Typhon.Game.abc-123_x"},
		{name: "replaces invalid", in: "a b/c:d", want: "Typhon.Game.a_b_c_d"},
		{name: "non ascii", in: "игра", want: "Typhon.Game.____"},
		{name: "empty", in: "", wantErr: ErrAppIDEmpty},
		{name: "truncated", in: long, want: "Typhon.Game." + strings.Repeat("a", 128-len("Typhon.Game."))},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := GameAppID(tc.in)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
			if err == nil {
				if err := ValidateAppID(got); err != nil {
					t.Fatalf("generated id %q is invalid: %v", got, err)
				}
			}
		})
	}
}

func TestValidateAppID(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		wantErr error
	}{
		{name: "launcher", in: LauncherAppID},
		{name: "empty", in: "", wantErr: ErrAppIDEmpty},
		{name: "space", in: "a b", wantErr: ErrAppIDInvalid},
		{name: "slash", in: "a/b", wantErr: ErrAppIDInvalid},
		{name: "too long", in: strings.Repeat("a", 129), wantErr: ErrAppIDTooLong},
		{name: "max", in: strings.Repeat("a", 128)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := ValidateAppID(tc.in); !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
		})
	}
}
