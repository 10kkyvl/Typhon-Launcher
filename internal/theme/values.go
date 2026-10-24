package theme

import (
	"regexp"
	"slices"
	"strconv"
	"strings"
)

const numPattern = `(?:\d+(?:\.\d+)?|\.\d+)`

var (
	numberRe   = regexp.MustCompile(`^-?` + numPattern + `$`)
	percentRe  = regexp.MustCompile(`^-?` + numPattern + `%$`)
	quantityRe = regexp.MustCompile(`^(-?)(` + numPattern + `)([a-z]+|%)?`)
	timeRe     = regexp.MustCompile(`^(` + numPattern + `)(ms|s)$`)
	hexColorRe = regexp.MustCompile(`^#(?:[0-9a-fA-F]{3,4}|[0-9a-fA-F]{6}|[0-9a-fA-F]{8})$`)
	hueRe      = regexp.MustCompile(`^-?` + numPattern + `(?:deg|rad|grad|turn)?$`)
	funcHeadRe = regexp.MustCompile(`^[a-z]+\(`)
	varNameRe  = regexp.MustCompile(`^--[a-z0-9-]+$`)
	familyRe   = regexp.MustCompile(`^-?[A-Za-z][A-Za-z0-9-]*$`)
	stepsRe    = regexp.MustCompile(`^[1-9][0-9]*$`)
)

const (
	maxAbsoluteLength = 10000
	maxRelativeLength = 1000
	maxTimeMillis     = 10000
)

var lengthUnits = map[string]float64{
	"px": maxAbsoluteLength, "pt": maxAbsoluteLength, "cm": maxAbsoluteLength, "mm": maxAbsoluteLength, "in": maxAbsoluteLength,
	"rem": maxRelativeLength, "em": maxRelativeLength, "ch": maxRelativeLength, "ex": maxRelativeLength, "%": maxRelativeLength,
	"vw": maxRelativeLength, "vh": maxRelativeLength, "vmin": maxRelativeLength, "vmax": maxRelativeLength,
	"dvh": maxRelativeLength, "svh": maxRelativeLength, "lvh": maxRelativeLength,
	"dvw": maxRelativeLength, "svw": maxRelativeLength, "lvw": maxRelativeLength,
}

var easeKeywords = setOf("linear", "ease", "ease-in", "ease-out", "ease-in-out", "step-start", "step-end")

var genericFamily = setOf("serif", "sans-serif", "monospace", "cursive", "fantasy", "system-ui",
	"ui-sans-serif", "ui-serif", "ui-monospace", "ui-rounded", "math", "emoji", "fangsong")

var (
	rectColorSpaces = setOf("srgb", "srgb-linear", "display-p3", "a98-rgb", "prophoto-rgb", "rec2020", "xyz", "xyz-d50", "xyz-d65")
	polarSpaces     = setOf("hsl", "hwb", "lch", "oklch")
	mixSpaces       = setOf("srgb", "srgb-linear", "display-p3", "a98-rgb", "prophoto-rgb", "rec2020", "lab", "oklab", "xyz", "xyz-d50", "xyz-d65", "hsl", "hwb", "lch", "oklch")
	hueMethods      = setOf("shorter", "longer", "increasing", "decreasing")
	jumpTerms       = setOf("jump-start", "jump-end", "jump-none", "jump-both", "start", "end")
)

// The full CSS Color 4 keyword list: a token value is a colour either way, so
// the launcher accepts exactly what the browser would.
var namedColors = setOf(strings.Fields(`aliceblue antiquewhite aqua aquamarine azure beige bisque black blanchedalmond
blue blueviolet brown burlywood cadetblue chartreuse chocolate coral cornflowerblue cornsilk crimson cyan darkblue
darkcyan darkgoldenrod darkgray darkgreen darkgrey darkkhaki darkmagenta darkolivegreen darkorange darkorchid darkred
darksalmon darkseagreen darkslateblue darkslategray darkslategrey darkturquoise darkviolet deeppink deepskyblue dimgray
dimgrey dodgerblue firebrick floralwhite forestgreen fuchsia gainsboro ghostwhite gold goldenrod gray green greenyellow
grey honeydew hotpink indianred indigo ivory khaki lavender lavenderblush lawngreen lemonchiffon lightblue lightcoral
lightcyan lightgoldenrodyellow lightgray lightgreen lightgrey lightpink lightsalmon lightseagreen lightskyblue
lightslategray lightslategrey lightsteelblue lightyellow lime limegreen linen magenta maroon mediumaquamarine
mediumblue mediumorchid mediumpurple mediumseagreen mediumslateblue mediumspringgreen mediumturquoise mediumvioletred
midnightblue mintcream mistyrose moccasin navajowhite navy oldlace olive olivedrab orange orangered orchid
palegoldenrod palegreen paleturquoise palevioletred papayawhip peachpuff peru pink plum powderblue purple
rebeccapurple red rosybrown royalblue saddlebrown salmon sandybrown seagreen seashell sienna silver skyblue slateblue
slategray slategrey snow springgreen steelblue tan teal thistle tomato turquoise violet wheat white whitesmoke yellow
yellowgreen currentcolor transparent`)...)

