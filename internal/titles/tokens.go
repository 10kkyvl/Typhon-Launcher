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

// Номер сборки в фидах носит буквенный суффикс («1.25h», «1.0.2.22714S») и
// части через подчёркивание («1.25.8.27_5409»). Оборванная на первой же букве
// версия оставляла её хвост в названии игры.
const verNumber = `\d+(?:[._]\d+){0,6}[a-z]{0,2}`

var (
	reBuildVer  = regexp.MustCompile(`(?i)\bbuild[.\-_ ]+(` + verNumber + `)\b`)
	reUpdateVer = regexp.MustCompile(`(?i)\bupdate[.\-_ ]+(` + verNumber + `)\b`)
	rePatchVer  = regexp.MustCompile(`(?i)\bpatch[.\-_ ]+(` + verNumber + `)\b`)
	reHotfixVer = regexp.MustCompile(`(?i)\bhotfix[.\-_ ]+(` + verNumber + `)\b`)
	reVVer      = regexp.MustCompile(`(?i)\bv(?:[.]+\s*)?(` + verNumber + `)\b`)
	// A separated V is ambiguous; extractVersion preserves title numerals.
	reVVerSpace = regexp.MustCompile(`(?i)\bv\s+(` + verNumber + `)\b`)
	reRVer      = regexp.MustCompile(`(?i)\br(\d{4,6})\b`)
	reDLCCount  = regexp.MustCompile(`(?i)\+\s*(\d+)\s*(?:dlc(?:'s|s)?|дополнени\p{L}*)`)

	reExt = regexp.MustCompile(`(?i)\.(rar|zip|iso|7z|exe|torrent)$`)
	reURL = regexp.MustCompile(`(?i)https?://\S+`)
	reWWW = regexp.MustCompile(`(?i)\bwww\.[^\s\]\)]+`)

	reBracket = regexp.MustCompile(`\[[^\[\]]*\]|\([^()]*\)|\{[^{}]*\}`)

	reMulti    = regexp.MustCompile(`(?i)\bmulti[\-]?\d{0,3}\b`)
	reSteamRip = regexp.MustCompile(`(?i)\bsteam[\-\s._]?rip\b`)
	// Только после разделителя: голое «Portable» в хвосте принадлежит названию
	// игры, как в Persona 3 Portable.
	rePortable = regexp.MustCompile(`(?i)[|/]\s*portable\b`)
	// Кириллическое «Репак» пишут наравне с латинским, а имя репакера за «от»
	// бывает потеряно — «Frontline Zed (2019) RePack от». Граница слова здесь
	// не годится: в RE2 \b знает только ASCII и после «Репак» не срабатывает.
	reRepackBy = regexp.MustCompile(`(?i)(?:\bre-?pack|(?:^|[\s._|-])ре-?пак)(?:[\s._-]+(?:by|от)(?:[\s._-]+[\p{L}0-9_.-]+)?)?(?:[\s._:,|-]|$)`)
	// Маркер раздачи целиком: «RePack от R.G. Механики», «Steam-Rip от Chovka».
	// Якорь на начало сегмента — «repack» посреди названия маркером не считается.
	reMarkerRepack = regexp.MustCompile(`(?i)^[\[(]?(re-?pack|ре-?пак|(?:steam|egs|epic|uplay|origin|gog|ea)[\s._-]?rip|rip|рип)[\])]?(?:[\s.:,_-]+|$)(?:(?:от|by|from)[\s.:,_-]*)?(.*)$`)

	// Сборка продолжается ревизией через дефис: «v1.0.10.1-r82675-b2».
	reVersionContinuation = regexp.MustCompile(`(?i)^\s*(?:/\s*(?:online\s*)?\d+(?:\.\d+)*(?:\s+online\b)?|\+\s*\d+(?:\.\d+)+|[-_:][a-z]{0,2}\d+(?:[._]\d+)*|\+\d{3,}(?:[._]\d+)*)`)
	reVersionBracket      = regexp.MustCompile(`^\s*\(([^()]*)\)`)
	reReleaseBracketStart = regexp.MustCompile(`(?i)^(?:v[.\s]*\d|build[.\s]+\d|update[.\s]+\d|patch[.\s]+\d)`)
	reRepackerBracket     = regexp.MustCompile(`(?i)^(fitgirl|dodi)\s+repack\b`)
	reBonusSuffix         = regexp.MustCompile(`(?i)\s+\+\s+(?:bonus\s+(?:content|osts?|soundtrack)|windows\s+7\s+fix|essential\s+mods\s+and\s+fixes)\b.*$`)
	// Имя репакера в хвосте пишут и без слова «repack»: «[RePack] by xatab».
	reByRepacker   = regexp.MustCompile(`(?i)[\s.,|)\]–—-]+by[\s.:_-]+([\p{L}0-9_. -]+?)\s*$`)
	reDecimalDot   = regexp.MustCompile(`(\d)\.(\d)`)
	reSepRun       = regexp.MustCompile(`[._\-]+`)
	reSpaceRun     = regexp.MustCompile(`\s+`)
	reYear         = regexp.MustCompile(`^(19[7-9]\d|20\d{2})$`)
	reBracketSplit = regexp.MustCompile(`[\s,./\-|]+`)

	// reNeverMatch стоит на месте языковых шаблонов, когда список языков пуст:
	// альтернатива из нуля вариантов совпала бы с пустой строкой везде.
	reNeverMatch = regexp.MustCompile(`$^`)
)
