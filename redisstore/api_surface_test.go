package redisstore

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// TestExportedAPISurface_NamesNoGoRedisType statically parses this package's
// own non-test source and asserts that no exported declaration's signature
// names a type from github.com/redis/go-redis/v9 (NFR-07): a consumer must
// never need to import go-redis to call redisstore's API. It is a real
// assertion over the parsed source, not a claim resting on review alone.
func TestExportedAPISurface_NamesNoGoRedisType(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("Glob error = %v", err)
	}

	fset := token.NewFileSet()
	var violations []string

	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, parser.ParseComments)
		if err != nil {
			t.Fatalf("ParseFile(%q) error = %v", name, err)
		}

		redisAlias := importAlias(f, "github.com/redis/go-redis/v9")

		ast.Inspect(f, func(n ast.Node) bool {
			switch decl := n.(type) {
			case *ast.FuncDecl:
				if !decl.Name.IsExported() {
					return true
				}
				if redisAlias != "" && refersToPackage(decl.Type, redisAlias) {
					violations = append(violations, fmt.Sprintf("%s:%d func %s", name, fset.Position(decl.Pos()).Line, decl.Name.Name))
				}
			case *ast.TypeSpec:
				if !decl.Name.IsExported() || redisAlias == "" {
					return true
				}
				// An unexported field is not part of the type's exported
				// signature: a caller cannot name it, so a go-redis type
				// there is an implementation detail, not a leak. Only
				// exported fields of an exported struct, and any non-struct
				// exported type (an alias, a func type, ...), count.
				st, isStruct := decl.Type.(*ast.StructType)
				if !isStruct {
					if refersToPackage(decl.Type, redisAlias) {
						violations = append(violations, fmt.Sprintf("%s:%d type %s", name, fset.Position(decl.Pos()).Line, decl.Name.Name))
					}
					return true
				}
				for _, field := range st.Fields.List {
					if !fieldIsExported(field) {
						continue
					}
					if refersToPackage(field.Type, redisAlias) {
						violations = append(violations, fmt.Sprintf("%s:%d type %s field", name, fset.Position(field.Pos()).Line, decl.Name.Name))
					}
				}
			}
			return true
		})
	}

	if len(violations) > 0 {
		t.Fatalf("exported declarations naming a go-redis type:\n%s", strings.Join(violations, "\n"))
	}
}

// goRedisPackageName is go-redis's own declared package name. It is not
// derivable from its import path by convention: the path's last segment is
// the major-version suffix "v9", not the identifier the package clause
// declares, which is "redis". A generic derivation would silently mismatch
// exactly the import this test exists to catch.
const goRedisImportPath = "github.com/redis/go-redis/v9"
const goRedisPackageName = "redis"

// importAlias returns the local name file uses for importPath, or "" if it
// is not imported. importPath must be goRedisImportPath: the fallback name
// for an unaliased import is only known correct for that one path (see
// goRedisPackageName).
func importAlias(f *ast.File, importPath string) string {
	if importPath != goRedisImportPath {
		panic("importAlias: only " + goRedisImportPath + " is supported")
	}
	for _, imp := range f.Imports {
		path := strings.Trim(imp.Path.Value, `"`)
		if path != importPath {
			continue
		}
		if imp.Name != nil {
			return imp.Name.Name
		}
		return goRedisPackageName
	}
	return ""
}

// fieldIsExported reports whether field has at least one exported name, or
// is an embedded (unnamed) field whose type name is exported.
func fieldIsExported(field *ast.Field) bool {
	if len(field.Names) == 0 {
		// Embedded field: its type IS its name.
		if ident, ok := field.Type.(*ast.Ident); ok {
			return ident.IsExported()
		}
		return false
	}
	for _, n := range field.Names {
		if n.IsExported() {
			return true
		}
	}
	return false
}

// refersToPackage reports whether any selector expression within n qualifies
// with pkgAlias, e.g. "redis.Client" when pkgAlias is "redis".
func refersToPackage(n ast.Node, pkgAlias string) bool {
	found := false
	ast.Inspect(n, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if ident, ok := sel.X.(*ast.Ident); ok && ident.Name == pkgAlias {
			found = true
		}
		return true
	})
	return found
}