func setOf(items ...string) map[string]bool {
	m := make(map[string]bool, len(items))
	for _, it := range items {
		m[it] = true
	}
	return m
}

type checker struct {
	self string
	neg  bool
}

func validTokenKind(name string, kind Kind, value string) bool {
	c := checker{self: name, neg: strings.HasPrefix(name, "--tracking-")}
	v := strings.TrimSpace(value)
	switch kind {
	case KindColor:
		return c.isColor(v)
	case KindLength:
		return c.isLength(v)
	case KindNumber:
		return numberRe.MatchString(v)
	case KindTime:
		return c.isTime(v)
	case KindEase:
		return c.isEase(v)
	case KindShadow:
		return c.isShadow(v)
	case KindFont:
		return c.isFont(v)
	}
	return false
}

func (c checker) isVar(v string, kind Kind) bool {
	inner, ok := funcArgs(v, "var")
	if !ok {
		return false
	}
	ref := strings.TrimSpace(inner)
	return varNameRe.MatchString(ref) && ref != c.self && allowedTokenSet[ref] == kind
}

func funcArgs(v, name string) (string, bool) {
	prefix := name + "("
	if len(v) <= len(prefix) || !strings.EqualFold(v[:len(prefix)], prefix) || !strings.HasSuffix(v, ")") {
		return "", false
	}
	inner := v[len(prefix) : len(v)-1]
	depth := 0
	for _, r := range inner {
		switch r {
		case '(':
			depth++
		case ')':
			depth--
			if depth < 0 {
				return "", false
			}
		}
	}
	if depth != 0 {
		return "", false
	}
	return inner, true
}

func splitTop(s string, sep rune) []string {
	var parts []string
	depth, start := 0, 0
	for i, r := range s {
		switch r {
		case '(':
			depth++
		case ')':
			depth--
		case sep:
			if depth == 0 {
				parts = append(parts, strings.TrimSpace(s[start:i]))
				start = i + len(string(r))
			}
		}
	}
	return append(parts, strings.TrimSpace(s[start:]))
}

func fieldsTop(s string) []string {
	var parts []string
	depth, start := 0, -1
	for i, r := range s {
		switch r {
		case '(':
			depth++
		case ')':
			depth--
		}
		if r == ' ' && depth == 0 {
			if start >= 0 {
				parts = append(parts, s[start:i])
				start = -1
			}
			continue
		}
		if start < 0 {
			start = i
		}
	}
	if start >= 0 {
		parts = append(parts, s[start:])
	}
	return parts
}

func (c checker) isColor(v string) bool {
	lower := strings.ToLower(v)
	if hexColorRe.MatchString(v) || namedColors[lower] || c.isVar(v, KindColor) {
		return true
	}
	if inner, ok := funcArgs(v, "color-mix"); ok {
		return c.isColorMix(inner)
	}
	if inner, ok := funcArgs(v, "color"); ok {
		return isColorFunction(inner)
	}
	for _, f := range []struct {
		name   string
		legacy bool
		hue    int
	}{
		{"rgb", true, -1}, {"rgba", true, -1}, {"hsl", true, 0}, {"hsla", true, 0},
		{"hwb", false, 0}, {"lab", false, -1}, {"oklab", false, -1}, {"lch", false, 2}, {"oklch", false, 2},
	} {
		if inner, ok := funcArgs(v, f.name); ok {
			return colorChannels(inner, f.legacy, f.hue)
		}
	}
	return false
}

func isChannel(s string) bool { return numberRe.MatchString(s) || percentRe.MatchString(s) }

func channelsOK(fields []string, hue int) bool {
	for i, f := range fields {
		if i == hue {
			if !hueRe.MatchString(f) {
				return false
			}
			continue
		}
		if !isChannel(f) {
			return false
		}
	}
	return true
}

func colorChannels(inner string, legacy bool, hue int) bool {
	if strings.Contains(inner, ",") {
		if !legacy || strings.Contains(inner, "/") {
			return false
		}
		parts := strings.Split(inner, ",")
		if len(parts) != 3 && len(parts) != 4 {
			return false
		}
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
		}
		return channelsOK(parts[:3], hue) && (len(parts) == 3 || isChannel(parts[3]))
	}
	return spaceChannels(inner, 3, hue)
}

