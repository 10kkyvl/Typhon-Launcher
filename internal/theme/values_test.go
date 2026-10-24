package theme

import (
	"errors"
	"strings"
	"testing"
)

func TestPresetTokensMatchTheirKind(t *testing.T) {
	for _, preset := range presets {
		for name, value := range preset.Tokens {
			kind, ok := allowedTokenSet[name]
			if !ok {
				continue
			}
			if !validTokenKind(name, kind, value) {
				t.Errorf("%s: %s = %q rejected as %s", preset.ID, name, value, kind)
			}
		}
	}
}

func TestValidTokenKind(t *testing.T) {
	cases := []struct {
		name  string
		kind  Kind
		valid []string
		bad   []string
	}{
		{"--accent", KindColor,
			[]string{"#fff", "#ffff", "#2d3fed", "#2d3fedaa", "transparent", "rgb(1, 2, 3)", "rgba(0, 0, 0, 0.5)", "rgb(0 0 0 / 50%)", "rgb(0 0 0)", "hsl(220, 50%, 40%)", "hsla(220deg, 50%, 40%, 0.3)", "hsl(0.5turn 50% 40%)",
				"red", "RebeccaPurple", "currentColor", "CurrentColor", "oklch(60% 0.2 250)", "oklch(60% 0.2 250 / 0.5)", "oklab(0.5 0.1 0.1)", "lab(50% 10 10)", "lch(50% 30 200deg)", "hwb(200 10% 10%)",
				"color(display-p3 1 0 0)", "color(srgb 0.1 0.2 0.3 / 0.5)", "var(--surface)", "var(--accent-hover)",
				"color-mix(in srgb, #fff 20%, transparent)", "color-mix(in srgb, rgba(0, 0, 0, 0.5), #000 30%)", "color-mix(in oklch, red, blue)", "color-mix(in oklch longer hue, red 30%, blue)", "color-mix(in srgb, var(--surface) 80%, white)", "color-mix(in lab, 40% red, blue)"},
			[]string{"", "url(x)", "#12", "#12345", "rgb(1, 2)", "rgb(a, b, c)", "rgb(1,2,3,)", "rgb(1 2 3 / )", "rgb(1 2 3 4)", "rgb(1, 2, 3 / 0.5)", "rgb(1 2, 3)", "oklch(1, 2, 3)",
				"color-mix(in srgb, #fff)", "color-mix(in foo, #fff, #000)", "color-mix(in srgb, #fff 120%, #000)", "color-mix(in srgb longer hue, red, blue)", "color(foo 1 0 0)", "color(srgb 1 0)", "12px", "rgba(0, 0, 0, 0.5", "notacolor", "var(--radius-md)", "var(--missing)", "var(--surface, red)"}},
		{"--space-2", KindLength,
			[]string{"0", "1.2rem", "4px", "50%", "10vw", "2ch", "1ex", "10vmin", "10vmax", "50dvh", "50svh", "50lvh", "50dvw", "50svw", "50lvw", "12pt", "1cm", "1mm", "1in",
				"calc(100% - 2rem)", "calc(2 * 1.2rem)", "calc(1.2rem * 2)", "calc(100% / 3)", "calc(1rem + (2px * 3))", "clamp(3.6rem, 2.6vw, 4.4rem)", "min(10px, 2rem)", "max(1rem, 5vh)", "min(2rem)", "max(2rem)", "min(10px + 2px, 3rem)",
				"var(--space-4)", "calc(var(--space-4) * 2)", "min(var(--space-4), 10px)", "10000px", "1000rem"},
			[]string{"", "red", "12", "1.2", "1.2pt2", "px", "5.px", "-1rem", "calc(1rem +)", "calc(1rem 2rem)", "clamp(1rem, 2rem)", "calc(red)", "1rem 2rem", "calc((1rem)",
				"calc(1rem * 2rem)", "calc(1rem / 2rem)", "calc(1rem / 0)", "calc(1rem / (1 - 1))", "calc(1rem + 2)", "calc(1rem+2px)", "10001px", "1001rem", "1001%", "var(--accent)", "var(--space-2)", "var(--space-4, 1px)", "1e3px"}},
		{"--tracking-title", KindLength,
			[]string{"-0.024em", "-1px", "0.1em", "0"},
			[]string{"-10001px", "-1001em", "-"}},
		{"--ui-scale", KindNumber,
			[]string{"1", "0", "0.5", "-2", ".5"},
			[]string{"", "1px", "red", "1.2.3", "1e3", "5."}},
		{"--dur", KindTime,
			[]string{"0s", "120ms", "0.3s", ".5s", "10s", "10000ms", "var(--dur-fast)"},
			[]string{"300", "ms", "-1s", "1m", "red", "10px", "11s", "10001ms", "var(--ease)"}},
		{"--ease", KindEase,
			[]string{"linear", "ease", "ease-in", "ease-out", "ease-in-out", "Ease-In", "LINEAR", "cubic-bezier(0.2, 0.8, 0.2, 1)", "cubic-bezier(0.34, 1.4, 0.64, 1)", "CUBIC-BEZIER(0, 0, 1, 1)",
				"steps(4)", "steps(4, end)", "steps(2, jump-both)", "STEPS(3, start)", "step-start", "step-end", "var(--ease-spring)"},
			[]string{"", "bounce", "cubic-bezier(0.2, 0.8, 0.2)", "cubic-bezier(2, 0, 0, 1)", "cubic-bezier(a, b, c, d)", "steps(0)", "steps(4, foo)", "steps(1.5)", "120ms", "var(--ease)", "var(--dur)"}},
		{"--shadow-pop", KindShadow,
			[]string{"none", "0 2px 4px rgba(0, 0, 0, 0.3)", "0 0.8rem 2.8rem rgba(0, 0, 0, 0.45), 0 2px 0.8rem rgba(0, 0, 0, 0.3)", "inset 0 1px 0 #fff", "0 1.2rem 2.8rem -0.8rem rgba(0, 0, 0, 0.6)",
				"0 2px 4px red", "0 2px 4px var(--accent)", "0 2px 4px oklch(50% 0.1 200)", "0 2px 4px color-mix(in srgb, red, blue)", "0 2px 4px", "0 2px", "0 0 0 1px", "var(--shadow-lift)", "var(--shadow-lift), 0 0 1px #000"},
			[]string{"", "red", "4px rgba(0, 0, 0, 0.3)", "0 2px 4px 1px 2px #000", "0 2px 4px #000,", "url(x)", "inset inset 0 1px #fff", "0 2px 4px red blue", "var(--shadow-pop)", "var(--accent)", "0 2px 4px #000 foo"}},
		{"--font-sans", KindFont,
			[]string{"sans-serif", "Inter Tight Variable, Inter, Segoe UI, system-ui, sans-serif", "Roboto Mono, monospace", "-apple-system, BlinkMacSystemFont, Segoe UI, sans-serif", "Noto Sans CJK JP, sans-serif"},
			[]string{"", "Inter,", ",Inter", "Inter; color", "Inter {", "--x", "Inter, , Arial", "Inter, var(--font-sans)", "Source Sans 3", "Font Awesome 5 Free", "3270 Nerd Font", "3270", "Inter, 3270 Nerd Font", "var(--accent)"}},
	}
	for _, c := range cases {
		for _, v := range c.valid {
			if !validTokenKind(c.name, c.kind, v) {
				t.Errorf("%s %s: %q rejected, want valid", c.name, c.kind, v)
			}
		}
		for _, v := range c.bad {
			if validTokenKind(c.name, c.kind, v) {
				t.Errorf("%s %s: %q accepted, want invalid", c.name, c.kind, v)
			}
		}
	}
}

