package titles

import (
	"regexp"
	"sort"
	"strings"
)

var langCodes = []string{
	"RUS", "ENG", "GER", "DEU", "FRA", "FRE", "ITA", "SPA", "ESP", "POL",
	"POR", "JPN", "CHI", "ZHO", "KOR", "CZE", "TUR", "UKR", "ARA", "SWE",
	"DAN", "NOR", "FIN", "HUN", "GRE", "NLD", "DUT",
}

var langCodeSet = func() map[string]struct{} {
	set := make(map[string]struct{}, len(langCodes))
	for _, c := range langCodes {
		set[strings.ToLower(c)] = struct{}{}
	}
	return set
}()

func isLangCode(lower string) bool {
	_, ok := langCodeSet[lower]
	return ok
}

var archTokens = map[string]string{
	"x64":   "x64",
	"x86":   "x86",
	"win64": "win64",
	"win32": "win32",
	"64bit": "64bit",
	"32bit": "32bit",
	"pc":    "pc",
}

var releaseSingleTags = map[string]string{
	"dlc":        "dlc",
	"dlcs":       "dlc",
	"update":     "update",
	"updates":    "update",
	"patch":      "patch",
	"hotfix":     "hotfix",
	"repack":     "repack",
	"codex":      "codex",
	"fitgirl":    "fitgirl",
	"dodi":       "dodi",
	"elamigos":   "elamigos",
	"xatab":      "xatab",
	"kaoskrew":   "kaoskrew",
	"masquerade": "masquerade",
	"crack":      "crack",
	"proper":     "proper",
	"incl":       "incl",
	"gog":        "gog",
}

var editionPhrases = []string{
	"Deluxe Edition",
	"Ultimate Edition",
	"Complete Edition",
	"Definitive Edition",
	"Standard Edition",
	"Gold Edition",
	"Enhanced Edition",
	"Anniversary Edition",
	"Legendary Edition",
	"Platinum Edition",
	"Premium Edition",
	"Special Edition",
	"Extended Edition",
	"Director's Cut",
	"GOTY",
	"Game of the Year Edition",
	"Game of the Year",
	"Collector's Edition",
	"Digital Deluxe Edition",
	"Digital Deluxe",
	"Remastered",
}

var releasePhraseTags = map[string]string{
	"All DLCs":           "dlc",
	"Selective Download": "selective-download",
}

type phraseTok struct {
	norm []string
	kind string
}

var phraseTable []phraseTok

func normKey(w string) string {
	w = strings.ToLower(w)
	w = strings.NewReplacer("'", "", "’", "", "`", "").Replace(w)
	return w
}

func init() {
	for _, p := range editionPhrases {
		ws := strings.Fields(p)
		norm := make([]string, len(ws))
		for i, w := range ws {
			norm[i] = normKey(w)
		}
		phraseTable = append(phraseTable, phraseTok{norm: norm, kind: "edition"})
	}
	for phrase, tag := range releasePhraseTags {
		ws := strings.Fields(phrase)
		norm := make([]string, len(ws))
		for i, w := range ws {
			norm[i] = normKey(w)
		}
		phraseTable = append(phraseTable, phraseTok{norm: norm, kind: "tag:" + tag})
	}
	sort.SliceStable(phraseTable, func(i, j int) bool {
		return len(phraseTable[i].norm) > len(phraseTable[j].norm)
	})
}

func buildLangAlternation() string {
	return strings.Join(langCodes, "|")
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

	reLangCombo  = regexp.MustCompile(`(?i)\b(?:` + buildLangAlternation() + `)(?:[/\-](?:` + buildLangAlternation() + `))+\b`)
	reLangSingle = regexp.MustCompile(`(?i)\b(?:` + buildLangAlternation() + `)\b`)
	reMulti      = regexp.MustCompile(`(?i)\bmulti[\-]?\d{0,3}\b`)
	reSteamRip   = regexp.MustCompile(`(?i)\bsteam[\-\s._]?rip\b`)
	reRepackBy   = regexp.MustCompile(`(?i)\bre-?pack(?:[\s._-]+by[\s._-]+[A-Za-z0-9_]+)?\b`)

	reDecimalDot   = regexp.MustCompile(`(\d)\.(\d)`)
	reSepRun       = regexp.MustCompile(`[._\-]+`)
	reSpaceRun     = regexp.MustCompile(`\s+`)
	reYear         = regexp.MustCompile(`^(19[7-9]\d|20\d{2})$`)
	reBracketSplit = regexp.MustCompile(`[\s,./\-]+`)
)
