package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"typhon/internal/selfupdate"
)

var errNoNotes = errors.New("devrelease: -notes file holds no entries")

// noteTemplate is a release note without its version and date: devrelease
// stamps them for each version of the chain. Entry i of a -notes file goes to
// version i, and the last entry to every version after it.
type noteTemplate struct {
	Summary string              `json:"summary,omitempty"`
	Changes []selfupdate.Change `json:"changes"`
}

func (n noteTemplate) note(version string, published time.Time) selfupdate.ReleaseNote {
	return selfupdate.ReleaseNote{Version: version, PublishedAt: published, Summary: n.Summary, Changes: n.Changes}
}

func templateFor(templates []noteTemplate, i int) noteTemplate {
	if i >= len(templates) {
		return templates[len(templates)-1]
	}
	return templates[i]
}

// The CHANGELOG the launcher's "what's new" usually shows is Russian, so the
// sample is too; the format carries one language per entry.
var sampleNotes = []noteTemplate{
	{
		Summary: "Проверочный выпуск из devrelease: обновление находится, скачивается с видимым прогрессом и ставится.",
		Changes: []selfupdate.Change{
			{Kind: selfupdate.ChangeAdded, Text: "После перезапуска лаунчер показывает этот список в «Что нового»"},
			{Kind: selfupdate.ChangeChanged, Text: "Загрузка обновления идёт ощутимое время, её можно отменить и продолжить"},
			{Kind: selfupdate.ChangeFixed, Text: "Перезапуск возвращает тот же тестовый экземпляр с теми же данными"},
		},
	},
	{
		Summary: "Второй проверочный выпуск подряд: источник предлагает его, не перезапускаясь.",
		Changes: []selfupdate.Change{
			{Kind: selfupdate.ChangeAdded, Text: "Ещё одно обновление сразу после предыдущего"},
			{Kind: selfupdate.ChangeFixed, Text: "История «Что нового» хранит записи обеих версий"},
		},
	},
	{
		Summary: "Третий проверочный выпуск, последний в цепочке по умолчанию.",
		Changes: []selfupdate.Change{
			{Kind: selfupdate.ChangeChanged, Text: "После него лаунчер сообщает, что обновлений нет"},
			{Kind: selfupdate.ChangeRemoved, Text: "Ничего настоящего: это проверочная запись"},
		},
	},
}

// loadNotes reads a JSON array of note templates, or returns the built-in
// sample for an empty path. Each entry is checked the way the launcher checks
// a manifest, so a bad file fails here and not as a refused update.
func loadNotes(path string) ([]noteTemplate, error) {
	if path == "" {
		return sampleNotes, nil
	}
	//nolint:gosec // G304: the path is the developer's own -notes flag to this local dev tool, not external input
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read notes: %w", err)
	}
	var templates []noteTemplate
	if err := json.Unmarshal(raw, &templates); err != nil {
		return nil, fmt.Errorf("decode notes %s: %w", path, err)
	}
	if len(templates) == 0 {
		return nil, errNoNotes
	}
	for i, n := range templates {
		if err := n.note("1.0.0", time.Now()).Validate(); err != nil {
			return nil, fmt.Errorf("notes %s entry %d: %w", path, i, err)
		}
	}
	return templates, nil
}
