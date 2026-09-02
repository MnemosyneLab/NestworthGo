package architecture_test

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestImportGraph(t *testing.T) {
	root := moduleRoot(t)
	domainDir := filepath.Join(root, "internal", "domain")
	applicationDir := filepath.Join(root, "internal", "application")
	dataDir := filepath.Join(root, "internal", "wailsapi", "data")
	recoveryDir := filepath.Join(root, "internal", "wailsapi", "recovery")

	assertNoImports(t, domainDir, "internal/domain", []string{
		"github.com/waltwang/nestworth-go/internal/application",
		"github.com/waltwang/nestworth-go/internal/infrastructure",
		"github.com/waltwang/nestworth-go/internal/wailsapi",
		"github.com/wailsapp/wails",
		"modernc.org/sqlite",
		"database/sql",
		"net/http",
	})
	assertNoImports(t, applicationDir, "internal/application", []string{
		"github.com/waltwang/nestworth-go/internal/infrastructure",
		"github.com/waltwang/nestworth-go/internal/wailsapi",
		"github.com/wailsapp/wails",
	})
	assertNoImports(t, dataDir, "internal/wailsapi/data", []string{
		"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite",
		"github.com/waltwang/nestworth-go/internal/infrastructure/backup",
		"github.com/waltwang/nestworth-go/internal/infrastructure/csvcodec",
	})
	assertNoImports(t, recoveryDir, "internal/wailsapi/recovery", []string{
		"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite",
		"github.com/waltwang/nestworth-go/internal/infrastructure/backup",
		"github.com/waltwang/nestworth-go/internal/infrastructure/csvcodec",
	})
}

func assertNoImports(t *testing.T, dir, label string, forbidden []string) {
	t.Helper()
	files := productionGoFiles(t, dir)
	if len(files) == 0 {
		t.Fatalf("%s: no production Go files", label)
	}
	fileSet := token.NewFileSet()
	for _, path := range files {
		parsed, err := parser.ParseFile(fileSet, path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, spec := range parsed.Imports {
			imp := strings.Trim(spec.Path.Value, `"`)
			for _, prefix := range forbidden {
				if imp == prefix || strings.HasPrefix(imp, prefix+"/") {
					t.Errorf("%s imports %s from %s", label, imp, path)
				}
			}
		}
	}
}

func productionGoFiles(t *testing.T, dir string) []string {
	t.Helper()
	var files []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		name := d.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			return nil
		}
		files = append(files, path)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}
