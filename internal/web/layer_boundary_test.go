package web

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// This structural guard checks dependency direction and DTO field types, not
// handler data flow. HTTP contract tests cover the actual response paths.
func TestLayerBoundaries(t *testing.T) {
	err := filepath.WalkDir("..", func(filename string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(filename, ".go") || strings.HasSuffix(filename, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), filename, nil, 0)
		if err != nil {
			return err
		}
		layer := strings.Split(filepath.ToSlash(filename), "/")[1]
		for _, imp := range file.Imports {
			dependency, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				return err
			}
			if (layer == "wallet" || layer == "chain") && strings.HasSuffix(dependency, "/internal/web") || layer == "chain" && strings.HasSuffix(dependency, "/internal/wallet") {
				t.Errorf("%s: inverted dependency %s", filename, dependency)
			}
		}
		if layer != "web" || !strings.HasSuffix(filename, "_dto.go") {
			return nil
		}
		for _, declaration := range file.Decls {
			group, ok := declaration.(*ast.GenDecl)
			if !ok || group.Tok != token.TYPE {
				continue
			}
			for _, specification := range group.Specs {
				definition := specification.(*ast.TypeSpec)
				ast.Inspect(definition.Type, func(node ast.Node) bool {
					selector, ok := node.(*ast.SelectorExpr)
					if !ok {
						return true
					}
					pkg, ok := selector.X.(*ast.Ident)
					if !ok || !(pkg.Name == "time" && selector.Sel.Name == "Time" || pkg.Name == "json" && selector.Sel.Name == "RawMessage") {
						t.Errorf("%s: DTO %s embeds an external type; explicitly map primitive fields", filename, definition.Name)
					}
					return true
				})
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
