// Command notices generates THIRD_PARTY_NOTICES.md from a curated manifest of
// third-party components. License identification is done by hand in
// manifest.json; this tool only locates and concatenates the verbatim
// license texts already present on disk (Go module cache, npm node_modules).
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"typhon/internal/storage"
)

type Component struct {
	Name         string `json:"name"`
	Version      string `json:"version"`
	License      string `json:"license"`
	Source       string `json:"source"`
	LicenseFile  string `json:"licenseFile"`
	LicenseFrom  string `json:"licenseFrom"`
	ManualReview bool   `json:"manualReview"`
	Note         string `json:"note"`
}

type Manifest struct {
	Components []Component `json:"components"`
}

type Group struct {
	Hash       string
	License    string
	Text       string
	Components []Component
}

func loadManifest(path string) (Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, fmt.Errorf("read manifest %s: %w", path, err)
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return Manifest{}, fmt.Errorf("parse manifest %s: %w", path, err)
	}
	for i := range m.Components {
		if err := validateComponent(m.Components[i]); err != nil {
			return Manifest{}, fmt.Errorf("manifest %s: %w", path, err)
		}
	}
	return m, nil
}

func validateComponent(c Component) error {
	if c.Name == "" {
		return errors.New("component with empty name")
	}
	if c.License == "" {
		return fmt.Errorf("component %s: empty license", c.Name)
	}
	if c.ManualReview {
		if c.Note == "" {
			return fmt.Errorf("component %s: manualReview requires a note", c.Name)
		}
		return nil
	}
	switch c.Source {
	case "go", "npm":
	default:
		return fmt.Errorf("component %s: unknown source %q", c.Name, c.Source)
	}
	if c.Version == "" {
		return fmt.Errorf("component %s: empty version", c.Name)
	}
	if c.LicenseFile == "" {
		return fmt.Errorf("component %s: empty licenseFile", c.Name)
	}
	return nil
}

func licensePath(c Component, goModCache, npmRoot string) (string, error) {
	switch c.Source {
	case "go":
		dir := c.LicenseFrom
		if dir == "" {
			dir = c.Name + "@" + c.Version
		}
		return filepath.Join(goModCache, dir, c.LicenseFile), nil
	case "npm":
		dir := c.LicenseFrom
		if dir == "" {
			dir = c.Name
		}
		return filepath.Join(npmRoot, dir, c.LicenseFile), nil
	default:
		return "", fmt.Errorf("component %s: unknown source %q", c.Name, c.Source)
	}
}

func readLicenseText(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("license file %s: %w", path, err)
	}
	text := strings.TrimSpace(string(data))
	if text == "" {
		return "", fmt.Errorf("license file %s is empty", path)
	}
	return text, nil
}

func resolveLicenseText(c Component, goModCache, npmRoot string) (string, error) {
	path, err := licensePath(c, goModCache, npmRoot)
	if err != nil {
		return "", fmt.Errorf("component %s %s: %w", c.Name, c.Version, err)
	}
	text, err := readLicenseText(path)
	if err != nil {
		return "", fmt.Errorf("component %s %s: %w", c.Name, c.Version, err)
	}
	return text, nil
}

func sortComponents(cs []Component) {
	sort.Slice(cs, func(i, j int) bool {
		if cs[i].Name != cs[j].Name {
			return cs[i].Name < cs[j].Name
		}
		return cs[i].Version < cs[j].Version
	})
}

func buildGroups(components []Component, goModCache, npmRoot string) ([]Group, []Component, error) {
	ordered := make([]Component, len(components))
	copy(ordered, components)
	sortComponents(ordered)

	var manual []Component
	byHash := map[string]*Group{}
	var order []string

	for _, c := range ordered {
		if c.ManualReview {
			manual = append(manual, c)
			continue
		}
		text, err := resolveLicenseText(c, goModCache, npmRoot)
		if err != nil {
			return nil, nil, err
		}
		sum := sha256.Sum256([]byte(text))
		hash := hex.EncodeToString(sum[:])
		g, ok := byHash[hash]
		if !ok {
			g = &Group{Hash: hash, License: c.License, Text: text}
			byHash[hash] = g
			order = append(order, hash)
		}
		g.Components = append(g.Components, c)
	}

	groups := make([]Group, 0, len(order))
	for _, h := range order {
		groups = append(groups, *byHash[h])
	}
	sort.Slice(groups, func(i, j int) bool {
		return groups[i].Components[0].Name < groups[j].Components[0].Name
	})
	return groups, manual, nil
}

func componentLabel(c Component) string {
	if c.Version == "" {
		return c.Name
	}
	return fmt.Sprintf("%s %s", c.Name, c.Version)
}

