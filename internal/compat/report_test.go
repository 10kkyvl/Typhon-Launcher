package compat

import (
	"encoding/json"
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
		"15.6.1":     "15.6",
		"15.6":       "15.6",
		"macOS 15.6": "15.6",
		"26":         "26.0",
		"":           "",
		"неизвестно": "",
	}
	for in, want := range cases {
		if got := shortOSVersion(in); got != want {
			t.Errorf("shortOSVersion(%q) = %q, want %q", in, got, want)
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

	report := s.buildReport("client-uuid", "0.3.1",
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
	report := s.buildReport("client-uuid", "0.3.1", Env{}, func(string) (Build, bool) { return Build{}, false })
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

	report := s.buildReport("c", "0.3.1",
		Env{OSVersion: "macOS 15.6.1", CrossOver: "26.3", Chip: "Apple M4"},
		func(string) (Build, bool) { return Build{GameID: "232567"}, true })

	if report.Env.Chip != "apple_m4" {
		t.Fatalf("Chip = %q, want apple_m4", report.Env.Chip)
	}
	if report.Env.OSVersion != "15.6" {
		t.Fatalf("OSVersion = %q, want 15.6", report.Env.OSVersion)
	}
}