func spaceChannels(inner string, want, hue int) bool {
	main, alpha, hasAlpha := strings.Cut(inner, "/")
	fields := strings.Fields(main)
	if len(fields) != want || !channelsOK(fields, hue) {
		return false
	}
	if !hasAlpha {
		return true
	}
	a := strings.Fields(alpha)
	return len(a) == 1 && isChannel(a[0])
}

func isColorFunction(inner string) bool {
	main, _, _ := strings.Cut(inner, "/")
	fields := strings.Fields(main)
	if len(fields) == 0 || !rectColorSpaces[fields[0]] {
		return false
	}
	_, alpha, hasAlpha := strings.Cut(inner, "/")
	if !hasAlpha {
		return len(fields) == 4 && channelsOK(fields[1:], -1)
	}
	return len(fields) == 4 && channelsOK(fields[1:], -1) && len(strings.Fields(alpha)) == 1 && isChannel(strings.Fields(alpha)[0])
}

func (c checker) isColorMix(inner string) bool {
	parts := splitTop(inner, ',')
	if len(parts) != 3 {
		return false
	}
	head := strings.Fields(parts[0])
	switch {
	case len(head) == 2:
	case len(head) == 4 && hueMethods[head[2]] && head[3] == "hue" && polarSpaces[head[1]]:
	default:
		return false
	}
	if head[0] != "in" || !mixSpaces[head[1]] {
		return false
	}
	return c.isMixStop(parts[1]) && c.isMixStop(parts[2])
}

func percentInRange(s string) bool {
	if !percentRe.MatchString(s) {
		return false
	}
	n, err := strconv.ParseFloat(strings.TrimSuffix(s, "%"), 64)
	return err == nil && n >= 0 && n <= 100
}

func (c checker) isMixStop(s string) bool {
	fields := fieldsTop(s)
	switch len(fields) {
	case 1:
		return c.isColor(fields[0])
	case 2:
		if percentInRange(fields[1]) {
			return c.isColor(fields[0])
		}
		if percentInRange(fields[0]) {
			return c.isColor(fields[1])
		}
	}
	return false
}

type calcValue struct {
	length bool
	n      float64
}

type calcParser struct {
	c   checker
	s   string
	i   int
	bad bool
}

func (c checker) isLength(v string) bool {
	if v == "0" {
		return true
	}
	val, ok := c.evalExpr(v)
	return ok && val.length
}

func (c checker) evalExpr(s string) (calcValue, bool) {
	p := &calcParser{c: c, s: s}
	val := p.expr()
	p.skip()
	if p.bad || p.i != len(p.s) {
		return calcValue{}, false
	}
	return val, true
}

func (p *calcParser) skip() {
	for p.i < len(p.s) && p.s[p.i] == ' ' {
		p.i++
	}
}

func (p *calcParser) fail() calcValue {
	p.bad = true
	return calcValue{}
}

func (p *calcParser) expr() calcValue {
	left := p.term()
	for !p.bad {
		p.skip()
		if p.i+1 >= len(p.s) || (p.s[p.i] != '+' && p.s[p.i] != '-') || p.s[p.i-1] != ' ' || p.s[p.i+1] != ' ' {
			break
		}
		op := p.s[p.i]
		p.i++
		right := p.term()
		if p.bad || left.length != right.length {
			return p.fail()
		}
		if !left.length {
			if op == '+' {
				left.n += right.n
			} else {
				left.n -= right.n
			}
		}
	}
	return left
}

func (p *calcParser) term() calcValue {
	left := p.factor()
	for !p.bad {
		p.skip()
		if p.i >= len(p.s) || (p.s[p.i] != '*' && p.s[p.i] != '/') {
			break
		}
		op := p.s[p.i]
		p.i++
		right := p.factor()
		if p.bad {
			return calcValue{}
		}
		if op == '*' {
			if left.length && right.length {
				return p.fail()
			}
			left = calcValue{length: left.length || right.length, n: left.n * right.n}
			continue
		}
		if right.length || right.n == 0 {
			return p.fail()
		}
		left = calcValue{length: left.length, n: left.n / right.n}
	}
	return left
}

func (p *calcParser) factor() calcValue {
	p.skip()
	rest := p.s[p.i:]
	switch {
	case rest == "":
		return p.fail()
	case rest[0] == '(':
		p.i++
		val := p.expr()
		p.skip()
		if p.bad || p.i >= len(p.s) || p.s[p.i] != ')' {
			return p.fail()
		}
		p.i++
		return val
	case funcHeadRe.MatchString(rest):
		return p.function(rest)
	}
	m := quantityRe.FindStringSubmatch(rest)
	if m == nil {
		return p.fail()
	}
	n, err := strconv.ParseFloat(m[2], 64)
	if err != nil {
		return p.fail()
	}
	p.i += len(m[0])
	if m[3] == "" {
		if m[1] == "-" {
			n = -n
		}
		return calcValue{n: n}
	}
	limit, ok := lengthUnits[m[3]]
	if !ok || n > limit || (m[1] == "-" && !p.c.neg) {
		return p.fail()
	}
	return calcValue{length: true}
}

