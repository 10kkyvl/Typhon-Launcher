package compat

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// Лимиты запроса зеркалят сервер (typhon-backend, internal/compat/report.go:
// MaxReportGames и MaxRequestBytes). Лишние игры сервер отклоняет 400, лишние
// байты — 413, и оба отказа окончательные: тот же отчёт не пройдёт и со второй
// попытки, поэтому крупный отчёт режется на запросы заранее.
const (
	maxReportGames = 500
	maxReportBytes = 64 << 10
)

var errReportTooLarge = errors.New("compat: отчёт не помещается в лимит запроса")

// Отчёт устроен так, чтобы в нём не было ни одного поля произвольной формы.
// Всё, что уезжает, — либо слово из закрытого списка, либо строка, прошедшая
// шаблон. Текст ошибки, путь, название релиза и magnet-ссылка попасть в него
// не могут не потому, что их вычищают, а потому, что для них нет места.

// Report — снимок журнала, отправляемый на сервер.
type Report struct {
	ClientID   string       `json:"client_id"`
	AppVersion string       `json:"app_version"`
	Env        Env          `json:"env"`
	Games      []GameReport `json:"games"`
}

// Env — окружение, в котором игры запускались.
type Env struct {
	OSVersion string `json:"os_version"`
	CrossOver string `json:"crossover,omitempty"`
	Chip      string `json:"chip"`
}

// GameReport — вердикт по одной игре. Version отдельно от Repacker: одна и та
// же сборка одного сборщика ведёт себя одинаково, разные версии — нет.
type GameReport struct {
	GameID   string `json:"game_id"`
	Repacker string `json:"repacker"`
	Version  string `json:"version,omitempty"`
	State    string `json:"state"`
	Reason   string `json:"reason,omitempty"`
}

// Build — то, что про игру знает библиотека и не знает журнал: чем игра
// является в общем каталоге и какой сборкой она установлена.
type Build struct {
	GameID   string
	Repacker string
	Version  string
}

const (
	// RepackerUnknown стоит и у найденных сканом игр, у которых релиза нет
	// вовсе, и у сборщиков, которых лаунчер не знает. Разделять их незачем:
	// в обоих случаях сказать про сборку нечего.
	RepackerUnknown = "unknown"
	ChipUnknown     = "unknown"
	ChipIntel       = "intel"
)

// repackers — подмножество разбираемых лаунчером слагов, которое умеет принять
// бэкенд: слаг уезжает на сервер, там список тоже закрытый, и незнакомое имя
// отклоняет весь отчёт целиком. Поэтому список растёт только вслед за
// задеплоенным бэкендом, а не вслед за словарём названий: остальные сборщики
// уезжают как RepackerUnknown и статистику не ломают.
var repackers = map[string]bool{
	"fitgirl":    true,
	"dodi":       true,
	"elamigos":   true,
	"xatab":      true,
	"kaoskrew":   true,
	"masquerade": true,
}

var chips = map[string]bool{
	"apple_m1": true,
	"apple_m2": true,
	"apple_m3": true,
	"apple_m4": true,
	"apple_m5": true,
	ChipIntel:  true,
}

// reasons — коды отказа, которые лаунчер отличает друг от друга. Это коды
// uierr с отброшенным префиксом пакета: заводить рядом второй словарь тех же
// самых отказов значило бы держать два списка, которые разойдутся.
var reasons = map[string]bool{
	"launch_failed":      true,
	"runtime_failed":     true,
	"executable_missing": true,
	"self_exit":          true,
}

const reasonUnknown = "unknown"

// gameIDPattern короче, чем у остальной телеметрии: сервер хранит
// идентификатор в bigint и берёт не больше 18 цифр, а одна неверная строка
// отклоняет весь отчёт.
var (
	gameIDPattern    = regexp.MustCompile(`^[1-9][0-9]{0,17}$`)
	versionPattern   = regexp.MustCompile(`^[0-9][0-9A-Za-z._-]{0,31}$`)
	crossOverPattern = regexp.MustCompile(`^[0-9]{1,4}(\.[0-9]{1,4}){0,2}$`)
	osVersionPattern = regexp.MustCompile(`^[0-9]{1,3}\.[0-9]{1,3}$`)
)

