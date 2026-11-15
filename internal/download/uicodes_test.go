package download

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
	"unicode"
)

var downloadTSCodePattern = regexp.MustCompile(`'(download\.[a-z0-9_]+)':`)

// uncodedPlayerErrors names the package-level errors.New sentinels with
// Russian text that may stay without a code. An entry only goes away: a new
// sentinel the player can meet gets a uierr code instead. "debt:" marks one
// that does reach the player and still waits for its code.
var uncodedPlayerErrors = map[string]string{
	"errStorageClosed":     "never leaves the package: onWriteError drops it before anything fails",
	"errPartAndFinalExist": "always behind download.part_promotion_failed, which the window reads first",
	"errMetainfoTooLarge":  "behind download.torrent_read_failed on FetchMetadata; debt: AddTask (updates apply and repair) wraps it in uncoded text",
	"errDuplicateTask":     "debt: AddTask returns it to updates, whose repair error shows the text",
	// Repoint runs inside a relocate job, which shows the error text.
	"errRepointEmptyPath":    "debt: relocate job error",
	"errRepointSamePath":     "debt: relocate job error",
	"errRepointNested":       "debt: relocate job error",
	"errDownloadsActive":     "debt: relocate job error",
	"errRepointVerifyFailed": "debt: relocate job error",
	"errRepointNotDir":       "debt: relocate job error",
	"errRepointNonRegular":   "debt: relocate job error",
	"errRepointOldRootStuck": "debt: relocate job error",
	"errRepointTargetNotDir": "debt: relocate job error",
}

type parsedDir struct {
	fset  *token.FileSet
	files []*ast.File
}

// parseDownloadTree parses every non-test file of the package and of the
// packages under it, build tags ignored, grouped by directory.
func parseDownloadTree(t *testing.T) map[string]parsedDir {
	t.Helper()
	dirs := map[string]parsedDir{}
	fset := token.NewFileSet()
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		dir := filepath.Dir(path)
		pd := dirs[dir]
		pd.fset = fset
		pd.files = append(pd.files, f)
		dirs[dir] = pd
		return nil
	})
	if err != nil {
		t.Fatalf("package tree not read: %v", err)
	}
	return dirs
}

// importName is the name f uses for the package at path, or "" when f does
// not import it.
func importName(f *ast.File, path string) string {
	for _, spec := range f.Imports {
		p, err := strconv.Unquote(spec.Path.Value)
		if err != nil || p != path {
			continue
		}
		if spec.Name != nil {
			return spec.Name.Name
		}
		return path[strings.LastIndex(path, "/")+1:]
	}
	return ""
}

func isCall(e ast.Expr, pkg string, names ...string) (*ast.CallExpr, bool) {
	call, ok := e.(*ast.CallExpr)
	if !ok || pkg == "" {
		return nil, false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return nil, false
	}
	x, ok := sel.X.(*ast.Ident)
	if !ok || x.Name != pkg {
		return nil, false
	}
	for _, n := range names {
		if sel.Sel.Name == n {
			return call, true
		}
	}
	return nil, false
}

// stringConsts maps the package-level string constants of one directory to
// their values; a constant defined by another one resolves through it.
func stringConsts(files []*ast.File) map[string]string {
	exprs := map[string]ast.Expr{}
	for _, f := range files {
		for _, decl := range f.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.CONST {
				continue
			}
			for _, spec := range gen.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for i, name := range vs.Names {
					if i < len(vs.Values) {
						exprs[name.Name] = vs.Values[i]
					}
				}
			}
		}
	}
	out := map[string]string{}
	for name := range exprs {
		e, seen := exprs[name], map[string]bool{name: true}
		for {
			if lit, ok := e.(*ast.BasicLit); ok && lit.Kind == token.STRING {
				if v, err := strconv.Unquote(lit.Value); err == nil {
					out[name] = v
				}
				break
			}
			id, ok := e.(*ast.Ident)
			if !ok || seen[id.Name] || exprs[id.Name] == nil {
				break
			}
			seen[id.Name] = true
			e = exprs[id.Name]
		}
	}
	return out
}