func render(m Manifest, groups []Group, manual []Component) string {
	all := make([]Component, len(m.Components))
	copy(all, m.Components)
	sortComponents(all)

	var b strings.Builder

	b.WriteString("# Сторонние компоненты Typhon\n\n")
	b.WriteString("Этот файл перечисляет сторонние компоненты (библиотеки и модули), " +
		"распространяемые вместе с Typhon, и приводит дословные тексты их лицензий. " +
		"Список курируется вручную в `tools/notices/manifest.json` (лицензия каждого " +
		"компонента определена человеком — автоматическое определение лицензий ненадёжно) " +
		"и собирается программой `tools/notices`.\n\n")

	b.WriteString("## Как обновить\n\n")
	b.WriteString("1. Добавьте или обновите запись в `tools/notices/manifest.json`.\n")
	b.WriteString("2. Запустите `go run ./tools/notices -o THIRD_PARTY_NOTICES.md` из корня репозитория.\n")
	b.WriteString("3. Убедитесь, что файл не разошёлся с манифестом, без перезаписи: `go run ./tools/notices -check`.\n\n")

	b.WriteString("## Сводная таблица\n\n")
	b.WriteString("| Компонент | Версия | Лицензия |\n")
	b.WriteString("| --- | --- | --- |\n")
	for _, c := range all {
		version := c.Version
		if version == "" {
			version = "-"
		}
		fmt.Fprintf(&b, "| %s | %s | %s |\n", c.Name, version, c.License)
	}
	b.WriteString("\n")

	for _, c := range all {
		if !c.ManualReview && c.Note != "" {
			fmt.Fprintf(&b, "Примечание (%s): %s\n\n", c.Name, c.Note)
		}
	}

	var mpl []Component
	for _, c := range all {
		if c.License == "MPL-2.0" {
			mpl = append(mpl, c)
		}
	}
	if len(mpl) > 0 {
		b.WriteString("## MPL-2.0\n\n")
		b.WriteString("Typhon использует перечисленные ниже модули под лицензией Mozilla " +
			"Public License 2.0 без изменения их исходного кода. Поэтому исходники этих " +
			"модулей предоставляются в исходной форме апстрима — по адресам их публичных " +
			"репозиториев, версии зафиксированы в `go.mod`/`go.sum` Typhon. Дословный текст " +
			"MPL-2.0 приведён в приложении ниже. Компоненты под MPL-2.0:\n\n")
		for _, c := range mpl {
			fmt.Fprintf(&b, "- %s\n", componentLabel(c))
		}
		b.WriteString("\n")
	}

	if len(manual) > 0 {
		b.WriteString("## Требуют ручной проверки\n\n")
		for _, c := range manual {
			fmt.Fprintf(&b, "### %s\n\n", componentLabel(c))
			fmt.Fprintf(&b, "Заявленная лицензия: %s.\n\n", c.License)
			fmt.Fprintf(&b, "%s\n\n", c.Note)
		}
	}

	b.WriteString("## Приложение: тексты лицензий\n\n")
	for _, g := range groups {
		labels := make([]string, len(g.Components))
		for i, c := range g.Components {
			labels[i] = componentLabel(c)
		}
		fmt.Fprintf(&b, "### %s — %s\n\n", g.License, strings.Join(labels, ", "))
		b.WriteString("```\n")
		b.WriteString(g.Text)
		b.WriteString("\n```\n\n")
	}

	return b.String()
}

func generate(manifestPath, goModCache, npmRoot string) ([]byte, error) {
	m, err := loadManifest(manifestPath)
	if err != nil {
		return nil, err
	}
	groups, manual, err := buildGroups(m.Components, goModCache, npmRoot)
	if err != nil {
		return nil, err
	}
	return []byte(render(m, groups, manual)), nil
}

func goModCacheDir() (string, error) {
	out, err := exec.Command("go", "env", "GOMODCACHE").Output()
	if err != nil {
		return "", fmt.Errorf("go env GOMODCACHE: %w", err)
	}
	dir := strings.TrimSpace(string(out))
	if dir == "" {
		return "", errors.New("go env GOMODCACHE returned empty path")
	}
	return dir, nil
}

func run() error {
	manifestPath := flag.String("manifest", filepath.Join("tools", "notices", "manifest.json"), "path to manifest.json")
	npmRoot := flag.String("npm-root", filepath.Join("frontend", "node_modules"), "path to frontend node_modules")
	goModCacheFlag := flag.String("gomodcache", "", "override GOMODCACHE (default: go env GOMODCACHE)")
	out := flag.String("o", "THIRD_PARTY_NOTICES.md", "output file path")
	check := flag.Bool("check", false, "check that the output file matches the manifest instead of writing it")
	flag.Parse()

	goModCache := *goModCacheFlag
	if goModCache == "" {
		dir, err := goModCacheDir()
		if err != nil {
			return err
		}
		goModCache = dir
	}

	content, err := generate(*manifestPath, goModCache, *npmRoot)
	if err != nil {
		return err
	}

	if *check {
		existing, err := os.ReadFile(*out)
		if err != nil {
			return fmt.Errorf("read %s: %w", *out, err)
		}
		if string(existing) != string(content) {
			return fmt.Errorf("%s is out of date with %s", *out, *manifestPath)
		}
		return nil
	}

	if err := storage.WriteAtomic(*out, content); err != nil {
		return fmt.Errorf("write %s: %w", *out, err)
	}
	return nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "notices:", err)
		//nolint:forbidigo // инвариант «завершение процесса только из main»: это и есть main пакета tools/notices
		os.Exit(1)
	}
}
