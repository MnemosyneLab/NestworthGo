package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"slices"
	"testing"
)

func TestShutdownStackRunsOnceInReverseOrderDespitePanic(t *testing.T) {
	var order []string
	s := &shutdownStack{}
	s.push(func() { order = append(order, "database") })
	s.push(func() { panic("synthetic shutdown failure") })
	s.push(func() { order = append(order, "backup") })

	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("step panic was swallowed")
			}
		}()
		s.run()
	}()
	s.run()

	if want := []string{"backup", "database"}; !slices.Equal(order, want) {
		t.Fatalf("shutdown order = %v, want %v", order, want)
	}
}

// macOS exits inside Wails' OnShutdown hook, so a plain defer in run would
// skip the final backup seal, MCP close and database close.
func TestRunRegistersCleanupWithWailsShutdown(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "main.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var run *ast.FuncDecl
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == "run" {
			run = fn
		}
	}
	if run == nil {
		t.Fatal("run not found")
	}
	defers, hooked := 0, false
	ast.Inspect(run.Body, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.FuncLit:
			return false
		case *ast.DeferStmt:
			defers++
			if !isShutdownRun(n.Call.Fun) {
				t.Errorf("run defers %s; push it onto the shutdown stack instead", exprName(n.Call.Fun))
			}
		case *ast.KeyValueExpr:
			if key, ok := n.Key.(*ast.Ident); ok && key.Name == "OnShutdown" && isShutdownRun(n.Value) {
				hooked = true
			}
		}
		return true
	})
	if defers != 1 {
		t.Errorf("run has %d defer statements, want only defer shutdown.run()", defers)
	}
	if !hooked {
		t.Error("application.Options does not set OnShutdown: shutdown.run")
	}
}

func isShutdownRun(expr ast.Expr) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	recv, ok := sel.X.(*ast.Ident)
	return ok && recv.Name == "shutdown" && sel.Sel.Name == "run"
}

func exprName(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		return exprName(e.X) + "." + e.Sel.Name
	default:
		return "a function value"
	}
}
