package diskusage

import (
	"context"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"

	"typhon/internal/diskreason"
	"typhon/internal/download"
	"typhon/internal/uierr"
)

func TestWarningsCarryACodePerAreaAndCause(t *testing.T) {
	r := newRig(t)
	r.addGame(t, "a", 10)
	r.lib.updErr = errors.New("disk full")
	r.rel.err = errors.New("journal unreadable")
	r.shots.err = uierr.New("screenshots.no_library", "библиотека не настроена")
	r.snaps.err = uierr.New("savebackup.disabled", "резервные копии недоступны")
	rep, err := r.svc.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, w := range rep.Warnings {
		got[w.Code] = w.Area
	}
	want := map[string]string{
		WarnSizeNotSaved:        "games",
		WarnMovesUnchecked:      "leftovers",
		WarnScreenshotsNoFolder: "screenshots",
		WarnSnapshotsDisabled:   "snapshots",
	}
	for code, area := range want {
		if got[code] != area {
			t.Errorf("warning %s: area %q, want %q (warnings %+v)", code, got[code], area, rep.Warnings)
		}
	}

	r.shots.err = errors.New("read screenshots folder: permission denied")
	r.snaps.err = errors.New("read snapshots: permission denied")
	rep, err = r.svc.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	codes := map[string]bool{}
	for _, w := range rep.Warnings {
		codes[w.Code] = true
	}
	if !codes[WarnScreenshotsUnreadable] || !codes[WarnSnapshotsUnreadable] {
		t.Fatalf("warnings = %+v, want the unreadable codes for uncoded causes", rep.Warnings)
	}
}

func TestUnreadableLeftoverFolderIsAWarningWithItsCode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("folder permissions do not deny listing on Windows")
	}
	r := newRig(t)
	if err := os.Chmod(r.downloads, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(r.downloads, 0o700); err != nil { //nolint:gosec // G302: a folder needs its execute bit to be listed again
			t.Error(err)
		}
	})
	if _, err := os.ReadDir(r.downloads); err == nil {
		t.Skip("running with rights that ignore folder permissions")
	}
	rep, err := r.svc.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Warnings) != 1 || rep.Warnings[0].Code != WarnFolderUnreadable || rep.Warnings[0].Area != "leftovers" {
		t.Fatalf("warnings = %+v", rep.Warnings)
	}
}

func TestScanDownloadReasonCodesCoverEveryRefusal(t *testing.T) {
	r := newRig(t)
	mk := func(id string) download.Download {
		writeBytes(t, filepath.Join(r.downloads, id, "d.bin"), 5)
		return download.Download{ID: id, Name: id, Destination: r.downloads, Status: download.StatusCompleted}.WithRoot(id)
	}
	r.dl.items = []download.Download{mk("upd"), mk("nolist")}
	r.inst.blocked["upd"] = uierr.New("updates.download_in_use", "x")
	r.inst.blocked["nolist"] = uierr.New("install.no_game_list", "x")
	rep, err := r.svc.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"upd": diskreason.UpdateOwned, "nolist": reasonGamesUnchecked}
	for _, d := range rep.Downloads {
		if d.ReasonCode != want[d.ID] {
			t.Errorf("%s reasonCode = %q, want %q", d.ID, d.ReasonCode, want[d.ID])
		}
	}
}

type goPackage struct {
	fset  *token.FileSet
	files []*ast.File
}

func parsePackage(t *testing.T, dir string) goPackage {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	pkg := goPackage{fset: token.NewFileSet()}
	for _, path := range matches {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(pkg.fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		pkg.files = append(pkg.files, f)
	}
	if len(pkg.files) == 0 {
		t.Fatalf("no Go files in %s", dir)
	}
	return pkg
}

// codedVars maps each package-level variable set to uierr.New("code", ...) to
// its code.
func codedVars(pkg goPackage) map[string]string {
	out := map[string]string{}
	for _, f := range pkg.files {
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
				for i, name := range vs.Names {
					if i >= len(vs.Values) {
						continue
					}
					call, ok := vs.Values[i].(*ast.CallExpr)
					if !ok || len(call.Args) == 0 {
						continue
					}
					sel, ok := call.Fun.(*ast.SelectorExpr)
					if !ok || sel.Sel.Name != "New" {
						continue
					}
					if lit, ok := call.Args[0].(*ast.BasicLit); ok && lit.Kind == token.STRING {
						if code, err := strconv.Unquote(lit.Value); err == nil {
							out[name.Name] = code
						}
					}
				}
			}
		}
	}
	return out
}

