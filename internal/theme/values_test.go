package theme

import (
	"errors"
	"testing"
)

func TestPresetTokensMatchTheirKind(t *testing.T) {
	for _, preset := range presets {
		for name, value := range preset.Tokens {
			kind, ok := allowedTokenSet[name]
			if !ok {
				continue
			}
			if !validTokenKind(kind, value) {
				t.Errorf("%s: %s = %q rejected as %s", preset.ID, name, value, kind)
			}
		}
	}
}

func TestValidTokenKind(t *testing.T) {
	cases := []struct {
		kind  Kind
		valid []string
		bad   []string
	}{
		{KindColor,
			[]string{"#fff", "#ffff", "#2d3fed", "#2d3fedaa", "transparent", "rgb(1, 2, 3)", "rgba(0, 0, 0, 0.5)", "rgb(0 0 0 / 50%)", "hsl(220, 50%, 40%)", "hsla(220deg, 50%, 40%, 0.3)", "color-mix(in srgb, #fff 20%, transparent)", "color-mix(in srgb, rgba(0, 0, 0, 0.5), #000 30%)"},
			[]string{"red", "url(x)", "#12", "#12345", "rgb(1, 2)", "rgb(a, b, c)", "color-mix(in srgb, #fff)", "color-mix(in foo, #fff, #000)", "12px", "rgba(0, 0, 0, 0.5"}},
		{KindLength,
			[]string{"0", "1.2rem", "-0.024em", "4px", "50%", "10vw", "calc(100% - 2rem)", "calc(2 * 1.2rem)", "clamp(3.6rem, 2.6vw, 4.4rem)", "min(10px, 2rem)", "max(1rem, 5vh)"},
			[]string{"red", "12", "1.2", "1.2pt", "px", "calc(1rem +)", "calc(1rem 2rem)", "clamp(1rem, 2rem)", "calc(red)", "1rem 2rem", "calc((1rem)"}},
		{KindNumber,
			[]string{"1", "0", "0.5", "-2", ".5"},
			[]string{"", "1px", "red", "1.2.3", "1e3"}},
		{KindTime,
			[]string{"0s", "120ms", "0.3s", ".5s"},
			[]string{"300", "ms", "-1s", "1m", "red", "10px"}},
		{KindEase,
			[]string{"linear", "ease", "ease-in", "ease-out", "ease-in-out", "cubic-bezier(0.2, 0.8, 0.2, 1)", "cubic-bezier(0.34, 1.4, 0.64, 1)"},
			[]string{"bounce", "cubic-bezier(0.2, 0.8, 0.2)", "cubic-bezier(2, 0, 0, 1)", "cubic-bezier(a, b, c, d)", "steps(4)", "120ms"}},
		{KindShadow,
			[]string{"none", "0 2px 4px rgba(0, 0, 0, 0.3)", "0 0.8rem 2.8rem rgba(0, 0, 0, 0.45), 0 2px 0.8rem rgba(0, 0, 0, 0.3)", "inset 0 1px 0 #fff", "0 1.2rem 2.8rem -0.8rem rgba(0, 0, 0, 0.6)"},
			[]string{"red", "0 2px", "4px rgba(0, 0, 0, 0.3)", "0 2px 4px", "0 2px 4px red", "0 2px 4px 1px 2px #000", "0 2px 4px #000,", "url(x)"}},
		{KindFont,
			[]string{"sans-serif", "Inter Tight Variable, Inter, Segoe UI, system-ui, sans-serif", "Roboto Mono, monospace"},
			[]string{"", "Inter,", ",Inter", "Inter; color", "Inter {", "-bad", "Inter, , Arial"}},
	}
	for _, c := range cases {
		for _, v := range c.valid {
			if !validTokenKind(c.kind, v) {
				t.Errorf("%s: %q rejected, want valid", c.kind, v)
			}
		}
		for _, v := range c.bad {
			if validTokenKind(c.kind, v) {
				t.Errorf("%s: %q accepted, want invalid", c.kind, v)
			}
		}
	}
}

func TestValidateRejectsValueOfWrongKind(t *testing.T) {
	cases := map[string]string{
		"--radius-md":  "red",
		"--dur":        "300",
		"--accent":     "12px",
		"--ease":       "fast",
		"--shadow-pop": "big",
	}
	for name, value := range cases {
		_, err := Validate(Theme{ID: "x", Name: "x", Base: BaseDark, Tokens: map[string]string{name: value}})
		if !errors.Is(err, ErrThemeTokenKind) {
			t.Errorf("Validate(%s: %s) = %v, want ErrThemeTokenKind", name, value, err)
		}
	}
	ok := Theme{ID: "x", Name: "x", Base: BaseDark, Tokens: map[string]string{"--radius-md": "0.6rem", "--accent": "#2d3fed"}}
	if _, err := Validate(ok); err != nil {
		t.Errorf("Validate(valid) = %v", err)
	}
}
