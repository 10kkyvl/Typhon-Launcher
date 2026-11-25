package handoffgate

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// gateCall is one call of Startup, Stopping or HeldBack found in the sources.
type gateCall struct {
	fn, name, dir, at string
}

// gateCalls parses every non-test Go file under root and returns the gate
// calls with the name each one passes. A name that is neither a string
// literal nor a string constant of the calling package is an error: the
// check must see every name.
func gateCalls(root string) ([]gateCall, error) {
	fset := token.NewFileSet()
	byDir := map[string][]*ast.File{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules", "frontend", "build", "bin", ".claude":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, perr := parser.ParseFile(fset, path, nil, 0)
		if perr != nil {
			return perr
		}
		byDir[filepath.Dir(path)] = append(byDir[filepath.Dir(path)], f)
		return nil
	})
	if err != nil {
		return nil, err
	}
	var calls []gateCall
	var errs []error
	for dir, files := range byDir {
		consts := map[string]string{}
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
					for i, id := range vs.Names {
						if i < len(vs.Values) {
							if lit, ok := vs.Values[i].(*ast.BasicLit); ok && lit.Kind == token.STRING {
								if v, uerr := strconv.Unquote(lit.Value); uerr == nil {
									consts[id.Name] = v
								}
							}
						}
					}
				}
			}
		}
		for _, f := range files {
			ast.Inspect(f, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				pkg, ok := sel.X.(*ast.Ident)
				if !ok || pkg.Name != "handoffgate" {
					return true
				}
				arg := -1
				switch sel.Sel.Name {
				case "Startup":
					arg = 1
				case "Stopping", "HeldBack":
					arg = 0
				}
				if arg < 0 || len(call.Args) <= arg {
					return true
				}
				at := fset.Position(call.Pos()).String()
				name := ""
				switch v := call.Args[arg].(type) {
				case *ast.BasicLit:
					if s, uerr := strconv.Unquote(v.Value); uerr == nil {
						name = s
					}
				case *ast.Ident:
					name = consts[v.Name]
				}
				if name == "" {
					errs = append(errs, fmt.Errorf("%s: the gate name is neither a string literal nor a string constant of the package", at))
					return true
				}
				calls = append(calls, gateCall{fn: sel.Sel.Name, name: name, dir: dir, at: at})
				return true
			})
		}
	}
	return calls, errors.Join(errs...)
}

// checkGateNames reports every name two packages register with Startup, and
// every Stopping or HeldBack of a package with a name its Startup does not
// register.
func checkGateNames(calls []gateCall) []string {
	var problems []string
	owner := map[string]gateCall{}
	registered := map[string]map[string]bool{}
	for _, c := range calls {
		if c.fn != "Startup" {
			continue
		}
		if first, ok := owner[c.name]; ok {
			problems = append(problems, fmt.Sprintf("gate name %q registered twice: %s and %s", c.name, first.at, c.at))
			continue
		}
		owner[c.name] = c
		if registered[c.dir] == nil {
			registered[c.dir] = map[string]bool{}
		}
		registered[c.dir][c.name] = true
	}
	for _, c := range calls {
		if c.fn != "Startup" && !registered[c.dir][c.name] {
			problems = append(problems, fmt.Sprintf("%s: %s(%q) names no Startup of its own package", c.at, c.fn, c.name))
		}
	}
	sort.Strings(problems)
	return problems
}

// Two services under one name share one held start: the first one's
// shutdown cancels the second's start, and the first one's start may run
// after its shutdown said nothing would. Every name is unique across the
// launcher, and each shutdown asks under its own service's name.
func TestGateNamesAreUniqueAcrossTheLauncher(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("the module root is not at %s: %v", root, err)
	}
	calls, err := gateCalls(root)
	if err != nil {
		t.Fatal(err)
	}
	startups := 0
	for _, c := range calls {
		if c.fn == "Startup" {
			startups++
		}
	}
	// 25 services were held back when this check was written; fewer means
	// the walk missed sources.
	if startups < 25 {
		t.Fatalf("found %d gate registrations, want at least 25: the walk missed sources", startups)
	}
	if problems := checkGateNames(calls); len(problems) > 0 {
		t.Fatalf("gate names:\n%s", strings.Join(problems, "\n"))
	}
}

func TestGateNameCheckFindsADuplicateAndAStrayName(t *testing.T) {
	calls := []gateCall{
		{fn: "Startup", name: "media", dir: "a", at: "a/a.go:1"},
		{fn: "Startup", name: "media", dir: "b", at: "b/b.go:1"},
		{fn: "Stopping", name: "medai", dir: "a", at: "a/a.go:9"},
	}
	problems := checkGateNames(calls)
	if len(problems) != 2 || !strings.Contains(strings.Join(problems, "\n"), "registered twice") || !strings.Contains(strings.Join(problems, "\n"), `"medai"`) {
		t.Fatalf("problems = %q, want the duplicate and the stray Stopping", problems)
	}
}
