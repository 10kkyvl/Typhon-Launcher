package compat

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestCleanRepackerKeepsOnlyKnownSlugs(t *testing.T) {
	cases := map[string]string{
		"fitgirl":  "fitgirl",
		"FitGirl":  "fitgirl",
		"  dodi  ": "dodi",
		"":         RepackerUnknown,
		"какой-то новый сборщик":  RepackerUnknown,
		"Silksong.v1.0-RUNE":      RepackerUnknown,
		"/Users/egorripa/Games/x": RepackerUnknown,
	}
	for in, want := range cases {
		if got := cleanRepacker(in); got != want {
			t.Errorf("cleanRepacker(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCleanVersionDropsAnythingUnexpected(t *testing.T) {
	cases := map[string]string{
		"1.0.28518":             "1.0.28518",
		"1.4":                   "1.4",
		"v1.4":                  "",
		"1.0 [FitGirl Repack]":  "",
		"":                      "",
		"C:\\Games\\Silksong":   "",
		strings.Repeat("9", 40): "",
	}
	for in, want := range cases {
		if got := cleanVersion(in); got != want {
			t.Errorf("cleanVersion(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestShortOSVersionKeepsTwoComponents(t *testing.T) {
	cases := map[string]string{
		"15.6.1":                       "15.6",
		"15.6":                         "15.6",
		"macOS 15.6":                   "15.6",
		"26":                           "26.0",
		"":                             "",
		"неизвестно":                   "",
		"darwin":                       "",
		"Windows 10 Pro (build 19045)": "",
	}
	for in, want := range cases {
		got, ok := shortOSVersion(in)
		if got != want || ok != (want != "") {
			t.Errorf("shortOSVersion(%q) = %q, %v, want %q", in, got, ok, want)
		}
	}
}

func TestChipFamilyNeverKeepsTheBrandString(t *testing.T) {
	cases := map[string]string{
		"Apple M4":     "apple_m4",
		"Apple M1 Pro": "apple_m1",
		"Apple M3 Max": "apple_m3",
		"Intel(R) Core(TM) i7-9750H CPU @ 2.60GHz": ChipIntel,
		"Apple M9":    ChipUnknown,
		"Unknown CPU": ChipUnknown,
		"":            ChipUnknown,
	}
	for in, want := range cases {
		if got := chipFamily(in); got != want {
			t.Errorf("chipFamily(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCleanReasonNarrowsToKnownCodes(t *testing.T) {
	cases := map[string]string{
		"library.launch_failed": "launch_failed",
		"launch_failed":         "launch_failed",
		"self_exit":             "self_exit",
		"library.some_new_code": reasonUnknown,
		"":                      reasonUnknown,
		"нет бутыля":            reasonUnknown,
	}
	for in, want := range cases {
		if got := cleanReason(in); got != want {
			t.Errorf("cleanReason(%q) = %q, want %q", in, got, want)
		}
	}
}

// Игра без канонического идентификатора не с чем сводить на сервере, а
// StateUnknown не говорит о совместимости ничего — зато говорит, что у
// человека стоит эта игра.
func TestGameReportSkipsWhatCannotBeAggregated(t *testing.T) {
	cases := []struct {
		name string
		st   Status
		b    Build
	}{
		{"нет идентификатора", Status{State: StateWorks}, Build{GameID: ""}},
		{"идентификатор не числовой", Status{State: StateWorks}, Build{GameID: "canon-1"}},
		{"состояние неизвестно", Status{State: StateUnknown}, Build{GameID: "232567"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, ok := gameReport(c.st, c.b); ok {
				t.Fatal("запись отправлена, хотя отправлять её нечего")
			}
		})
	}
}

func TestGameReportCarriesTheVerdictAndTheBuild(t *testing.T) {
	got, ok := gameReport(
		Status{State: StateBroken, LastCode: "library.launch_failed"},
		Build{GameID: "232567", Repacker: "FitGirl", Version: "1.0.28518"},
	)
	if !ok {
		t.Fatal("запись не собралась")
	}
	want := GameReport{
		GameID: "232567", Repacker: "fitgirl", Version: "1.0.28518",
		State: "broken", Reason: "launch_failed",
	}
	if got != want {
		t.Fatalf("gameReport = %+v, want %+v", got, want)
	}
}

// Успех не везёт причину: причины у него нет, а пустое поле на сервере пришлось
// бы отличать от «причина неизвестна».
func TestWorkingGameCarriesNoReason(t *testing.T) {
	got, _ := gameReport(Status{State: StateWorks, LastCode: "self_exit"}, Build{GameID: "1"})
	if got.Reason != "" {
		t.Fatalf("Reason = %q, want пусто", got.Reason)
	}
}

// Главная проверка приватности: то, что журнал знает про машину, не должно
// оказаться в байтах, которые уходят на сервер. Проверяется не поле за полем, а
// весь маршалированный отчёт — так тест переживёт добавление нового поля.
func TestReportCarriesNothingIdentifying(t *testing.T) {
	s := newTestService(t)
	s.RecordLaunchFailure("g1", "library.launch_failed", `не найден бутыль /Users/egorripa/Library/Application Support/Typhon/bottles/x`)
	s.RecordLaunchFailure("g1", "library.launch_failed", `не найден бутыль /Users/egorripa/Library/Application Support/Typhon/bottles/x`)
	s.RecordSession("g2", 30*time.Minute, true)

	report, ok := s.buildReport("client-uuid", "0.3.1",
		Env{OSVersion: "15.6", CrossOver: "26.3", Chip: "apple_m4"},
		func(localID string) (Build, bool) {
			switch localID {
			case "g1":
				return Build{GameID: "232567", Repacker: "fitgirl", Version: "1.0.28518"}, true
			case "g2":
				return Build{GameID: "1020", Repacker: "", Version: ""}, true
			}
			return Build{}, false
		})
	if !ok {
		t.Fatal("отчёт не собрался")
	}

	raw, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	body := string(raw)
	forbidden := []string{
		"egorripa",            // имя учётной записи
		"/Users/",             // локальный путь
		"Application Support", // локальный путь
		"бутыль",              // свободный текст ошибки
		"не найден",           // свободный текст ошибки
		"Silksong",            // сырое название релиза
		"magnet:",             // ссылка на раздачу
		"Apple M4",            // строка бренда процессора
	}
	for _, f := range forbidden {
		if strings.Contains(body, f) {
			t.Errorf("в отчёте оказалось %q:\n%s", f, body)
		}
	}
	if len(report.Games) != 2 {
		t.Fatalf("игр в отчёте %d, want 2:\n%s", len(report.Games), body)
	}
}

// Отчёт без игр отправлять незачем: он не несёт ни одного наблюдения, зато
// сообщает, что на этой машине стоит лаунчер такой-то версии.
func TestEmptyJournalMakesNoReport(t *testing.T) {
	s := newTestService(t)
	report, ok := s.buildReport("client-uuid", "0.3.1", Env{OSVersion: "15.6", Chip: "apple_m4"},
		func(string) (Build, bool) { return Build{}, false })
	if !ok {
		t.Fatal("отчёт не собрался")
	}
	if len(report.Games) != 0 {
		t.Fatalf("игр в отчёте %d, want 0", len(report.Games))
	}
	if report.Empty() != true {
		t.Fatal("Empty() = false для отчёта без игр")
	}
}

// Вызывающий не обязан помнить, что чип надо сводить к семейству: сырая строка
// бренда обязана свестись сама, иначе однажды она уедет целиком.
func TestRawBrandStringIsNarrowedOnItsWay(t *testing.T) {
	s := newTestService(t)
	s.RecordSession("g1", 30*time.Minute, true)

	report, ok := s.buildReport("c", "0.3.1",
		Env{OSVersion: "macOS 15.6.1", CrossOver: "26.3", Chip: "Apple M4"},
		func(string) (Build, bool) { return Build{GameID: "232567"}, true })
	if !ok {
		t.Fatal("отчёт не собрался")
	}

	if report.Env.Chip != "apple_m4" {
		t.Fatalf("Chip = %q, want apple_m4", report.Env.Chip)
	}
	if report.Env.OSVersion != "15.6" {
		t.Fatalf("OSVersion = %q, want 15.6", report.Env.OSVersion)
	}
}

func TestDedupeGamesLeavesOneRowPerBuild(t *testing.T) {
	works := GameReport{GameID: "1", Repacker: "dodi", Version: "1.0", State: string(StateWorks)}
	crashed := GameReport{GameID: "1", Repacker: "dodi", Version: "1.0", State: string(StateBroken), Reason: "launch_failed"}
	missing := crashed
	missing.Reason = "executable_missing"
	other := GameReport{GameID: "2", Repacker: RepackerUnknown, State: string(StateWorks)}
	otherVersion := works
	otherVersion.Version = "1.1"

	cases := []struct {
		name string
		in   []GameReport
		want []GameReport
	}{
		{"без повторов", []GameReport{works, other}, []GameReport{works, other}},
		{"другая версия — другая сборка", []GameReport{works, otherVersion}, []GameReport{works, otherVersion}},
		{"один вердикт дважды", []GameReport{works, other, works}, []GameReport{works, other}},
		{"противоречие выбрасывает сборку", []GameReport{works, other, crashed}, []GameReport{other}},
		{"разные причины отказа", []GameReport{crashed, missing}, []GameReport{{GameID: "1", Repacker: "dodi", Version: "1.0", State: string(StateBroken), Reason: reasonUnknown}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := dedupeGames(tc.in)
			if len(got) != len(tc.want) {
				t.Fatalf("dedupeGames = %+v, want %+v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("dedupeGames = %+v, want %+v", got, tc.want)
				}
			}
		})
	}
}

func splitTestReport(n int, wide bool) Report {
	games := make([]GameReport, n)
	for i := range games {
		games[i] = GameReport{GameID: strconv.Itoa(100000000000000000 + i), Repacker: RepackerUnknown, State: string(StateWorks)}
		if wide {
			games[i].Repacker = "masquerade"
			games[i].Version = wideVersion
			games[i].State = string(StateBroken)
			games[i].Reason = "executable_missing"
		}
	}
	return Report{
		ClientID:   "3f1c2b8e-4d5a-4f6b-9c7d-8e9f0a1b2c3d",
		AppVersion: "1.0.0",
		Env:        Env{OSVersion: "26.6", CrossOver: "26.3", Chip: "apple_m4"},
		Games:      games,
	}
}

func TestSplitReportKeepsEveryGameWithinServerLimits(t *testing.T) {
	cases := []struct {
		name      string
		games     int
		wide      bool
		wantParts func(int) bool
	}{
		{"одна игра", 1, false, func(n int) bool { return n == 1 }},
		{"ровно по лимиту числа игр", maxReportGames, false, func(n int) bool { return n == 1 }},
		{"на одну игру больше лимита", maxReportGames + 1, false, func(n int) bool { return n == 2 }},
		{"тысяча игр", 1000, false, func(n int) bool { return n == 2 }},
		{"полторы тысячи игр", 1500, false, func(n int) bool { return n == 3 }},
		{"лимит по байтам раньше лимита по числу игр", maxReportGames, true, func(n int) bool { return n >= 2 }},
		{"тысяча длинных игр", 1000, true, func(n int) bool { return n >= 3 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			report := splitTestReport(tc.games, tc.wide)
			parts, err := splitReport(report, maxReportGames, maxReportBytes)
			if err != nil {
				t.Fatalf("splitReport: %v", err)
			}
			if !tc.wantParts(len(parts)) {
				t.Fatalf("частей %d на %d игр", len(parts), tc.games)
			}
			var joined []GameReport
			for i, part := range parts {
				body, err := json.Marshal(part)
				if err != nil {
					t.Fatalf("marshal части %d: %v", i, err)
				}
				if len(body) > backendMaxRequestBytes {
					t.Errorf("часть %d занимает %d байт, лимит сервера %d", i, len(body), backendMaxRequestBytes)
				}
				if len(part.Games) == 0 || len(part.Games) > backendMaxReportGames {
					t.Errorf("в части %d %d игр, лимит сервера %d", i, len(part.Games), backendMaxReportGames)
				}
				if part.ClientID != report.ClientID || part.AppVersion != report.AppVersion || part.Env != report.Env {
					t.Errorf("часть %d потеряла общую часть отчёта: %+v", i, part)
				}
				joined = append(joined, part.Games...)
			}
			if len(joined) != len(report.Games) {
				t.Fatalf("в частях %d игр из %d", len(joined), len(report.Games))
			}
			for i := range joined {
				if joined[i] != report.Games[i] {
					t.Fatalf("игра %d изменилась или встала не на своё место: %+v", i, joined[i])
				}
			}
		})
	}
}

func TestSplitReportWithoutGamesHasNothingToSend(t *testing.T) {
	parts, err := splitReport(splitTestReport(0, false), maxReportGames, maxReportBytes)
	if err != nil || len(parts) != 0 {
		t.Fatalf("parts = %d, err = %v, want нет частей и нет ошибки", len(parts), err)
	}
}

func TestSplitReportRefusesWhatCannotFit(t *testing.T) {
	report := splitTestReport(3, true)
	envelope, err := json.Marshal(Report{ClientID: report.ClientID, AppVersion: report.AppVersion, Env: report.Env, Games: []GameReport{}})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	oneGame, err := json.Marshal(report.Games[0])
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	if _, err := splitReport(report, maxReportGames, len(envelope)+len(oneGame)-1); !errors.Is(err, errReportTooLarge) {
		t.Fatalf("игра больше лимита: err = %v, want errReportTooLarge", err)
	}
	parts, err := splitReport(report, maxReportGames, len(envelope)+len(oneGame))
	if err != nil || len(parts) != 3 {
		t.Fatalf("по одной игре в запросе: parts = %d, err = %v", len(parts), err)
	}
}
