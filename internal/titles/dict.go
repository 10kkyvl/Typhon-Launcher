package titles

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync/atomic"
	"unicode/utf8"
)

const (
	MaxDictBytes   = 1 << 20
	MaxDictEntries = 2000
	MaxDictToken   = 128
	DictVersion    = 1
)

var (
	ErrDictVersion  = errors.New("titles: unsupported dictionary version")
	ErrDictTooLarge = errors.New("titles: dictionary is too large")
	ErrDictEntry    = errors.New("titles: invalid dictionary entry")
)

//go:embed titles-dict.json
var builtinDict []byte

// Spec — обновляемая часть разбора названий. Поля, отсутствующие в слое,
// оставляют значение предыдущего слоя; списки при этом заменяются целиком, а
// словари сливаются по ключу, и пустое значение ключ удаляет.
type Spec struct {
	Version           int               `json:"version"`
	LangCodes         []string          `json:"langCodes,omitempty"`
	ArchTokens        map[string]string `json:"archTokens,omitempty"`
	ReleaseTags       map[string]string `json:"releaseTags,omitempty"`
	EditionPhrases    []string          `json:"editionPhrases,omitempty"`
	ReleasePhraseTags map[string]string `json:"releasePhraseTags,omitempty"`
	BracketFiller     []string          `json:"bracketFiller,omitempty"`
	RepackerPriority  []string          `json:"repackerPriority,omitempty"`
	GameTypes         []string          `json:"gameTypes,omitempty"`
}

type Dict struct {
	langCodes         []string
	langSet           map[string]struct{}
	archTokens        map[string]string
	releaseSingleTags map[string]string
	phraseTable       []phraseTok
	bracketFiller     map[string]struct{}
	repackerPriority  []string
	gameTypes         map[string]struct{}

	reLangCombo  *regexp.Regexp
	reLangSingle *regexp.Regexp
}

type phraseTok struct {
	norm []string
	kind string
}

func ParseSpec(data []byte) (Spec, error) {
	if len(data) > MaxDictBytes {
		return Spec{}, fmt.Errorf("%w: %d bytes, limit %d", ErrDictTooLarge, len(data), MaxDictBytes)
	}
	var spec Spec
	if err := json.Unmarshal(data, &spec); err != nil {
		return Spec{}, fmt.Errorf("titles: decode dictionary: %w", err)
	}
	if spec.Version < 1 || spec.Version > DictVersion {
		return Spec{}, fmt.Errorf("%w: %d", ErrDictVersion, spec.Version)
	}
	if err := spec.validate(); err != nil {
		return Spec{}, err
	}
	return spec, nil
}

func (s Spec) validate() error {
	lists := map[string][]string{
		"langCodes":        s.LangCodes,
		"bracketFiller":    s.BracketFiller,
		"editionPhrases":   s.EditionPhrases,
		"repackerPriority": s.RepackerPriority,
		"gameTypes":        s.GameTypes,
	}
	for name, list := range lists {
		if len(list) > MaxDictEntries {
			return fmt.Errorf("%w: %s has %d entries, limit %d", ErrDictTooLarge, name, len(list), MaxDictEntries)
		}
		for _, item := range list {
			if err := checkToken(name, item); err != nil {
				return err
			}
		}
	}

	maps := map[string]map[string]string{
		"archTokens":        s.ArchTokens,
		"releaseTags":       s.ReleaseTags,
		"releasePhraseTags": s.ReleasePhraseTags,
	}
	for name, table := range maps {
		if len(table) > MaxDictEntries {
			return fmt.Errorf("%w: %s has %d entries, limit %d", ErrDictTooLarge, name, len(table), MaxDictEntries)
		}
		for key, value := range table {
			if err := checkToken(name, key); err != nil {
				return err
			}
			if value == "" {
				continue
			}
			if err := checkToken(name, value); err != nil {
				return err
			}
		}
	}
	return nil
}

func checkToken(field, token string) error {
	if strings.TrimSpace(token) == "" {
		return fmt.Errorf("%w: %s has a blank entry", ErrDictEntry, field)
	}
	if utf8.RuneCountInString(token) > MaxDictToken {
		return fmt.Errorf("%w: %s entry is longer than %d characters", ErrDictEntry, field, MaxDictToken)
	}
	return nil
}