func cleanRepacker(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if repackers[s] {
		return s
	}
	return RepackerUnknown
}

// cleanVersion отбрасывает версию целиком, если она не уложилась в шаблон.
// Пустая версия честнее подчищенной: сборки сойдутся по репакеру, а мусор из
// названия релиза наружу не уедет.
func cleanVersion(s string) string {
	s = strings.TrimSpace(s)
	if versionPattern.MatchString(s) {
		return s
	}
	return ""
}

func cleanReason(code string) string {
	code = strings.TrimSpace(code)
	if i := strings.LastIndex(code, "."); i >= 0 {
		code = code[i+1:]
	}
	if reasons[code] {
		return code
	}
	return reasonUnknown
}

func cleanCrossOver(s string) string {
	s = strings.TrimSpace(s)
	if crossOverPattern.MatchString(s) {
		return s
	}
	return ""
}

// shortOSVersion оставляет от версии системы только старшие две части: точный
// номер сборки сужает круг машин, а про совместимость говорит не больше.
func shortOSVersion(s string) (string, bool) {
	s = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(s), "macOS"))
	s = strings.TrimSpace(s)
	parts := strings.Split(s, ".")
	if len(parts) > 2 {
		parts = parts[:2]
	}
	if len(parts) == 1 {
		parts = append(parts, "0")
	}
	out := strings.Join(parts, ".")
	if osVersionPattern.MatchString(out) {
		return out, true
	}
	return "", false
}

// chipFamily сводит строку бренда процессора к семейству. Сырая строка не
// уезжает никогда: на Intel это конкретная модель, то есть отпечаток железа.
func chipFamily(brand string) string {
	b := strings.ToLower(brand)
	switch {
	case strings.Contains(b, "intel"):
		return ChipIntel
	case strings.Contains(b, "apple"):
		for _, gen := range []string{"m1", "m2", "m3", "m4", "m5"} {
			if strings.Contains(b, " "+gen) {
				return "apple_" + gen
			}
		}
		return ChipUnknown
	default:
		return ChipUnknown
	}
}

// cleanChip принимает и уже сведённое семейство, и сырую строку бренда: так
// вызывающему не нужно помнить, что именно он передаёт, а строка бренда не
// может уехать наружу из-за того, что кто-то забыл её свести.
func cleanChip(s string) string {
	if v := strings.ToLower(strings.TrimSpace(s)); chips[v] {
		return v
	}
	return chipFamily(s)
}

// gameReport переводит вердикт журнала в строку отчёта. Второе значение ложно,
// когда отправлять нечего: без канонического идентификатора запись не с чем
// сводить, а StateUnknown ничего не говорит о совместимости — зато говорит,
// что у человека стоит эта игра.
func gameReport(st Status, b Build) (GameReport, bool) {
	if !gameIDPattern.MatchString(b.GameID) {
		return GameReport{}, false
	}
	r := GameReport{
		GameID:   b.GameID,
		Repacker: cleanRepacker(b.Repacker),
		Version:  cleanVersion(b.Version),
		State:    string(st.State),
	}
	switch st.State {
	case StateWorks:
	case StateBroken:
		r.Reason = cleanReason(st.LastCode)
	default:
		return GameReport{}, false
	}
	return r, true
}

// Empty говорит, что отправлять нечего. Отчёт без единого наблюдения не несёт
// ничего полезного, зато сообщает, что на этой машине стоит лаунчер.
func (r Report) Empty() bool { return len(r.Games) == 0 }

// buildReport переводит журнал в отчёт. resolve отвечает на вопрос, которого
// журнал не знает: чем игра является в общем каталоге и какой сборкой она
// установлена. Игра, про которую ответа нет, в отчёт не попадает.
//
// Второе значение ложно, когда у окружения нет версии macOS. Общая статистика
// отвечает на вопрос, идёт ли игра под CrossOver, и без версии macOS сервер
// отчёт не принимает. На Windows её нет в принципе: игра там запускается
// нативно и про CrossOver ничего не говорит, поэтому отчёта с Windows нет.
func (s *Service) buildReport(clientID, appVersion string, env Env, resolve func(localID string) (Build, bool)) (Report, bool) {
	osVersion, ok := shortOSVersion(env.OSVersion)
	if !ok {
		return Report{}, false
	}
	all := s.All()
	games := make([]GameReport, 0, len(all))
	for _, st := range all {
		build, ok := resolve(st.GameID)
		if !ok {
			continue
		}
		if g, ok := gameReport(st, build); ok {
			games = append(games, g)
		}
	}
	return Report{
		ClientID:   clientID,
		AppVersion: appVersion,
		Env: Env{
			OSVersion: osVersion,
			CrossOver: cleanCrossOver(env.CrossOver),
			Chip:      cleanChip(env.Chip),
		},
		Games: dedupeGames(games),
	}, true
}

