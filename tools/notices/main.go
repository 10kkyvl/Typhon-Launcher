// Command notices generates THIRD_PARTY_NOTICES.md from a curated manifest of
// third-party components. License identification is done by hand in
// manifest.json; this tool only locates and concatenates the verbatim
// license texts already present on disk (Go module cache, npm node_modules,
// licenses vendored into the repository).
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
	NoteEn       string `json:"noteEn"`
	LicenseEn    string `json:"licenseEn"`
}

type Manifest struct {
	Components []Component `json:"components"`
}

type Roots struct {
	GoModCache string
	NpmRoot    string
	RepoRoot   string
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
		if c.Version == "" {
			return fmt.Errorf("component %s: empty version", c.Name)
		}
	case "file":
		if c.LicenseFrom != "" {
			return fmt.Errorf("component %s: source file does not use licenseFrom", c.Name)
		}
	default:
		return fmt.Errorf("component %s: unknown source %q", c.Name, c.Source)
	}
	if c.LicenseFile == "" {
		return fmt.Errorf("component %s: empty licenseFile", c.Name)
	}
	return nil
}

func repoRelativePath(name, rel string) (string, error) {
	if rel == "" {
		return "", fmt.Errorf("component %s: empty repository path", name)
	}
	clean := filepath.Clean(filepath.FromSlash(rel))
	if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("component %s: repository path %q escapes the repository", name, rel)
	}
	return clean, nil
}

