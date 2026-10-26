package theme

import (
	"errors"
	"testing"
)

func TestValidateCSSRefusesFetchBypasses(t *testing.T) {
	cases := []struct {
		name string
		css  string
	}{
		{"hex escape inside url", `.a { background: u\72l(https://evil.test/x.png); }`},
		{"escaped first letter", `.a { background: \75rl(https://evil.test/x.png); }`},
		{"escaped import", `@\69mport "https://evil.test/x.css";`},
		{"lone backslash", `.a { content: "\"; }`},
		{"image-set string", `.a { background-image: image-set("https://evil.test/x.png" 1x); }`},
		{"webkit image-set", `.a { background-image: -webkit-image-set("https://evil.test/x.png" 1x); }`},
		{"image-set uppercase", `.a { background-image: IMAGE-SET("https://evil.test/x.png" 1x); }`},
		{"image-set space before paren", `.a { background-image: image-set ("https://evil.test/x.png" 1x); }`},
		{"image-set newline before paren", ".a { background-image: image-set\n\t(\"https://evil.test/x.png\" 1x); }"},
		{"image-set comment inside name", `.a { background-image: image-/**/set("https://evil.test/x.png" 1x); }`},
		{"cross-fade", `.a { background-image: cross-fade("https://evil.test/a.png", "https://evil.test/b.png", 50%); }`},
		{"webkit cross-fade", `.a { background-image: -webkit-cross-fade("https://evil.test/a.png", "https://evil.test/b.png", 50%); }`},
		{"image function", `.a { background-image: image("https://evil.test/x.png"); }`},
		{"src function", `.a { background-image: src("https://evil.test/x.png"); }`},
		{"font-face", `@font-face { font-family: x; src: local(x); }`},
		{"font-face uppercase", `@FONT-FACE { font-family: x; }`},
		{"namespace", `@namespace svg "http://www.w3.org/2000/svg";`},
		{"typed attr", `.a { background-image: attr(data-src url); }`},
		{"typed attr spaced", `.a { background-image: attr (data-src type(<url>)); }`},
		{"url space before paren", `.a { background: url ("https://evil.test/x.png"); }`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateCSS(tc.css); !errors.Is(err, ErrThemeCSSContent) {
				t.Fatalf("validateCSS(%q) = %v, want ErrThemeCSSContent", tc.css, err)
			}
		})
	}
}

func TestValidateCSSKeepsOrdinaryRules(t *testing.T) {
	cases := []string{
		``,
		`.panel { color: red; }`,
		`[data-part="sidebar-item"][data-id="library"] { order: 2; }`,
		`[data-part="game-section"][data-id="reviews"] { display: none; }`,
		`.a { background-image: linear-gradient(90deg, #000, #fff); }`,
		`/* soft corners */ .card { border-radius: 1.2rem; }`,
		`.a:hover { box-shadow: 0 0 0 2px var(--accent-ring); }`,
	}
	for _, css := range cases {
		if err := validateCSS(css); err != nil {
			t.Errorf("validateCSS(%q) = %v, want nil", css, err)
		}
	}
}

func TestValidateTokenValueRefusesBackslash(t *testing.T) {
	for _, value := range []string{`\23 fff`, `u\72l(x)`, `#fff\`} {
		if err := validateTokenValue(value); !errors.Is(err, ErrThemeTokenValue) {
			t.Errorf("validateTokenValue(%q) = %v, want ErrThemeTokenValue", value, err)
		}
	}
}