// goErrorCodes collects the code of every uierr.New and uierr.Wrap in the
// tree, whether written as a literal or as a package constant. A code it
// cannot resolve fails the test: it could be anything.
func goErrorCodes(t *testing.T, dirs map[string]parsedDir) []string {
	t.Helper()
	seen := map[string]bool{}
	for _, pd := range dirs {
		consts := stringConsts(pd.files)
		for _, f := range pd.files {
			uierrName := importName(f, "typhon/internal/uierr")
			ast.Inspect(f, func(n ast.Node) bool {
				e, ok := n.(ast.Expr)
				if !ok {
					return true
				}
				call, ok := isCall(e, uierrName, "New", "Wrap")
				if !ok || len(call.Args) == 0 {
					return true
				}
				switch arg := call.Args[0].(type) {
				case *ast.BasicLit:
					if v, err := strconv.Unquote(arg.Value); err == nil && arg.Kind == token.STRING {
						seen[v] = true
						return true
					}
				case *ast.Ident:
					if v, ok := consts[arg.Name]; ok {
						seen[v] = true
						return true
					}
				}
				t.Errorf("%s: the code of this uierr call is neither a string literal nor a package constant", pd.fset.Position(call.Pos()))
				return true
			})
		}
	}
	out := make([]string, 0, len(seen))
	for code := range seen {
		out = append(out, code)
	}
	sort.Strings(out)
	return out
}

func tsErrorCodes(t *testing.T, path string) []string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("не прочитан %s: %v", path, err)
	}
	seen := map[string]bool{}
	for _, match := range downloadTSCodePattern.FindAllStringSubmatch(string(body), -1) {
		seen[match[1]] = true
	}
	out := make([]string, 0, len(seen))
	for code := range seen {
		out = append(out, code)
	}
	sort.Strings(out)
	return out
}

// Коды ошибок — контракт между Go и интерфейсом: переименование с одной
// стороны не ломает сборку, а тихо возвращает пользователю запасной текст.
func TestErrorCodesMatchTheFrontendTable(t *testing.T) {
	goCodes := goErrorCodes(t, parseDownloadTree(t))
	if len(goCodes) < 20 {
		t.Fatalf("в пакете найдено %d кодов, ожидалось не меньше 20", len(goCodes))
	}

	tsPath := filepath.Join("..", "..", "frontend", "src", "lib", "install", "installErrors.ts")
	tsCodes := tsErrorCodes(t, tsPath)

	inTS := map[string]bool{}
	for _, code := range tsCodes {
		inTS[code] = true
	}
	for _, code := range goCodes {
		if !inTS[code] {
			t.Errorf("код %q возвращается из Go, но не переводится в installErrors.ts", code)
		}
	}

	inGo := map[string]bool{}
	for _, code := range goCodes {
		inGo[code] = true
	}
	for _, code := range tsCodes {
		if !inGo[code] {
			t.Errorf("код %q переводится в installErrors.ts, но Go его не возвращает", code)
		}
	}
}

func hasCyrillic(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Cyrillic, r) {
			return true
		}
	}
	return false
}

// A package-level errors.New with Russian text is a sentence meant for the
// player, and the window translates codes, not sentences (I-15).
func TestRussianSentinelErrorsCarryACode(t *testing.T) {
	found := map[string]bool{}
	for _, pd := range parseDownloadTree(t) {
		for _, f := range pd.files {
			errorsName := importName(f, "errors")
			for _, decl := range f.Decls {
				gen, ok := decl.(*ast.GenDecl)
				if !ok || gen.Tok != token.VAR {
					continue
				}
				for _, spec := range gen.Specs {
					vs, ok := spec.(*ast.ValueSpec)
					if !ok {
						continue
					}
					for i, value := range vs.Values {
						call, ok := isCall(value, errorsName, "New")
						if !ok || len(call.Args) != 1 || i >= len(vs.Names) {
							continue
						}
						lit, ok := call.Args[0].(*ast.BasicLit)
						if !ok || lit.Kind != token.STRING {
							continue
						}
						text, err := strconv.Unquote(lit.Value)
						if err != nil || !hasCyrillic(text) {
							continue
						}
						name := vs.Names[i].Name
						found[name] = true
						if _, allowed := uncodedPlayerErrors[name]; !allowed {
							t.Errorf("%s: %s is Russian text without a code: make it uierr.New with a code and a table entry, or name it in uncodedPlayerErrors with the reason it never reaches the player",
								pd.fset.Position(call.Pos()), name)
						}
					}
				}
			}
		}
	}
	for name := range uncodedPlayerErrors {
		if !found[name] {
			t.Errorf("uncodedPlayerErrors names %s, which is no longer an uncoded Russian sentinel: drop the entry", name)
		}
	}
}