func licensePath(c Component, roots Roots) (string, error) {
	switch c.Source {
	case "go":
		dir := c.LicenseFrom
		if dir == "" {
			dir = c.Name + "@" + c.Version
		}
		return filepath.Join(roots.GoModCache, dir, c.LicenseFile), nil
	case "npm":
		dir := c.LicenseFrom
		if dir == "" {
			dir = c.Name
		}
		return filepath.Join(roots.NpmRoot, dir, c.LicenseFile), nil
	case "file":
		rel, err := repoRelativePath(c.Name, c.LicenseFile)
		if err != nil {
			return "", err
		}
		return filepath.Join(roots.RepoRoot, rel), nil
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

func resolveLicenseText(c Component, roots Roots) (string, error) {
	path, err := licensePath(c, roots)
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

func buildGroups(components []Component, roots Roots) ([]Group, []Component, error) {
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
		text, err := resolveLicenseText(c, roots)
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

type frame struct {
	title           string
	intro           string
	summaryHeading  string
	tableHeader     string
	noteFormat      string
	mplHeading      string
	mplIntro        string
	manualHeading   string
	declaredFormat  string
	appendixHeading string
}

var frames = map[string]frame{
	"ru": {
		title:           "# Сторонние компоненты Typhon\n\n",
		intro:           "Этот файл перечисляет сторонние компоненты (библиотеки и модули), распространяемые вместе с Typhon, и приводит дословные тексты их лицензий. Лицензия каждого компонента определена человеком: автоматическое определение лицензий ненадёжно.\n\n",
		summaryHeading:  "## Сводная таблица\n\n",
		tableHeader:     "| Компонент | Версия | Лицензия |\n",
		noteFormat:      "Примечание (%s): %s\n\n",
		mplHeading:      "## MPL-2.0\n\n",
		mplIntro:        "Typhon использует перечисленные ниже модули под лицензией Mozilla Public License 2.0 без изменения их исходного кода. Поэтому исходники этих модулей предоставляются в исходной форме апстрима — по адресам их публичных репозиториев, версии зафиксированы в `go.mod`/`go.sum` Typhon. Дословный текст MPL-2.0 приведён в приложении ниже. Компоненты под MPL-2.0:\n\n",
		manualHeading:   "## Компоненты с отдельными условиями распространения\n\n",
		declaredFormat:  "Заявленная лицензия: %s.\n\n",
		appendixHeading: "## Приложение: тексты лицензий\n\n",
	},
	"en": {
		title:           "# Typhon Third-Party Components\n\n",
		intro:           "This file lists the third-party components (libraries and modules) distributed with Typhon and reproduces the verbatim texts of their licenses. The license of every component was determined by hand: automatic license detection is unreliable.\n\n",
		summaryHeading:  "## Summary table\n\n",
		tableHeader:     "| Component | Version | License |\n",
		noteFormat:      "Note (%s): %s\n\n",
		mplHeading:      "## MPL-2.0\n\n",
		mplIntro:        "Typhon uses the modules listed below under the Mozilla Public License 2.0 without modifying their source code. Their sources are therefore provided in the original upstream form — at the addresses of their public repositories, with versions pinned in Typhon's `go.mod`/`go.sum`. The verbatim text of MPL-2.0 is reproduced in the appendix below. Components under MPL-2.0:\n\n",
		manualHeading:   "## Components with separate distribution terms\n\n",
		declaredFormat:  "Declared license: %s.\n\n",
		appendixHeading: "## Appendix: license texts\n\n",
	},
}

func isASCII(s string) bool {
	for _, r := range s {
		if r > 127 {
			return false
		}
	}
	return true
}

func componentNote(c Component, lang string) (string, error) {
	if lang != "en" {
		return c.Note, nil
	}
	if c.Note != "" && c.NoteEn == "" {
		return "", fmt.Errorf("component %s: note has no noteEn translation", c.Name)
	}
	return c.NoteEn, nil
}

func componentLicense(c Component, lang string) (string, error) {
	if lang != "en" {
		return c.License, nil
	}
	if c.LicenseEn != "" {
		return c.LicenseEn, nil
	}
	if !isASCII(c.License) {
		return "", fmt.Errorf("component %s: license %q has no licenseEn translation", c.Name, c.License)
	}
	return c.License, nil
}

func componentLabel(c Component) string {
	if c.Version == "" {
		return c.Name
	}
	return fmt.Sprintf("%s %s", c.Name, c.Version)
}

func render(m Manifest, groups []Group, manual []Component, lang string) (string, error) {
	f, ok := frames[lang]
	if !ok {
		return "", fmt.Errorf("unknown language %q", lang)
	}

	all := make([]Component, len(m.Components))
	copy(all, m.Components)
	sortComponents(all)

	var b strings.Builder

	b.WriteString(f.title)
	b.WriteString(f.intro)

	b.WriteString(f.summaryHeading)
	b.WriteString(f.tableHeader)
	b.WriteString("| --- | --- | --- |\n")
	for _, c := range all {
		version := c.Version
		if version == "" {
			version = "-"
		}
		license, err := componentLicense(c, lang)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&b, "| %s | %s | %s |\n", c.Name, version, license)
	}
	b.WriteString("\n")

	for _, c := range all {
		if c.ManualReview || c.Note == "" {
			continue
		}
		note, err := componentNote(c, lang)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&b, f.noteFormat, c.Name, note)
	}

	var mpl []Component
	for _, c := range all {
		if c.License == "MPL-2.0" {
			mpl = append(mpl, c)
		}
	}
	if len(mpl) > 0 {
		b.WriteString(f.mplHeading)
		b.WriteString(f.mplIntro)
		for _, c := range mpl {
			fmt.Fprintf(&b, "- %s\n", componentLabel(c))
		}
		b.WriteString("\n")
	}

	if len(manual) > 0 {
		b.WriteString(f.manualHeading)
		for _, c := range manual {
			license, err := componentLicense(c, lang)
			if err != nil {
				return "", err
			}
			note, err := componentNote(c, lang)
			if err != nil {
				return "", err
			}
			fmt.Fprintf(&b, "### %s\n\n", componentLabel(c))
			fmt.Fprintf(&b, f.declaredFormat, license)
			fmt.Fprintf(&b, "%s\n\n", note)
		}
	}

	b.WriteString(f.appendixHeading)
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

	return b.String(), nil
}

func generate(manifestPath string, roots Roots, lang string) ([]byte, error) {
	m, err := loadManifest(manifestPath)
	if err != nil {
		return nil, err
	}
	groups, manual, err := buildGroups(m.Components, roots)
	if err != nil {
		return nil, err
	}
	text, err := render(m, groups, manual, lang)
	if err != nil {
		return nil, err
	}
	return []byte(text), nil
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

func defaultOutput(lang string) string {
	if lang == "ru" {
		return "THIRD_PARTY_NOTICES.md"
	}
	return "THIRD_PARTY_NOTICES." + lang + ".md"
}

func run() error {
	manifestPath := flag.String("manifest", filepath.Join("tools", "notices", "manifest.json"), "path to manifest.json")
	npmRoot := flag.String("npm-root", filepath.Join("frontend", "node_modules"), "path to frontend node_modules")
	repoRoot := flag.String("repo-root", ".", "path to the repository root for licenses vendored in the repository")
	goModCacheFlag := flag.String("gomodcache", "", "override GOMODCACHE (default: go env GOMODCACHE)")
	lang := flag.String("lang", "ru", "language of the generated text around the license texts: ru or en")
	out := flag.String("o", "", "output file path (default: THIRD_PARTY_NOTICES.md, THIRD_PARTY_NOTICES.en.md for -lang en)")
	check := flag.Bool("check", false, "check that the output file matches the manifest instead of writing it")
	flag.Parse()

	if _, ok := frames[*lang]; !ok {
		return fmt.Errorf("unknown language %q", *lang)
	}
	if *out == "" {
		*out = defaultOutput(*lang)
	}

	goModCache := *goModCacheFlag
	if goModCache == "" {
		dir, err := goModCacheDir()
		if err != nil {
			return err
		}
		goModCache = dir
	}

	content, err := generate(*manifestPath, Roots{GoModCache: goModCache, NpmRoot: *npmRoot, RepoRoot: *repoRoot}, *lang)
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
