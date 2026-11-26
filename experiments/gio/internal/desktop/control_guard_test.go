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
	"accounts_view.go": "selectable account row that fills a card",
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

// TestVerticalFlexesUseColumnForFlexibleRows keeps a Flexed row out of a
// vertical layout.Flex. Under the shell stage the height is unbounded, and
// layout.Flex would give that whole height to the Flexed row. ui.Column reads
// the window height instead and reports the true content height.
func TestVerticalFlexesUseColumnForFlexibleRows(t *testing.T) {
	// These files lay out the window itself, outside the scrolling stage, so
	// their height is always bounded.
	outsideStage := map[string]bool{"shell.go": true, "onboarding_view.go": true}
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") || outsideStage[name] {
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
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			method, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || method.Sel.Name != "Layout" {
				return true
			}
			literal, ok := method.X.(*ast.CompositeLit)
			if !ok {
				return true
			}
			if kind, ok := literal.Type.(*ast.SelectorExpr); !ok || kind.Sel.Name != "Flex" {
				return true
			}
			vertical := false
			for _, element := range literal.Elts {
				pair, ok := element.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				if key, ok := pair.Key.(*ast.Ident); ok && key.Name == "Axis" {
					if value, ok := pair.Value.(*ast.SelectorExpr); ok && value.Sel.Name == "Vertical" {
						vertical = true
					}
				}
			}
			if !vertical {
				return true
			}
			for _, arg := range call.Args {
				child, ok := arg.(*ast.CallExpr)
				if !ok {
					continue
				}
				if flexed, ok := child.Fun.(*ast.SelectorExpr); ok && flexed.Sel.Name == "Flexed" {
					t.Errorf("%s: vertical layout.Flex has a Flexed row; use ui.Column with ui.Fill", fset.Position(child.Pos()))
				}
			}
			return true
		})
	}
}
