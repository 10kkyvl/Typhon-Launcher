//go:build windows || (darwin && !devmock)

package install

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// TestReadDiscoveredComponentsRejectsOversizedFile закрывает находку про
// инвариант VI.34: readDiscoveredComponents читал /SAVEINF внешнего
// установщика без ограничения размера. Валидные компоненты стоят в начале
// файла и легко распознались бы наивным чтением — тест доказывает, что файл
// за разумным потолком отклоняется целиком, а не молча усекается до этого
// потолка с частичным (и от того неверным) списком компонентов.
func TestReadDiscoveredComponentsRejectsOversizedFile(t *testing.T) {
	dir := t.TempDir()
	inf := filepath.Join(dir, "discovery.inf")

	var buf bytes.Buffer
	buf.WriteString("[Setup]\nComponents=main,extra\n")
	buf.Write(bytes.Repeat([]byte("x"), discoveryFileLimit))
	if err := os.WriteFile(inf, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	components, reason, err := readDiscoveredComponents(inf, installOptions{SkipExtras: true})
	if err != nil {
		t.Fatalf("readDiscoveredComponents: %v", err)
	}
	if reason == "" {
		t.Fatalf("reason = %q, components = %v — want an explicit refusal instead of parsing a file over the size limit", reason, components)
	}
	if components != nil {
		t.Fatalf("components = %v, want nil when the discovery file exceeds the size limit", components)
	}
}

// TestReadDiscoveredComponentsAcceptsFileWithinLimit — контрольный случай:
// файл в пределах потолка обрабатывается как раньше.
func TestReadDiscoveredComponentsAcceptsFileWithinLimit(t *testing.T) {
	dir := t.TempDir()
	inf := filepath.Join(dir, "discovery.inf")
	if err := os.WriteFile(inf, []byte("[Setup]\nComponents=main,vcredist\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	components, reason, err := readDiscoveredComponents(inf, installOptions{SkipExtras: true})
	if err != nil {
		t.Fatalf("readDiscoveredComponents: %v", err)
	}
	if reason != "" {
		t.Fatalf("reason = %q, want no refusal for a file within the limit", reason)
	}
	if len(components) != 1 || components[0] != "main" {
		t.Fatalf("components = %v, want [main] (vcredist filtered out by SkipExtras)", components)
	}
}