func TestVarCannotReferenceItself(t *testing.T) {
	if validTokenKind("--accent", KindColor, "var(--accent)") {
		t.Error("--accent: var(--accent) accepted")
	}
}

func TestValidateRejectsValueOfWrongKind(t *testing.T) {
	cases := map[string]string{
		"--radius-md":  "red",
		"--dur":        "300",
		"--accent":     "12px",
		"--ease":       "fast",
		"--shadow-pop": "big",
		"--space-2":    "-1rem",
	}
	for name, value := range cases {
		_, err := Validate(Theme{ID: "x", Name: "x", Base: BaseDark, Tokens: map[string]string{name: value}})
		if !errors.Is(err, ErrThemeTokenKind) {
			t.Errorf("Validate(%s: %s) = %v, want ErrThemeTokenKind", name, value, err)
		}
	}
	ok := Theme{ID: "x", Name: "x", Base: BaseDark, Tokens: map[string]string{
		"--radius-md": "0.6rem", "--accent": "#2d3fed", "--bg": "var(--surface)", "--text": "white", "--tracking-title": "-0.03em",
	}}
	if _, err := Validate(ok); err != nil {
		t.Errorf("Validate(valid) = %v", err)
	}
}

func TestValidateNamesEveryWrongTokenInOrder(t *testing.T) {
	bad := Theme{ID: "x", Name: "x", Base: BaseDark, Tokens: map[string]string{
		"--radius-md": "red", "--dur": "300", "--accent": "12px", "--bg": "#000", "--ease": "fast",
	}}
	want := "--accent (color), --dur (time), --ease (ease), --radius-md (length)"
	for i := 0; i < 20; i++ {
		_, err := Validate(bad)
		if !errors.Is(err, ErrThemeTokenKind) || !strings.HasPrefix(err.Error(), want) {
			t.Fatalf("Validate error = %v, want it to start with %q", err, want)
		}
	}
}

func TestValidateRejectsVarCycles(t *testing.T) {
	cases := map[string]struct {
		tokens map[string]string
		want   string
	}{
		"two": {map[string]string{"--bg": "var(--surface)", "--surface": "var(--bg)"}, "--bg, --surface"},
		"three": {map[string]string{
			"--text": "var(--text-2)", "--text-2": "var(--text-3)", "--text-3": "var(--text)", "--bg": "#000",
		}, "--text, --text-2, --text-3"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Validate(Theme{ID: "x", Name: "x", Base: BaseDark, Tokens: c.tokens})
			if !errors.Is(err, ErrThemeTokenKind) || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("Validate error = %v, want ErrThemeTokenKind naming %q", err, c.want)
			}
		})
	}
}

func TestValidateAcceptsVarChainsWithoutCycle(t *testing.T) {
	tokens := map[string]string{
		"--bg": "var(--surface)", "--surface": "var(--surface-2)", "--surface-2": "#101010",
		"--text": "var(--text-2)",
	}
	if _, err := Validate(Theme{ID: "x", Name: "x", Base: BaseDark, Tokens: tokens}); err != nil {
		t.Fatalf("Validate(chain) = %v", err)
	}
}
