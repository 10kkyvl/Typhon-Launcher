package achievements

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func at(sec int64) time.Time { return time.Unix(sec, 0).UTC() }

func sameUnlocks(t *testing.T, got, want map[string]time.Time) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for key, wantAt := range want {
		gotAt, ok := got[key]
		if !ok || !gotAt.Equal(wantAt) {
			t.Fatalf("key %q: got %v (present %v), want %v; all: %v", key, gotAt, ok, wantAt, got)
		}
	}
}

func TestParseJSON(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want map[string]time.Time
		err  error
	}{
		{
			name: "earned and not earned",
			in:   `{"ACH_A":{"earned":true,"earned_time":1700000000},"ACH_B":{"earned":false,"earned_time":0}}`,
			want: map[string]time.Time{"ACH_A": at(1700000000)},
		},
		{
			name: "alternative names, case and number flags",
			in:   `{"A":{"Unlocked":true,"time":1700000000},"B":{"achieved":1,"unlock_time":1700000001},"C":{"ACHIEVED":"1"},"D":{"achieved":0}}`,
			want: map[string]time.Time{"A": at(1700000000), "B": at(1700000001), "C": {}},
		},
		{
			name: "milliseconds",
			in:   `{"A":{"earned":true,"earned_time":1700000000000}}`,
			want: map[string]time.Time{"A": at(1700000000)},
		},
		{
			name: "implausible time is unknown",
			in:   `{"A":{"earned":true,"earned_time":12}}`,
			want: map[string]time.Time{"A": {}},
		},
		{
			name: "byte order mark and unknown fields",
			in:   "\xef\xbb\xbf" + `{"A":{"earned":true,"progress":3,"max_progress":9}}`,
			want: map[string]time.Time{"A": {}},
		},
		{
			name: "keys outside the allowed set are dropped",
			in:   `{"../x":{"earned":true},"a b":{"earned":true},"A..B":{"earned":true},"":{"earned":true},"OK_1.x-y":{"earned":true}}`,
			want: map[string]time.Time{"OK_1.x-y": {}},
		},
		{
			name: "values that are not objects carry no progress",
			in:   `{"A":true,"B":[1],"C":{"earned":true}}`,
			want: map[string]time.Time{"C": {}},
		},
		{name: "broken json", in: `{"A":{"earned":`, err: ErrBadJSON},
		{name: "empty file", in: ``, err: ErrBadJSON},
		{name: "blank file", in: "  \n", err: ErrBadJSON},
		{name: "scalar root", in: `42`, err: ErrBadJSON},
		{name: "definitions array is not progress", in: `[{"name":"A","displayName":"a"}]`, err: ErrNotProgress},
		{name: "truncated array", in: `[{"name":`, err: ErrBadJSON},
		{name: "over the size limit", in: `{"A":{"earned":true,"pad":"` + strings.Repeat("x", maxFileBytes) + `"}}`, err: ErrFileTooBig},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseJSON(strings.NewReader(tc.in))
			if tc.err != nil {
				if !errors.Is(err, tc.err) {
					t.Fatalf("err = %v, want %v", err, tc.err)
				}
				return
			}
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			sameUnlocks(t, got, tc.want)
		})
	}
}

func TestParseINI(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want map[string]time.Time
		err  error
	}{
		{
			name: "achieved and unlock time",
			in:   "[ACH_A]\nAchieved=1\nUnlockTime=1700000000\n[ACH_B]\nAchieved=0\nUnlockTime=0\n",
			want: map[string]time.Time{"ACH_A": at(1700000000)},
		},
		{
			name: "service sections, case, spaces and packed state",
			in: "\r\n; comment\r\n[SteamAchievements]\r\nCount=2\r\n[Steam]\r\nAchieved=1\r\n" +
				"[ACH_X]\r\n State = 0101 \r\n Time=1700000005\r\n[ach_y]\r\n unlocked = TRUE \r\n[ACH_Z]\r\nState=0000\r\n",
			want: map[string]time.Time{"ACH_X": at(1700000005), "ach_y": {}},
		},
		{
			name: "progress counter is not a moment",
			in:   "[A]\nachieved=1\nCurProgress=50\n[B]\nachieved=1\nCurProgress=1700000000\n",
			want: map[string]time.Time{"A": {}, "B": at(1700000000)},
		},
		{
			name: "unknown keys are ignored and the earlier time wins",
			in:   "[A]\nachieved=1\nunlocktime=1700000100\nicon=a.png\n[A]\nachieved=1\nunlocktime=1700000050\n",
			want: map[string]time.Time{"A": at(1700000050)},
		},
		{
			name: "section names outside the allowed set are dropped",
			in:   "[A..B]\nachieved=1\n[bad key]\nachieved=1\n[../x]\nachieved=1\n[]\nachieved=1\n[OK]\nachieved=1\n",
			want: map[string]time.Time{"OK": {}},
		},
		{name: "settings before any section are ignored", in: "achieved=1\n[A]\nachieved=0\n", want: map[string]time.Time{}},
		{name: "empty file", in: "", want: map[string]time.Time{}},
		{name: "unclosed section", in: "[A\nachieved=1\n", err: ErrBadINI},
		{name: "line without a value", in: "[A]\nachieved\n", err: ErrBadINI},
		{name: "over the size limit", in: "[A]\nachieved=1\npad=" + strings.Repeat("x", maxFileBytes) + "\n", err: ErrFileTooBig},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseINI(strings.NewReader(tc.in))
			if tc.err != nil {
				if !errors.Is(err, tc.err) {
					t.Fatalf("err = %v, want %v", err, tc.err)
				}
				return
			}
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			sameUnlocks(t, got, tc.want)
		})
	}
}

func TestParseErrorsAreDistinct(t *testing.T) {
	_, jsonErr := ParseJSON(strings.NewReader(`{`))
	_, iniErr := ParseINI(strings.NewReader("[A"))
	if errors.Is(jsonErr, ErrBadINI) || errors.Is(iniErr, ErrBadJSON) || !errors.Is(jsonErr, ErrBadJSON) || !errors.Is(iniErr, ErrBadINI) {
		t.Fatalf("json err %v, ini err %v", jsonErr, iniErr)
	}
}

func TestMergeKeyKeepsTheEarlierKnownTime(t *testing.T) {
	cases := []struct {
		name       string
		prev, next time.Time
		want       time.Time
	}{
		{"earlier wins", at(1700000100), at(1700000000), at(1700000000)},
		{"later loses", at(1700000000), at(1700000100), at(1700000000)},
		{"unknown loses to known", at(1700000000), time.Time{}, at(1700000000)},
		{"known replaces unknown", time.Time{}, at(1700000000), at(1700000000)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := map[string]time.Time{"K": tc.prev}
			mergeKey(m, "K", tc.next)
			if !m["K"].Equal(tc.want) {
				t.Fatalf("got %v, want %v", m["K"], tc.want)
			}
		})
	}
}
