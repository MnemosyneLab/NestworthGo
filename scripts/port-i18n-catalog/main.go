// Command port-i18n-catalog parses internal/i18n/i18n.go's `translations`
// map literal (English/Simplified Chinese/Traditional Chinese) plus
// internal/i18n/errors.go's `errorTranslations` map literal, and emits one
// i18next resource JSON file per locale under frontend/src/i18n/.
//
// This is a one-off/occasional maintenance tool for the Wails v3 migration
// (docs/migration/wails-v3-implementation-plan.md Phase 3/5): the catalog
// **content** is the input for the new i18next resources, not something to
// redesign from scratch (technical design Sec8). Run it again with
// `go run ./scripts/port-i18n-catalog` any time internal/i18n gains new
// keys before Phase 6 retires the Go package. It never writes to
// internal/i18n; it only reads it.
package main

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"log"
	"os"
	"path/filepath"
	"sort"
)

type localeCatalog map[string]string

func main() {
	root, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}
	// Allow running from either the repo root or scripts/port-i18n-catalog.
	repoRoot := root
	if filepath.Base(root) == "port-i18n-catalog" {
		repoRoot = filepath.Join(root, "..", "..")
	}

	en := localeCatalog{}
	zhCN := localeCatalog{}
	zhTW := localeCatalog{}

	mustExtract(filepath.Join(repoRoot, "internal/i18n/i18n.go"), "translations", en, zhCN, zhTW)
	mustExtract(filepath.Join(repoRoot, "internal/i18n/errors.go"), "errorTranslations", en, zhCN, zhTW)

	outDir := filepath.Join(repoRoot, "frontend/src/i18n/locales")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		log.Fatal(err)
	}
	writeJSON(filepath.Join(outDir, "en.json"), en)
	writeJSON(filepath.Join(outDir, "zh-CN.json"), zhCN)
	writeJSON(filepath.Join(outDir, "zh-TW.json"), zhTW)

	// fieldLabelKeys maps a domain.Error.Field wire value (e.g.
	// "accountId") to a catalog key (e.g. "error.field.accountID"), not to
	// translated text directly, so it is locale-independent and ported as
	// its own flat JSON map rather than merged into the three locale
	// files above.
	fieldLabelKeys := extractStringMap(filepath.Join(repoRoot, "internal/i18n/errors.go"), "fieldLabelKeys")
	writeFlatJSON(filepath.Join(outDir, "fieldLabelKeys.json"), fieldLabelKeys)

	fmt.Printf("Ported %d keys to %s\n", len(en), outDir)
}

// mustExtract parses one Go source file and extracts a
// map[string]translation{"key": {"en", "zh-CN", "zh-TW"}, ...} literal
// named varName, writing each language's value into the corresponding
// output map.
func mustExtract(path, varName string, en, zhCN, zhTW localeCatalog) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
	if err != nil {
		log.Fatalf("parse %s: %v", path, err)
	}

	var found bool
	ast.Inspect(file, func(n ast.Node) bool {
		valueSpec, ok := n.(*ast.ValueSpec)
		if !ok || len(valueSpec.Names) != 1 || valueSpec.Names[0].Name != varName {
			return true
		}
		if len(valueSpec.Values) != 1 {
			return true
		}
		composite, ok := valueSpec.Values[0].(*ast.CompositeLit)
		if !ok {
			return true
		}
		for _, elt := range composite.Elts {
			kv, ok := elt.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			key := mustStringLit(kv.Key)
			valueComposite, ok := kv.Value.(*ast.CompositeLit)
			if !ok || len(valueComposite.Elts) != 3 {
				log.Fatalf("%s: key %q: expected a 3-element translation literal", path, key)
			}
			en[key] = mustStringLit(valueComposite.Elts[0])
			zhCN[key] = mustStringLit(valueComposite.Elts[1])
			zhTW[key] = mustStringLit(valueComposite.Elts[2])
		}
		found = true
		return false
	})
	if !found {
		log.Fatalf("%s: variable %q not found", path, varName)
	}
}

// extractStringMap extracts a simple map[string]string (or
// map[domain.ErrorCode]string) literal's string-literal keys and values.
// Non-string-literal keys (e.g. domain.ErrCode identifiers) are skipped,
// since fieldLabelKeys is the only caller today and its keys are always
// plain field-name string literals.
func extractStringMap(path, varName string) map[string]string {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
	if err != nil {
		log.Fatalf("parse %s: %v", path, err)
	}
	result := map[string]string{}
	var found bool
	ast.Inspect(file, func(n ast.Node) bool {
		valueSpec, ok := n.(*ast.ValueSpec)
		if !ok || len(valueSpec.Names) != 1 || valueSpec.Names[0].Name != varName {
			return true
		}
		composite, ok := valueSpec.Values[0].(*ast.CompositeLit)
		if !ok {
			return true
		}
		for _, elt := range composite.Elts {
			kv, ok := elt.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			keyLit, ok := kv.Key.(*ast.BasicLit)
			if !ok || keyLit.Kind != token.STRING {
				continue
			}
			key := mustStringLit(kv.Key)
			result[key] = mustStringLit(kv.Value)
		}
		found = true
		return false
	})
	if !found {
		log.Fatalf("%s: variable %q not found", path, varName)
	}
	return result
}

func writeFlatJSON(path string, values map[string]string) {
	file, err := os.Create(path)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(values); err != nil {
		log.Fatal(err)
	}
}

func mustStringLit(expr ast.Expr) string {
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		log.Fatalf("expected a string literal, got %T", expr)
	}
	value, err := stringLitValue(lit.Value)
	if err != nil {
		log.Fatal(err)
	}
	return value
}

func stringLitValue(raw string) (string, error) {
	var value string
	_, err := fmt.Sscanf(raw, "%q", &value)
	if err == nil {
		return value, nil
	}
	// Fall back to strconv.Unquote-equivalent handling if Sscanf's %q ever
	// mishandles a literal (it should not for this file's ASCII-quoted,
	// UTF-8-content strings, but fail loudly instead of silently emitting
	// wrong data).
	return "", fmt.Errorf("could not unquote string literal %s: %w", raw, err)
}

func writeJSON(path string, catalog localeCatalog) {
	keys := make([]string, 0, len(catalog))
	for key := range catalog {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	nested := map[string]any{}
	for _, key := range keys {
		setNested(nested, key, catalog[key])
	}

	file, err := os.Create(path)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(nested); err != nil {
		log.Fatal(err)
	}
}

// setNested turns a dotted key like "accounts.create" into a nested JSON
// object {"accounts": {"create": ...}}, matching i18next's conventional
// namespaced-key JSON shape.
func setNested(root map[string]any, dottedKey, value string) {
	parts := splitDots(dottedKey)
	node := root
	for i, part := range parts {
		if i == len(parts)-1 {
			node[part] = value
			return
		}
		next, ok := node[part].(map[string]any)
		if !ok {
			next = map[string]any{}
			node[part] = next
		}
		node = next
	}
}

func splitDots(s string) []string {
	var parts []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '.' {
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	parts = append(parts, s[start:])
	return parts
}