// Merge накладывает слой поверх текущей спеки. Списки заменяются целиком,
// потому что для repackerPriority значим порядок, а для editionPhrases —
// возможность выбросить ошибочную фразу. Словари сливаются по ключу: пустое
// значение удаляет ключ, иначе один лишний репакер требовал бы копировать
// весь список.
func (s Spec) Merge(layer Spec) Spec {
	out := s
	out.Version = DictVersion
	if layer.LangCodes != nil {
		out.LangCodes = append([]string(nil), layer.LangCodes...)
	}
	if layer.EditionPhrases != nil {
		out.EditionPhrases = append([]string(nil), layer.EditionPhrases...)
	}
	if layer.BracketFiller != nil {
		out.BracketFiller = append([]string(nil), layer.BracketFiller...)
	}
	if layer.RepackerPriority != nil {
		out.RepackerPriority = append([]string(nil), layer.RepackerPriority...)
	}
	if layer.GameTypes != nil {
		out.GameTypes = append([]string(nil), layer.GameTypes...)
	}
	out.ArchTokens = mergeTable(out.ArchTokens, layer.ArchTokens)
	out.ReleaseTags = mergeTable(out.ReleaseTags, layer.ReleaseTags)
	out.ReleasePhraseTags = mergeTable(out.ReleasePhraseTags, layer.ReleasePhraseTags)
	return out
}

func mergeTable(base, layer map[string]string) map[string]string {
	out := make(map[string]string, len(base)+len(layer))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range layer {
		if v == "" {
			delete(out, k)
			continue
		}
		out[k] = v
	}
	return out
}

func NewDict(spec Spec) (*Dict, error) {
	if err := spec.validate(); err != nil {
		return nil, err
	}

	d := &Dict{
		langCodes:         make([]string, 0, len(spec.LangCodes)),
		langSet:           make(map[string]struct{}, len(spec.LangCodes)),
		archTokens:        make(map[string]string, len(spec.ArchTokens)),
		releaseSingleTags: make(map[string]string, len(spec.ReleaseTags)),
		gameTypes:         make(map[string]struct{}, len(spec.GameTypes)),
		bracketFiller:     make(map[string]struct{}, len(spec.BracketFiller)),
	}
	for _, filler := range spec.BracketFiller {
		d.bracketFiller[strings.ToLower(strings.TrimSpace(filler))] = struct{}{}
	}

	for _, code := range spec.LangCodes {
		code = strings.ToUpper(strings.TrimSpace(code))
		if code == "" {
			continue
		}
		if _, ok := d.langSet[strings.ToLower(code)]; ok {
			continue
		}
		d.langSet[strings.ToLower(code)] = struct{}{}
		d.langCodes = append(d.langCodes, code)
	}
	for key, value := range spec.ArchTokens {
		d.archTokens[strings.ToLower(strings.TrimSpace(key))] = value
	}
	for key, value := range spec.ReleaseTags {
		d.releaseSingleTags[strings.ToLower(strings.TrimSpace(key))] = value
	}
	for _, kind := range spec.GameTypes {
		d.gameTypes[gameTypeKey(kind)] = struct{}{}
	}
	for _, repacker := range spec.RepackerPriority {
		d.repackerPriority = append(d.repackerPriority, strings.ToLower(strings.TrimSpace(repacker)))
	}

	for _, phrase := range spec.EditionPhrases {
		d.phraseTable = append(d.phraseTable, phraseTok{norm: normPhrase(phrase), kind: "edition"})
	}
	phrases := make([]string, 0, len(spec.ReleasePhraseTags))
	for phrase := range spec.ReleasePhraseTags {
		phrases = append(phrases, phrase)
	}
	sort.Strings(phrases)
	for _, phrase := range phrases {
		d.phraseTable = append(d.phraseTable, phraseTok{norm: normPhrase(phrase), kind: "tag:" + spec.ReleasePhraseTags[phrase]})
	}
	sort.SliceStable(d.phraseTable, func(i, j int) bool {
		return len(d.phraseTable[i].norm) > len(d.phraseTable[j].norm)
	})

	alternation := strings.Join(d.langCodes, "|")
	if alternation == "" {
		// Пустая альтернатива в regexp совпадает с пустой строкой в любом
		// месте: без языков языковые правила должны не срабатывать вовсе.
		d.reLangCombo = reNeverMatch
		d.reLangSingle = reNeverMatch
	} else {
		combo, err := regexp.Compile(`(?i)\b(?:` + alternation + `)(?:[/\-](?:` + alternation + `))+\b`)
		if err != nil {
			return nil, fmt.Errorf("titles: compile language pattern: %w", err)
		}
		single, err := regexp.Compile(`(?i)\b(?:` + alternation + `)\b`)
		if err != nil {
			return nil, fmt.Errorf("titles: compile language pattern: %w", err)
		}
		d.reLangCombo, d.reLangSingle = combo, single
	}

	return d, nil
}

