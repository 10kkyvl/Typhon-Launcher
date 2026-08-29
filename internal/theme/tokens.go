package theme

type Kind string

const (
	KindColor  Kind = "color"
	KindLength Kind = "length"
	KindNumber Kind = "number"
	KindShadow Kind = "shadow"
	KindFont   Kind = "font"
	KindEase   Kind = "ease"
	KindTime   Kind = "time"
)

type Token struct {
	Name string
	Kind Kind
}

// settingsOwnedTokens are declared in tokens.css but belong to a setting with
// its own control, not to a theme: letting a theme carry them would give two
// owners to one value and make the last writer win at random.
var settingsOwnedTokens = []string{"--ui-scale"}

var allowedTokens = []Token{
	{"--bg", KindColor},
	{"--bg-sidebar", KindColor},
	{"--surface", KindColor},
	{"--surface-2", KindColor},
	{"--surface-3", KindColor},
	{"--surface-4", KindColor},
	{"--border", KindColor},
	{"--border-strong", KindColor},
	{"--hover", KindColor},
	{"--hover-strong", KindColor},
	{"--text", KindColor},
	{"--text-2", KindColor},
	{"--text-3", KindColor},
	{"--accent", KindColor},
	{"--accent-hover", KindColor},
	{"--accent-subtle", KindColor},
	{"--accent-text", KindColor},
	{"--accent-ring", KindColor},
	{"--success", KindColor},
	{"--success-subtle", KindColor},
	{"--warning", KindColor},
	{"--warning-subtle", KindColor},
	{"--danger", KindColor},
	{"--danger-subtle", KindColor},
	{"--radius-xs", KindLength},
	{"--radius-sm", KindLength},
	{"--radius-md", KindLength},
	{"--radius-lg", KindLength},
	{"--radius-xl", KindLength},
	{"--cut", KindLength},
	{"--space-1", KindLength},
	{"--space-2", KindLength},
	{"--space-3", KindLength},
	{"--space-4", KindLength},
	{"--space-5", KindLength},
	{"--space-6", KindLength},
	{"--space-8", KindLength},
	{"--space-10", KindLength},
	{"--space-12", KindLength},
	{"--page-x", KindLength},
	{"--page-max", KindLength},
	{"--prose-max", KindLength},
	{"--sidebar-w", KindLength},
	{"--topbar-h", KindLength},
	{"--font-xs", KindLength},
	{"--font-sm", KindLength},
	{"--font-md", KindLength},
	{"--font-lg", KindLength},
	{"--font-xl", KindLength},
	{"--font-title", KindLength},
	{"--font-hero", KindLength},
	{"--tracking-title", KindLength},
	{"--tracking-heading", KindLength},
	{"--control-sm", KindLength},
	{"--control-md", KindLength},
	{"--control-lg", KindLength},
	{"--font-sans", KindFont},
	{"--ease", KindEase},
	{"--dur", KindTime},
	{"--dur-fast", KindTime},
	{"--dur-panel", KindTime},
	{"--shadow-pop", KindShadow},
	{"--shadow-modal", KindShadow},
}

var allowedTokenSet = func() map[string]Kind {
	m := make(map[string]Kind, len(allowedTokens))
	for _, tok := range allowedTokens {
		m[tok.Name] = tok.Kind
	}
	return m
}()
