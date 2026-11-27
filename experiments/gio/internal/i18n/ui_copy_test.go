package i18n

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"unicode"
)

// Check whole presentation packages, including helpers that do not import Gio
// themselves. Looking at literal definitions catches text passed through a
// variable, model, option, formatter or a new rendering wrapper.
func TestPresentationCopyUsesCatalog(t *testing.T) {
	files := token.NewFileSet()
	packages := make(map[string][]*ast.File)
	tree := os.DirFS("../..")
	err := fs.WalkDir(tree, ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if name != "." {
				if strings.HasPrefix(entry.Name(), ".") || entry.Name() == "vendor" || entry.Name() == "testdata" {
					return fs.SkipDir
				}
				// Test tooling that renders frames to images. Its errors reach
				// developers running tests, never a household.
				if name == "internal/snapshot" {
					return fs.SkipDir
				}
				// The developer-only chart gallery renders synthetic English
				// sample data to check shared chart components. It is not part
				// of the household UI and never ships in the application.
				if name == "tools/chartgallery" {
					return fs.SkipDir
				}
				if _, err := fs.Stat(tree, path.Join(name, "go.mod")); err == nil {
					return fs.SkipDir
				} else if !errors.Is(err, fs.ErrNotExist) {
					return err
				}
			}
			return nil
		}
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			return nil
		}
		source, err := fs.ReadFile(tree, name)
		if err != nil {
			return err
		}
		// Resolve local identifiers so a local variable shadowing
		// the log/slog import cannot acquire a diagnostic exception.
		file, err := parser.ParseFile(files, name, source, 0)
		if err != nil {
			return err
		}
		dir := path.Dir(name)
		packages[dir] = append(packages[dir], file)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	var checked []string
	for dir, pkg := range packages {
		if !slices.ContainsFunc(pkg, importsPresentation) {
			continue
		}
		checked = append(checked, dir)
		for _, file := range pkg {
			for _, literal := range uncatalogedText(file) {
				t.Errorf("%s: localize user-facing text %s in all message catalogs", files.Position(literal.Pos()), literal.Value)
			}
		}
	}
	for _, dir := range []string{".", "internal/desktop", "internal/ui"} {
		if !slices.Contains(checked, dir) {
			t.Fatalf("presentation package %s was not scanned", dir)
		}
	}
}

func importsPresentation(file *ast.File) bool {
	for _, imp := range file.Imports {
		path, _ := strconv.Unquote(imp.Path.Value)
		if strings.HasPrefix(path, "gioui.org/") || path == "github.com/waltwang/nestworth-go/experiments/gio/internal/ui" {
			return true
		}
	}
	return false
}

var numericFormat = regexp.MustCompile(`%(?:0[0-9]+)?[ds]`)
var currencyCode = regexp.MustCompile(`^[A-Z]{3}$`)
var machineID = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
var diagnosticFieldKey = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

func uncatalogedText(file *ast.File) []*ast.BasicLit {
	var violations []*ast.BasicLit
	ast.PreorderStack(file, nil, func(node ast.Node, parents []ast.Node) bool {
		lit, ok := node.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		value, _ := strconv.Unquote(lit.Value)
		if strings.IndexFunc(value, unicode.IsLetter) >= 0 && !nonDisplayLiteral(lit, value, parents) && !diagnosticLogLiteral(file, lit, parents) && !accessibilityContextKey(file, value, parents) && !(file.Name.Name == "main" && launcherLiteral(value, parents)) {
			violations = append(violations, lit)
		}
		return true
	})
	return violations
}

