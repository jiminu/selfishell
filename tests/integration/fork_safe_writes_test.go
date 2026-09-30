package integration

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"testing"
)

// Parallel tests fork constantly. A file written outside testutil's ForkLock
// can be inherited open for writing, and executing it then fails with ETXTBSY.
func TestIntegrationWritesHoldForkLock(t *testing.T) {
	t.Parallel()
	files, err := filepath.Glob("*.go")
	if err != nil || len(files) == 0 {
		t.Fatalf("integration sources: %v %v", files, err)
	}
	fset := token.NewFileSet()
	for _, name := range files {
		file, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			if sel, ok := n.(*ast.SelectorExpr); ok {
				if pkg, ok := sel.X.(*ast.Ident); ok && pkg.Name == "os" {
					switch sel.Sel.Name {
					case "Create", "CreateTemp", "OpenFile", "WriteFile":
						t.Errorf("%s: os.%s; use testutil.WriteFile or testutil.AppendFile", fset.Position(sel.Pos()), sel.Sel.Name)
					}
				}
			}
			return true
		})
	}
}
