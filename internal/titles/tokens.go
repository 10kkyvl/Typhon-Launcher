package titles

import (
	"regexp"
	"strings"
)

func normKey(w string) string {
	w = strings.ToLower(w)
	w = strings.NewReplacer("'", "", "’", "", "`", "").Replace(w)
	return w
}

var (
	reBuildVer  = regexp.MustCompile(`(?i)\bbuild[.\-_ ]+(\d+(?:\.\d+){0,4})\b`)
	reUpdateVer = regexp.MustCompile(`(?i)\bupdate[.\-_ ]+(\d+(?:\.\d+){0,4})\b`)
	rePatchVer  = regexp.MustCompile(`(?i)\bpatch[.\-_ ]+(\d+(?:\.\d+){0,4})\b`)
	reHotfixVer = regexp.MustCompile(`(?i)\bhotfix[.\-_ ]+(\d+(?:\.\d+){0,4})\b`)
	reVVer      = regexp.MustCompile(`(?i)\bv(\d+(?:\.\d+){0,4})\b`)
	reRVer      = regexp.MustCompile(`(?i)\br(\d{4,6})\b`)
	reDLCCount  = regexp.MustCompile(`(?i)\+\s*(\d+)\s*(?:dlc(?:'s|s)?|дополнени\p{L}*)`)

	reExt = regexp.MustCompile(`(?i)\.(rar|zip|iso|7z|exe|torrent)$`)
	reURL = regexp.MustCompile(`(?i)https?://\S+`)
	reWWW = regexp.MustCompile(`(?i)\bwww\.[^\s\]\)]+`)

	reBracket = regexp.MustCompile(`\[[^\[\]]*\]|\([^()]*\)|\{[^{}]*\}`)

	reMulti    = regexp.MustCompile(`(?i)\bmulti[\-]?\d{0,3}\b`)
	reSteamRip = regexp.MustCompile(`(?i)\bsteam[\-\s._]?rip\b`)
	reRepackBy = regexp.MustCompile(`(?i)\bre-?pack(?:[\s._-]+by[\s._-]+[A-Za-z0-9_]+)?\b`)

	reDecimalDot   = regexp.MustCompile(`(\d)\.(\d)`)
	reSepRun       = regexp.MustCompile(`[._\-]+`)
	reSpaceRun     = regexp.MustCompile(`\s+`)
	reYear         = regexp.MustCompile(`^(19[7-9]\d|20\d{2})$`)
	reBracketSplit = regexp.MustCompile(`[\s,./\-]+`)

	// reNeverMatch стоит на месте языковых шаблонов, когда список языков пуст:
	// альтернатива из нуля вариантов совпала бы с пустой строкой везде.
	reNeverMatch = regexp.MustCompile(`$^`)
)