func normPhrase(phrase string) []string {
	words := strings.Fields(phrase)
	norm := make([]string, len(words))
	for i, w := range words {
		norm[i] = normKey(w)
	}
	return norm
}

func gameTypeKey(kind string) string {
	return strings.ToLower(strings.Join(strings.Fields(kind), " "))
}

// Builtin возвращает вшитую спеку. Она же служит нижним слоем для серверного и
// пользовательского словарей.
func Builtin() (Spec, error) {
	return ParseSpec(builtinDict)
}

var (
	active  atomic.Pointer[Dict]
	initErr error
)

// Вшитый словарь разбирается один раз при загрузке пакета. Ошибка здесь
// означает испорченную сборку, а не состояние, из которого можно продолжать:
// она запоминается в initErr, main.go читает её через Ready() и отказывается
// стартовать. Пустой словарь в active кладётся только для того, чтобы Parse не
// разыменовывал nil на пути к этому отказу, и виден сразу — разбор вырождается
// в голую нормализацию, а не в правдоподобный результат.
func init() {
	spec, err := Builtin()
	if err != nil {
		initErr = err
		active.Store(emptyDict())
		return
	}
	dict, err := NewDict(spec)
	if err != nil {
		initErr = err
		active.Store(emptyDict())
		return
	}
	active.Store(dict)
}

func emptyDict() *Dict {
	dict, err := NewDict(Spec{Version: DictVersion})
	if err != nil {
		return &Dict{reLangCombo: reNeverMatch, reLangSingle: reNeverMatch}
	}
	return dict
}

// Ready сообщает, удалось ли разобрать вшитый словарь.
func Ready() error { return initErr }

// Active — словарь, на котором работают пакетные Parse, Repacker и IsGameType.
func Active() *Dict { return active.Load() }

// SetActive заменяет активный словарь целиком. Замена атомарна: разбор
// названий идёт из фонового рефетча источников одновременно с чтением из UI.
func SetActive(d *Dict) {
	if d == nil {
		return
	}
	active.Store(d)
}

func Parse(raw string) Parsed { return Active().Parse(raw) }

func Repacker(tags []string) string { return Active().Repacker(tags) }

func IsGameType(kind string) bool { return Active().IsGameType(kind) }

// IsGameType сообщает, годится ли тип каталожной записи для матчинга репаков.
// Пустой тип — это запись, которую бэкенд ещё не переливал, и она считается
// игрой: иначе матчинг встал бы у всех до конца переливки.
func (d *Dict) IsGameType(kind string) bool {
	if d == nil {
		return true
	}
	key := gameTypeKey(kind)
	if key == "" {
		return true
	}
	if len(d.gameTypes) == 0 {
		return true
	}
	_, ok := d.gameTypes[key]
	return ok
}

func (d *Dict) Repacker(tags []string) string {
	if d == nil || len(tags) == 0 {
		return ""
	}
	set := make(map[string]bool, len(tags))
	for _, t := range tags {
		set[strings.ToLower(t)] = true
	}
	for _, p := range d.repackerPriority {
		if set[p] {
			return p
		}
	}
	return ""
}

// isFiller отвечает за слова, которые внутри скобок не несут названия:
// «+ all DLC» без них остаётся неопознанной группой и целиком уезжает в
// название игры.
func (d *Dict) isFiller(lower string) bool {
	_, ok := d.bracketFiller[lower]
	return ok
}

func (d *Dict) isLangCode(lower string) bool {
	_, ok := d.langSet[lower]
	return ok
}
