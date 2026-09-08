package compat

import (
	"testing"

	"typhon/internal/titles"
)

// Слаг, который лаунчер отправляет в статистику, обязан быть тем же, что
// разбирается из названия релиза. Слаг, которого разбор не производит, в отчёт
// попасть не может, и его присутствие здесь означало бы разошедшиеся списки.
func TestRepackersComeFromTitles(t *testing.T) {
	spec, err := titles.Builtin()
	if err != nil {
		t.Fatalf("titles.Builtin: %v", err)
	}
	known := make(map[string]bool, len(spec.RepackerPriority))
	for _, slug := range spec.RepackerPriority {
		known[slug] = true
	}
	for slug := range repackers {
		if !known[slug] {
			t.Errorf("repacker %q принимается статистикой, но разбор названий его не выдаёт", slug)
		}
	}
}