func (p *calcParser) function(rest string) calcValue {
	open := strings.IndexByte(rest, '(')
	name := rest[:open]
	depth, end := 0, -1
scan:
	for j := open; j < len(rest); j++ {
		switch rest[j] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				end = j
				break scan
			}
		}
	}
	if end < 0 {
		return p.fail()
	}
	whole, inner := rest[:end+1], rest[open+1:end]
	p.i += end + 1
	switch name {
	case "var":
		if !p.c.isVar(whole, KindLength) {
			return p.fail()
		}
		return calcValue{length: true}
	case "calc":
		val, ok := p.c.evalExpr(inner)
		if !ok {
			return p.fail()
		}
		return val
	case "min", "max", "clamp":
		args := splitTop(inner, ',')
		if name == "clamp" && len(args) != 3 {
			return p.fail()
		}
		var first calcValue
		for i, a := range args {
			val, ok := p.c.evalExpr(a)
			if !ok || (i > 0 && val.length != first.length) {
				return p.fail()
			}
			first = val
		}
		return first
	}
	return p.fail()
}

func (c checker) isTime(v string) bool {
	if c.isVar(v, KindTime) {
		return true
	}
	m := timeRe.FindStringSubmatch(v)
	if m == nil {
		return false
	}
	n, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return false
	}
	if m[2] == "s" {
		n *= 1000
	}
	return n <= maxTimeMillis
}

func (c checker) isEase(v string) bool {
	lower := strings.ToLower(v)
	if easeKeywords[lower] || c.isVar(v, KindEase) {
		return true
	}
	if inner, ok := funcArgs(lower, "cubic-bezier"); ok {
		args := splitTop(inner, ',')
		if len(args) != 4 {
			return false
		}
		for i, a := range args {
			n, err := strconv.ParseFloat(a, 64)
			if err != nil || !numberRe.MatchString(a) || (i%2 == 0 && (n < 0 || n > 1)) {
				return false
			}
		}
		return true
	}
	if inner, ok := funcArgs(lower, "steps"); ok {
		args := splitTop(inner, ',')
		switch len(args) {
		case 1:
			return stepsRe.MatchString(args[0])
		case 2:
			return stepsRe.MatchString(args[0]) && jumpTerms[args[1]]
		}
	}
	return false
}

func (c checker) isShadow(v string) bool {
	if v == "none" {
		return true
	}
	shadow := checker{self: c.self, neg: true}
	for _, one := range splitTop(v, ',') {
		if !shadow.isOneShadow(one) {
			return false
		}
	}
	return true
}

func (c checker) isOneShadow(s string) bool {
	if c.isVar(s, KindShadow) {
		return true
	}
	lengths, colors, inset := 0, 0, 0
	for _, tok := range fieldsTop(s) {
		switch {
		case tok == "inset":
			inset++
		case c.isLength(tok):
			lengths++
		case c.isColor(tok):
			colors++
		default:
			return false
		}
	}
	return lengths >= 2 && lengths <= 4 && colors <= 1 && inset <= 1
}

func (c checker) isFont(v string) bool {
	for _, fam := range strings.Split(v, ",") {
		fam = strings.TrimSpace(fam)
		if genericFamily[fam] || c.isVar(fam, KindFont) {
			continue
		}
		words := strings.Fields(fam)
		if len(words) == 0 {
			return false
		}
		for _, w := range words {
			if !familyRe.MatchString(w) {
				return false
			}
		}
	}
	return true
}

var varRefRe = regexp.MustCompile(`var\((--[a-z0-9-]+)\)`)

// varCycles returns the sorted names of the tokens that take part in a var()
// cycle. A reference to a token the theme does not set is not an edge: it
// resolves to the default value.
func varCycles(tokens map[string]string) []string {
	edges := make(map[string][]string, len(tokens))
	for name, value := range tokens {
		for _, m := range varRefRe.FindAllStringSubmatch(value, -1) {
			if _, ok := tokens[m[1]]; ok {
				edges[name] = append(edges[name], m[1])
			}
		}
	}
	var cyclic []string
	for name := range edges {
		seen := map[string]bool{}
		queue := append([]string(nil), edges[name]...)
		for len(queue) > 0 {
			cur := queue[0]
			queue = queue[1:]
			if seen[cur] {
				continue
			}
			seen[cur] = true
			queue = append(queue, edges[cur]...)
		}
		if seen[name] {
			cyclic = append(cyclic, name)
		}
	}
	slices.Sort(cyclic)
	return cyclic
}