// These exceptions describe where data identifiers and language-neutral formats
// are used. The same strings still fail if written as a label or placeholder.
func nonDisplayLiteral(lit *ast.BasicLit, value string, parents []ast.Node) bool {
	parent := parents[len(parents)-1]
	switch n := parent.(type) {
	case *ast.ImportSpec, *ast.Field:
		return true // Import paths and struct tags.
	case *ast.ValueSpec:
		if typ, ok := n.Type.(*ast.SelectorExpr); ok && named(typ.X, "domain") && typ.Sel.Name == "CurrencyCode" {
			return currencyCode.MatchString(value)
		}
		// Font family names are data identifiers for the text shaper.
		if typ, ok := n.Type.(*ast.SelectorExpr); ok && named(typ.X, "font") && typ.Sel.Name == "Typeface" {
			return true
		}
		if typ, ok := n.Type.(*ast.SelectorExpr); ok && named(typ.X, "charts") && typ.Sel.Name == "ID" {
			return machineID.MatchString(value)
		}
		return named(n.Type, "routeID") && machineID.MatchString(value)
	case *ast.KeyValueExpr:
		if named(n.Key, "ID") && machineID.MatchString(value) {
			return true
		}
		if literal, ok := parents[len(parents)-2].(*ast.CompositeLit); ok {
			if typ, ok := literal.Type.(*ast.SelectorExpr); ok {
				if named(typ.X, "key") && typ.Sel.Name == "Filter" && named(n.Key, "Name") {
					return len(value) == 1 && value[0] >= 'A' && value[0] <= 'Z'
				}
				if named(typ.X, "clipboard") && typ.Sel.Name == "WriteCmd" && named(n.Key, "Type") {
					return value == "application/text"
				}
			}
		}
		// A published review filter is a query value, not visible status copy.
		if named(n.Key, "Status") && machineID.MatchString(value) {
			if literal, ok := parents[len(parents)-2].(*ast.CompositeLit); ok {
				typ, ok := literal.Type.(*ast.SelectorExpr)
				return ok && named(typ.X, "domain") && typ.Sel.Name == "ReviewAlertFilter"
			}
		}
		return false
	case *ast.CaseClause:
		return slices.Contains(n.List, ast.Expr(lit))
	case *ast.BinaryExpr:
		return n.Op == token.EQL || n.Op == token.NEQ
	case *ast.CallExpr:
		if len(n.Args) == 0 {
			return false
		}
		if named(n.Fun, "newItem") && n.Args[0] == lit {
			return machineID.MatchString(value)
		}
		callee, ok := n.Fun.(*ast.SelectorExpr)
		if !ok {
			return false
		}
		switch {
		case callee.Sel.Name == "HistoricalOverview":
			// The third argument accepts the HistoricalOverview comparison sentinel.
			return len(n.Args) == 3 && n.Args[2] == lit && value == "current"
		case named(callee.X, "app") && callee.Sel.Name == "Title":
			return value == "Nestworth" || value == "Nestworth Gio"
		case named(callee.X, "format") && callee.Sel.Name == "Money":
			return len(n.Args) > 1 && n.Args[1] == lit && currencyCode.MatchString(value)
		case named(callee.X, "filepath") && callee.Sel.Name == "Glob":
			// The Darwin text shaper looks for an installed system font asset.
			return n.Args[0] == lit && value == "/System/Library/AssetsV2/com_apple_MobileAsset_Font*/*.asset/AssetData/PingFang.ttc"
		case named(callee.X, "fmt") && callee.Sel.Name == "Sprintf" && n.Args[0] == lit:
			withoutNumbers := numericFormat.ReplaceAllString(value, "0")
			if strings.IndexFunc(withoutNumbers, unicode.IsLetter) < 0 {
				return true
			}
			field, ok := parents[len(parents)-2].(*ast.KeyValueExpr)
			return ok && named(field.Key, "ID") && machineID.MatchString(withoutNumbers)
		}
	}
	return false
}

