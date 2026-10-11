package support

import (
	"strings"
	"testing"
)

type runeCase struct {
	name string
	r    rune
}

func between(r rune) string {
	return "one" + string(r) + "two"
}

func TestSubjectRefusesWhatTheServerRefuses(t *testing.T) {
	cases := []runeCase{
		{"tab", 0x09},
		{"nul", 0x00},
		{"escape", 0x1b},
		{"delete", 0x7f},
		{"c1 control", 0x85},
		{"soft hyphen", 0xad},
		{"zero width space", 0x200b},
		{"zero width non-joiner", 0x200c},
		{"zero width joiner", 0x200d},
		{"left-to-right mark", 0x200e},
		{"right-to-left mark", 0x200f},
		{"line separator", 0x2028},
		{"paragraph separator", 0x2029},
		{"left-to-right embedding", 0x202a},
		{"right-to-left override", 0x202e},
		{"word joiner", 0x2060},
		{"left-to-right isolate", 0x2066},
		{"pop directional isolate", 0x2069},
		{"byte order mark inside", 0xfeff},
		{"tag character", 0xe0041},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newRig(t, rigOptions{})
			r.start(t)
			input := validInput()
			input.AttachLogs = true
			input.Subject = between(tc.r)

			_, err := r.svc.Create(input)
			if apiErr := apiCode(t, err); apiErr.Code != CodeTicketInvalidSubject || apiErr.Field != "subject" {
				t.Fatalf("error = {%q %q}", apiErr.Code, apiErr.Field)
			}
			if events := r.journal.list(); len(events) != 0 {
				t.Fatalf("events = %v: logs went out for a subject the server refuses", events)
			}
		})
	}
}

func TestBodyRefusesWhatTheServerRefuses(t *testing.T) {
	cases := []runeCase{
		{"nul", 0x00},
		{"escape", 0x1b},
		{"form feed", 0x0c},
		{"delete", 0x7f},
		{"c1 control", 0x85},
		{"line separator", 0x2028},
		{"paragraph separator", 0x2029},
		{"left-to-right embedding", 0x202a},
		{"right-to-left embedding", 0x202b},
		{"pop directional formatting", 0x202c},
		{"left-to-right override", 0x202d},
		{"right-to-left override", 0x202e},
		{"left-to-right isolate", 0x2066},
		{"right-to-left isolate", 0x2067},
		{"first strong isolate", 0x2068},
		{"pop directional isolate", 0x2069},
		{"byte order mark inside", 0xfeff},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newRig(t, rigOptions{})
			r.start(t)
			input := validInput()
			input.AttachLogs = true
			input.Body = between(tc.r)

			_, err := r.svc.Create(input)
			if apiErr := apiCode(t, err); apiErr.Code != CodeTicketInvalidBody || apiErr.Field != "body" {
				t.Fatalf("create error = {%q %q}", apiErr.Code, apiErr.Field)
			}
			_, err = r.svc.Reply(1042, between(tc.r), true)
			if apiErr := apiCode(t, err); apiErr.Code != CodeTicketInvalidBody {
				t.Fatalf("reply code = %q", apiErr.Code)
			}
			if events := r.journal.list(); len(events) != 0 {
				t.Fatalf("events = %v: logs went out for a body the server refuses", events)
			}
		})
	}
}

func TestTextKeepsWhatTheServerKeeps(t *testing.T) {
	joined := string([]rune{0x1F468, 0x200d, 0x1F469, 0x200d, 0x1F467})
	heart := string([]rune{0x2764, 0xfe0f})
	persian := string([]rune{0x645, 0x6cc, 0x200c, 0x62e, 0x648, 0x627, 0x647, 0x645})
	cases := []struct {
		name    string
		subject string
		body    string
	}{
		{"line breaks and tabs in the body", "Subject", "one\n\ttwo\r\nthree"},
		{"emoji joiner and variation selector", "Subject", "family " + joined + " and " + heart},
		{"zero width non-joiner in a script that needs it", "Subject", persian},
		{"no-break space and other spaces", between(0xa0), between(0x2003)},
		{"plain non-latin text", "Не устанавливается", "Ошибка при распаковке"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newRig(t, rigOptions{})
			r.start(t)
			input := validInput()
			input.Subject = tc.subject
			input.Body = tc.body

			if _, err := r.svc.Create(input); err != nil {
				t.Fatalf("Create: %v", err)
			}
			if _, err := r.svc.Reply(1042, tc.body, false); err != nil {
				t.Fatalf("Reply: %v", err)
			}
		})
	}
}

func TestBodyLimitCountsALineBreakOnce(t *testing.T) {
	body := strings.Repeat("ab\r\n", MaxBodyRunes/3)
	if got := len([]rune(body)); got <= MaxBodyRunes {
		t.Fatalf("fixture has %d runes, want more than %d before folding", got, MaxBodyRunes)
	}
	r := newRig(t, rigOptions{})
	r.start(t)
	input := validInput()
	input.Body = body

	if _, err := r.svc.Create(input); err != nil {
		t.Fatalf("Create: %v", err)
	}
	sent := as[string](t, decodeBody(t, r.api.only())["body"])
	if strings.ContainsRune(sent, '\r') {
		t.Fatal("body still has carriage returns")
	}
}
