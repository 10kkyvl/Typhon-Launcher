package redist

import "regexp"

var vcDisplayPattern = regexp.MustCompile(`(?i)^Microsoft Visual C\+\+ (2005|2008|2010|2012|2013)\b.*Redistributable`)

func parseVCDisplayName(display, version string) (Runtime, bool) {
	m := vcDisplayPattern.FindStringSubmatch(display)
	if m == nil {
		return Runtime{}, false
	}
	return Runtime{Name: "Visual C++ " + m[1], Version: version, Arch: archOf(display)}, true
}