// splitReport режет отчёт на запросы, каждый из которых укладывается в лимиты
// сервера. Сервер раскладывает наблюдения построчно по ключу (машина, игра,
// сборка, окружение) и ничего не удаляет, поэтому несколько запросов с одним
// client_id оставляют в базе то же, что один большой: ни одна игра не теряется,
// и отбирать из журнала нечего. Порядок игр и общая часть отчёта сохраняются.
//
// Размер считается по сериализованным кускам, а итог каждого запроса ещё раз
// проверяется по настоящему json.Marshal: расчёт, разошедшийся с сериализацией,
// даёт отказ, а не запрос, который сервер оборвёт.
func splitReport(r Report, maxGames, maxBytes int) ([]Report, error) {
	head := r
	head.Games = []GameReport{}
	envelope, err := json.Marshal(head)
	if err != nil {
		return nil, fmt.Errorf("encode compat report envelope: %w", err)
	}

	var parts []Report
	start, size := 0, len(envelope)
	for i, g := range r.Games {
		raw, err := json.Marshal(g)
		if err != nil {
			return nil, fmt.Errorf("encode compat report game %d: %w", i, err)
		}
		add := len(raw)
		if i > start {
			add++
		}
		if i > start && (i-start >= maxGames || size+add > maxBytes) {
			parts = append(parts, reportPart(r, start, i))
			start, size, add = i, len(envelope), len(raw)
		}
		if size+add > maxBytes {
			return nil, fmt.Errorf("%w: игра %d занимает %d байт при лимите %d", errReportTooLarge, i, size+add, maxBytes)
		}
		size += add
	}
	if start < len(r.Games) {
		parts = append(parts, reportPart(r, start, len(r.Games)))
	}

	for i, part := range parts {
		body, err := json.Marshal(part)
		if err != nil {
			return nil, fmt.Errorf("encode compat report part %d: %w", i+1, err)
		}
		if len(body) > maxBytes || len(part.Games) > maxGames {
			return nil, fmt.Errorf("%w: часть %d из %d занимает %d байт и %d игр при лимитах %d и %d",
				errReportTooLarge, i+1, len(parts), len(body), len(part.Games), maxBytes, maxGames)
		}
	}
	return parts, nil
}

func reportPart(r Report, from, to int) Report {
	part := r
	part.Games = r.Games[from:to:to]
	return part
}

// dedupeGames сводит строки одной сборки в одну: сервер отклоняет отчёт
// целиком, если сборка в нём повторяется, а так бывает, когда одна игра стоит
// в библиотеке дважды. Одинаковые вердикты сливаются, противоречащие
// выбрасываются: выбрать один из них молча значило бы соврать.
func dedupeGames(games []GameReport) []GameReport {
	type key struct{ gameID, repacker, version string }
	keyOf := func(g GameReport) key { return key{g.GameID, g.Repacker, g.Version} }

	index := make(map[key]int, len(games))
	conflicts := make(map[key]bool)
	out := make([]GameReport, 0, len(games))
	for _, g := range games {
		k := keyOf(g)
		i, seen := index[k]
		switch {
		case !seen:
			index[k] = len(out)
			out = append(out, g)
		case out[i].State != g.State:
			conflicts[k] = true
		case out[i].Reason != g.Reason:
			out[i].Reason = reasonUnknown
		}
	}
	if len(conflicts) == 0 {
		return out
	}
	kept := out[:0]
	for _, g := range out {
		if !conflicts[keyOf(g)] {
			kept = append(kept, g)
		}
	}
	return kept
}