// Only literal messages and machine field keys passed directly to the
// standard logger's Info/Warn functions are diagnostic copy. Do not exempt
// a package/file, an identifier's definition, string values of log fields,
// indirect calls or shadowed receivers. Those still go through the guard.
func diagnosticLogLiteral(file *ast.File, lit *ast.BasicLit, parents []ast.Node) bool {
	call, ok := parents[len(parents)-1].(*ast.CallExpr)
	if !ok || len(call.Args) == 0 {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || (sel.Sel.Name != "Info" && sel.Sel.Name != "Warn") {
		return false
	}
	receiver, ok := sel.X.(*ast.Ident)
	if !ok || receiver.Obj != nil {
		return false
	}
	standardLogger := slices.ContainsFunc(file.Imports, func(imp *ast.ImportSpec) bool {
		importPath, _ := strconv.Unquote(imp.Path.Value)
		if importPath != "log/slog" {
			return false
		}
		name := "slog"
		if imp.Name != nil {
			name = imp.Name.Name
		}
		return receiver.Name == name
	})
	if !standardLogger {
		return false
	}
	if call.Args[0] == lit {
		return true
	}
	// slog also accepts Attrs and arbitrary arguments. Recognize only the
	// unambiguous alternating key/value form with literal machine keys.
	if len(call.Args)%2 == 0 || call.Ellipsis.IsValid() {
		return false
	}
	for i := 1; i < len(call.Args); i += 2 {
		key, ok := call.Args[i].(*ast.BasicLit)
		if !ok || key.Kind != token.STRING {
			return false
		}
		name, err := strconv.Unquote(key.Value)
		if err != nil || !diagnosticFieldKey.MatchString(name) {
			return false
		}
	}
	index := slices.Index(call.Args, ast.Expr(lit))
	return index > 0 && index%2 == 1
}

// The bridge uses a namespaced layout.Context key. Its accessible names
// and descriptions still go through the normal catalog guard.
func accessibilityContextKey(file *ast.File, value string, parents []ast.Node) bool {
	if file.Name.Name != "accessibility" || value != "nestworth.accessibility" {
		return false
	}
	spec, ok := parents[len(parents)-1].(*ast.ValueSpec)
	return ok && len(spec.Names) == 1 && spec.Names[0].Name == "frameKey"
}

// The module entry point also parses CLI commands and loads bundle assets.
// Only the arguments of those concrete operations are exempt.
func launcherLiteral(value string, parents []ast.Node) bool {
	for _, ancestor := range slices.Backward(parents) {
		switch n := ancestor.(type) {
		case *ast.BinaryExpr, *ast.ParenExpr:
			continue // For example, a brand prefix concatenated into a CLI error.
		case *ast.CallExpr:
			if named(n.Fun, "loadBrandImage") {
				return value == "mark.png" || value == "wordmark.png"
			}
			sel, ok := n.Fun.(*ast.SelectorExpr)
			if !ok {
				return false
			}
			switch {
			case named(sel.X, "errors"):
				return sel.Sel.Name == "New"
			case named(sel.X, "fmt"):
				return sel.Sel.Name == "Errorf" || sel.Sel.Name == "Println" || sel.Sel.Name == "Fprintln"
			case named(sel.X, "filepath"):
				return sel.Sel.Name == "Join" && value == "Resources"
			}
		}
		return false
	}
	return false
}

func named(expr ast.Expr, name string) bool {
	id, ok := expr.(*ast.Ident)
	return ok && id.Name == name
}

func TestPresentationCopyGuardDetectsInlineAndIndirectText(t *testing.T) {
	source := `package main
import "gioui.org/io/semantic"
import "log/slog"
const title = "Settings"
const route routeID = "settings"
func layout() {
	view.Button(click, "save")
	view.Message("保存", ink)
	view.Label(gtx, title, size, weight, ink)
	semantic.DescriptionOp("説明")
	style.Hint = "Search institutions"
	option := Choice{ID: "checking", Label: "Checking"}
	hint := fmt.Sprintf("Timezone: %s", zone)
	view.Tooltip(gtx, hint)
	view.Message("Nestworth Gio", ink)
	material.Label(theme, 14, "Save")
	fmt.Fprintln(os.Stderr, brand + " could not start:")
	errors.New("expected a command")
	format.Money(value, "EUR", prefs)
	app.Title("Nestworth Gio")
	view.Label(gtx, language.Text(SettingsGeneral), size, weight, ink)
	newItem("device-settings", SettingsGeneral, icon, nil)
	_ = Widget{ID: fmt.Sprintf("test-%02d", index)}
	_ = key.Filter{Name: "C"}
	_ = clipboard.WriteCmd{Type: "application/text"}
	slog.Info("application starting")
}`
	file, err := parser.ParseFile(token.NewFileSet(), "preview.go", source, 0)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, literal := range uncatalogedText(file) {
		value, _ := strconv.Unquote(literal.Value)
		got = append(got, value)
	}
	want := []string{"Settings", "save", "保存", "説明", "Search institutions", "Checking", "Timezone: %s", "Nestworth Gio", "Save"}
	if !slices.Equal(got, want) {
		t.Fatalf("flagged %q, want %q", got, want)
	}
}

func TestPresentationCopyGuardAllowsOnlyDirectDiagnosticMessagesAndKeys(t *testing.T) {
	for _, tc := range []struct {
		name, source string
		want         []string
	}{
		{
			name: "standard logger and data keys",
			source: `package desktop
import "log/slog"
func layout() {
	slog.Info("Gio titlebar native handshake verified",
		"gio_frame_px", frame, "native_buttons", count)
	slog.Warn("restoring system decorations", "native_full_size", false)
	slog.Warn("integrated titlebar unavailable")
	slog.Info("diagnostic", "reason", "Visible field value")
	slog.Info("diagnostic", "reason", "save")
	slog.Info("diagnostic", "invalid field key", count)
	slog.Info("diagnostic", key, "Indirect key value")
	slog.Info("diagnostic", slog.Int("attr", 3), "Mixed value")
	slog.Info("concatenated " + "message")
	const indirect = "Indirect message"
	slog.Info(indirect)
	view.Message("Gio titlebar native handshake verified", ink)
	view.Label(gtx, "native_buttons", size, weight, ink)
	{
		slog := fakeLogger
		slog.Info("Shadowed UI copy")
	}
}`,
			want: []string{"Visible field value", "save", "invalid field key", "Indirect key value", "attr", "Mixed value", "concatenated ", "message", "Indirect message", "Gio titlebar native handshake verified", "native_buttons", "Shadowed UI copy"},
		},
		{
			name: "not the standard logger",
			source: `package desktop
import slog "example.com/presentation"
func layout() { slog.Info("Visible text", "some_key", value) }`,
			want: []string{"Visible text", "some_key"},
		},
		{
			name: "aliased standard logger",
			source: `package desktop
import diagnostics "log/slog"
func layout() {
	diagnostics.Warn("Native geometry unavailable", "native_buttons", 0)
	view.Message("Native geometry unavailable", ink)
}`,
			want: []string{"Native geometry unavailable"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			file, err := parser.ParseFile(token.NewFileSet(), "diagnostics.go", tc.source, 0)
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, literal := range uncatalogedText(file) {
				value, _ := strconv.Unquote(literal.Value)
				got = append(got, value)
			}
			if !slices.Equal(got, tc.want) {
				t.Fatalf("flagged %q, want %q", got, tc.want)
			}
		})
	}
}
