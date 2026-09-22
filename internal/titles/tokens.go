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

// Номер сборки в фидах носит буквенный суффикс («1.25h», «1.0.2.22714S»),
// смешанный хвост («0.4.2f9», «1.34.0r20112»), части через подчёркивание
// («1.25.8.27_5409») и иногда буквенный префикс («CL16601», «g1.06»).
// Оборванная на первой же букве версия оставляла её хвост в названии игры.
// Dotted build branches are single letters or letter-number codes (.F,
// .r40883.f). Keep edition words, languages and architecture tags separate.
const verNumericPart = `(?:[0-9][0-9a-zа-я]*|[a-zа-я]\b|[abfr][0-9][0-9a-zа-я]*\b)`
const verNumeric = `(?:\d+[a-zа-я]+[0-9][0-9a-zа-я._]*|\d+[a-zа-я]|\d+(?:[._]` + verNumericPart + `){0,8})`
const verCode = `(?:[a-zа-я]{1,6}(?:\.[0-9a-zа-я]+)*\.[0-9][0-9a-zа-я]*(?:\.[0-9a-zа-я]+)*|[a-zа-я]{2,6}[-_:][0-9a-zа-я]*[0-9][0-9a-zа-я]*(?:[._:][0-9a-zа-я]+)*|[a-zа-я]{1,6}\d[0-9a-zа-я._]*)`
const verNumber = `(?:` + verNumeric + `|` + verCode + `)`

var (
	reBuildVer       = regexp.MustCompile(`(?i)\bbuild[.#\-_ ]+(` + verNumeric + `|cl[._-]?\d+)`)
	reUpdateVer      = regexp.MustCompile(`(?i)\bupdate[.#\-_ ]+(` + verNumeric + `)`)
	rePatchVer       = regexp.MustCompile(`(?i)\bpatch[.#\-_ ]+(` + verNumeric + `)`)
	reHotfixVer      = regexp.MustCompile(`(?i)\bhotfix[.#\-_ ]+(` + verNumeric + `)`)
	reVVer           = regexp.MustCompile(`(?i)\bv(?:[.]+\s*)?(` + verNumeric + `)`)
	reVVerCode       = regexp.MustCompile(`(?i)\bv[.]+\s*(` + verCode + `)`)
	reVVerDirectCode = regexp.MustCompile(`(?i)\bv([brsuv]\d[0-9a-zа-я._]*)`)
	// A separated V is ambiguous; extractVersion preserves title numerals.
	reVVerSpace = regexp.MustCompile(`(?i)\bv\s+(` + verNumeric + `)`)
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

	// Сборка продолжается ревизией через дефис или attached hash: «v1.0.10.1-r82675-b2»,
	// «v1.0.0-5db267» и «v1.1.0+e0d30dc159».
	reVersionContinuation = regexp.MustCompile(`(?i)^\s*(?:/\s*(?:online\s*)?\d+(?:\.\d+)*(?:\s+online\b)?|\+(?:\s*\d+(?:\.\d+)+|\d{3,}(?:[._-][0-9a-zа-я]+)*|[a-zа-я][0-9a-zа-я]*[0-9][0-9a-zа-я]*(?:[._-][0-9a-zа-я]+)*)|[-_:#][0-9a-zа-я]*[0-9][0-9a-zа-я]*(?:[._-][0-9a-zа-я]+)*)`)
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
