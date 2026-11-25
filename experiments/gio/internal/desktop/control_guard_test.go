package desktop

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// rawControls are Gio material constructors that bypass the shared controls in
// internal/ui. Pages must use ui.View.ButtonOf and friends so a visual fix at
// the owning component reaches every call site.
var rawControls = map[string]bool{"Button": true, "IconButton": true, "ButtonLayout": true, "CheckBox": true, "RadioButton": true, "Switch": true, "List": true}

// rawControlExceptions are the deliberate exceptions, each tied to a control
// the shared kit does not provide yet. Remove an entry when its control moves.
var rawControlExceptions = map[string]string{
	"accounts_view.go":          "selectable account row that fills a card",
	"account_statement_view.go": "completeness checkbox until the shared checkbox lands",
}

func TestPagesUseSharedControlsInsteadOfRawMaterialWidgets(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") || rawControlExceptions[name] != "" {
			continue
		}
		source, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		file, err := parser.ParseFile(token.NewFileSet(), name, source, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if pkg, ok := selector.X.(*ast.Ident); ok && pkg.Name == "material" && rawControls[selector.Sel.Name] {
				t.Errorf("%s uses material.%s; use the shared control in internal/ui", name, selector.Sel.Name)
			}
			return true
		})
	}
}

// TestPagesNameTheirResponsiveThresholds keeps width decisions out of inline
// numbers. A comparison against gtx.Dp(<literal>) hides a breakpoint; use
// ui.Breakpoint*, a ui.Grid, or a named constant that says what the width means.
func TestPagesNameTheirResponsiveThresholds(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		source, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, name, source, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			compare, ok := node.(*ast.BinaryExpr)
			if !ok || (compare.Op != token.LSS && compare.Op != token.LEQ && compare.Op != token.GTR && compare.Op != token.GEQ) {
				return true
			}
			for _, side := range []ast.Expr{compare.X, compare.Y} {
				call, ok := side.(*ast.CallExpr)
				if !ok || len(call.Args) != 1 {
					continue
				}
				selector, ok := call.Fun.(*ast.SelectorExpr)
				literal, isLiteral := call.Args[0].(*ast.BasicLit)
				if ok && selector.Sel.Name == "Dp" && isLiteral && literal.Kind == token.INT {
					t.Errorf("%s: compares a width with gtx.Dp(%s); name the threshold", fset.Position(call.Pos()), literal.Value)
				}
			}
			return true
		})
	}
}
