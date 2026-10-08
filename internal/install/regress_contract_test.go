package install

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// Интерфейс видит записи установки по именам полей и строковым значениям, и
// переименование на стороне Go не ломает сборку, а тихо оставляет пустое поле
// или кнопку, которая никогда не покажется. Эти проверки сверяют то, что
// фронтенд читает, с тем, что отдаёт пакет (рядом с uicodes_test.go, который
// сверяет коды ошибок).

const installTS = "services/install.ts"

func readFrontend(t *testing.T, rel string) string {
	t.Helper()
	path := filepath.Join("..", "..", "frontend", "src", "lib", filepath.FromSlash(rel))
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("не прочитан %s: %v", path, err)
	}
	return string(body)
}

func tsUnion(t *testing.T, src, name string) []string {
	t.Helper()
	block := regexp.MustCompile(`(?s)export type ` + name + ` =(.*?);`).FindStringSubmatch(src)
	if block == nil {
		t.Fatalf("в install.ts нет типа %s", name)
	}
	var out []string
	for _, m := range regexp.MustCompile(`'([a-z_0-9]+)'`).FindAllStringSubmatch(block[1], -1) {
		out = append(out, m[1])
	}
	sort.Strings(out)
	return out
}

func tsFields(t *testing.T, src, name string) []string {
	t.Helper()
	block := regexp.MustCompile(`(?s)export interface ` + name + ` \{(.*?)\n\}`).FindStringSubmatch(src)
	if block == nil {
		t.Fatalf("в install.ts нет интерфейса %s", name)
	}
	var out []string
	for _, m := range regexp.MustCompile(`(?m)^\s+([A-Za-z0-9_]+)\??:`).FindAllStringSubmatch(block[1], -1) {
		out = append(out, m[1])
	}
	return out
}

func goJSONFields(v any) map[string]bool {
	out := map[string]bool{}
	typ := reflect.TypeOf(v)
	for i := 0; i < typ.NumField(); i++ {
		name, _, _ := strings.Cut(typ.Field(i).Tag.Get("json"), ",")
		if name != "" && name != "-" {
			out[name] = true
		}
	}
	return out
}

// goConsts читает значения констант из исходника: так новая константа Go
// попадает под проверку без правки теста.
func goConsts(t *testing.T, file string, match func(name string, typ ast.Expr) bool) []string {
	t.Helper()
	parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
	if err != nil {
		t.Fatalf("разбор %s: %v", file, err)
	}
	var out []string
	for _, decl := range parsed.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for i, ident := range vs.Names {
				if !match(ident.Name, vs.Type) || i >= len(vs.Values) {
					continue
				}
				lit, ok := vs.Values[i].(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					continue
				}
				value, err := strconv.Unquote(lit.Value)
				if err != nil {
					t.Fatalf("константа %s: %v", ident.Name, err)
				}
				out = append(out, value)
			}
		}
	}
	sort.Strings(out)
	return out
}

func ofType(name string) func(string, ast.Expr) bool {
	return func(_ string, typ ast.Expr) bool {
		id, ok := typ.(*ast.Ident)
		return ok && id.Name == name
	}
}

func withPrefix(prefix string) func(string, ast.Expr) bool {
	return func(name string, _ ast.Expr) bool { return strings.HasPrefix(name, prefix) }
}

func TestFrontendUnionsMatchTheGoConstants(t *testing.T) {
	src := readFrontend(t, installTS)
	cases := []struct {
		union string
		want  []string
	}{
		{"InstallStatus", goConsts(t, "model.go", ofType("Status"))},
		{"InstallType", goConsts(t, "model.go", ofType("Type"))},
		{"InstallMode", goConsts(t, "model.go", withPrefix("Mode"))},
		{"RemovalMethod", goConsts(t, "removal.go", ofType("RemovalMethod"))},
	}
	for _, tc := range cases {
		t.Run(tc.union, func(t *testing.T) {
			if len(tc.want) == 0 {
				t.Fatal("в исходнике Go не найдено ни одной константы")
			}
			got := tsUnion(t, src, tc.union)
			if strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Fatalf("install.ts: %v\nGo:         %v", got, tc.want)
			}
		})
	}
}

func TestFrontendReadsOnlyFieldsTheGoStructsSerialize(t *testing.T) {
	src := readFrontend(t, installTS)
	cases := []struct {
		iface string
		model any
	}{
		{"Installation", Installation{}},
		{"Plan", Plan{}},
		{"PlanInfo", PlanInfo{}},
		{"Candidate", Candidate{}},
		{"RemovalInfo", RemovalInfo{}},
		{"RemoveOptions", RemoveOptions{}},
		{"StartOptions", StartOptions{}},
	}
	for _, tc := range cases {
		t.Run(tc.iface, func(t *testing.T) {
			have := goJSONFields(tc.model)
			fields := tsFields(t, src, tc.iface)
			if len(fields) == 0 {
				t.Fatalf("у интерфейса %s не найдено полей", tc.iface)
			}
			for _, field := range fields {
				if !have[field] {
					t.Errorf("install.ts читает %s.%s, а пакет такого поля не отдаёт", tc.iface, field)
				}
			}
		})
	}
}

func TestFrontendListensToEventsTheServiceEmits(t *testing.T) {
	emitted := map[string]bool{}
	for _, name := range goConsts(t, "service.go", withPrefix("event")) {
		emitted[name] = true
	}
	if len(emitted) < 6 {
		t.Fatalf("в сервисе найдено %d событий, ожидалось 6", len(emitted))
	}
	store := readFrontend(t, "stores/install.ts")
	heard := regexp.MustCompile(`Events\.On\('(install:[a-z]+)'`).FindAllStringSubmatch(store, -1)
	if len(heard) == 0 {
		t.Fatal("stores/install.ts не слушает ни одного события установки")
	}
	for _, m := range heard {
		if !emitted[m[1]] {
			t.Errorf("интерфейс слушает %q, а сервис его не отправляет", m[1])
		}
		delete(emitted, m[1])
	}
	for name := range emitted {
		t.Errorf("сервис отправляет %q, а stores/install.ts его не слушает", name)
	}
}

// Запись о задании сохраняется как есть и читается после обновления лаунчера:
// ключ installations.json нельзя убрать или переименовать молча. Новые ключи
// добавлять можно.
func TestInstallationKeepsItsStoredKeys(t *testing.T) {
	want := []string{
		"archivePath", "bytesDone", "bytesTotal", "candidates", "chainStep", "completedAt", "contentRoot", "currentFile",
		"destination", "detectedVersion", "downloadId", "engine", "error", "executable", "extraInstallers", "gameId", "id",
		"installerPath", "interactive", "manualInstaller", "mode", "name", "origin", "owned", "ownedDestination", "progress",
		"silent", "skipRegister", "sourcePath", "startedAt", "status", "type", "unattended", "uninstall", "uninstallUnknown",
		"versionSource", "workingDir",
	}
	have := goJSONFields(Installation{})
	for _, key := range want {
		if !have[key] {
			t.Errorf("ключ %q записи установки пропал: записи, сохранённые прошлой версией, потеряют это поле", key)
		}
	}
}