// refusalCodes collects the code of every error the named method returns. A
// non-nil return that carries no coded sentinel fails the test, except the
// names in passThrough, which hand on an error a source read elsewhere wrote.
func refusalCodes(t *testing.T, pkg goPackage, method string, passThrough ...string) []string {
	t.Helper()
	coded := codedVars(pkg)
	var codes []string
	found := false
	for _, f := range pkg.files {
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || fn.Name.Name != method {
				continue
			}
			found = true
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				ret, ok := n.(*ast.ReturnStmt)
				if !ok || len(ret.Results) == 0 {
					return true
				}
				if id, ok := ret.Results[0].(*ast.Ident); ok && (id.Name == "nil" || slices.Contains(passThrough, id.Name)) {
					return true
				}
				var hit []string
				ast.Inspect(ret.Results[0], func(n ast.Node) bool {
					if id, ok := n.(*ast.Ident); ok && coded[id.Name] != "" {
						hit = append(hit, coded[id.Name])
					}
					return true
				})
				if len(hit) == 0 {
					t.Errorf("%s: %s refuses with an error that has no code", pkg.fset.Position(ret.Pos()), method)
				}
				codes = append(codes, hit...)
				return true
			})
		}
	}
	if !found {
		t.Fatalf("%s not found", method)
	}
	return codes
}

// The installs answer whether a download may be deleted, with the vetoes of
// the update service registered into them: every refusal either can give has
// a reason code of its own, so a new one cannot fall into "unknown" unseen.
func TestDownloadRefusalSourcesHaveTheirOwnReasonCode(t *testing.T) {
	codes := refusalCodes(t, parsePackage(t, filepath.Join("..", "install")), "CheckDownloadDeletable", "err")
	codes = append(codes, refusalCodes(t, parsePackage(t, filepath.Join("..", "updates")), "CheckDownloadDeletable")...)
	if len(codes) < 4 {
		t.Fatalf("found %d refusal codes (%v), want at least the four known today", len(codes), codes)
	}
	for _, code := range codes {
		got := downloadReasonCode(uierr.New(code, "x"))
		if got == diskreason.Unknown || !slices.Contains(reasonCodes, got) {
			t.Errorf("refusal %s maps to %q", code, got)
		}
	}
}

func diskreasonValues(t *testing.T) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, f := range parsePackage(t, filepath.Join("..", "diskreason")).files {
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
					if i >= len(vs.Values) {
						continue
					}
					if lit, ok := vs.Values[i].(*ast.BasicLit); ok {
						if v, err := strconv.Unquote(lit.Value); err == nil {
							out[name.Name] = v
						}
					}
				}
			}
		}
	}
	return out
}

// Every reason code an owner of the data can answer, and every one disk usage
// writes itself, is in the set the frontend translates.
func TestEveryReasonCodeASourceCanGiveIsInTheSet(t *testing.T) {
	values := diskreasonValues(t)
	for _, dir := range []string{"../install", "../download", "../relocate", "."} {
		pkg := parsePackage(t, filepath.FromSlash(dir))
		for _, f := range pkg.files {
			ast.Inspect(f, func(n ast.Node) bool {
				sel, ok := n.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				if x, ok := sel.X.(*ast.Ident); !ok || x.Name != "diskreason" {
					return true
				}
				code, known := values[sel.Sel.Name]
				if !known || !slices.Contains(reasonCodes, code) {
					t.Errorf("%s: diskreason.%s is not in reasonCodes", pkg.fset.Position(sel.Pos()), sel.Sel.Name)
				}
				return true
			})
		}
	}
	for _, code := range reasonCodes {
		if !strings.HasPrefix(code, "diskusage.reason.") {
			t.Errorf("reason code %q is outside the frontend's key space", code)
		}
	}
}

// Every warning the scan raises names its code from the closed set by
// constant, so the window never gets a warning it cannot word.
func TestEveryWarningIsRaisedWithACodeFromTheSet(t *testing.T) {
	pkg := parsePackage(t, ".")
	names := map[string]string{}
	for _, f := range pkg.files {
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
						if lit, ok := vs.Values[i].(*ast.BasicLit); ok {
							if v, err := strconv.Unquote(lit.Value); err == nil {
								names[name.Name] = v
							}
						}
					}
				}
			}
		}
	}
	calls := 0
	for _, f := range pkg.files {
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "warn" {
				return true
			}
			calls++
			id, ok := call.Args[0].(*ast.Ident)
			if !ok || warningArea[names[id.Name]] == "" {
				t.Errorf("%s: warn is not given a warning code from the set", pkg.fset.Position(call.Pos()))
			}
			return true
		})
	}
	if calls < 5 {
		t.Fatalf("found %d warn calls, want the scan's", calls)
	}
	for code, area := range warningArea {
		if !strings.HasPrefix(code, "diskusage.warning.") || area == "" {
			t.Errorf("warning %q in area %q", code, area)
		}
	}
}
