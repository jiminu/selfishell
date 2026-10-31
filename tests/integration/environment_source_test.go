package integration

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// Product code reads the CLI's injected Environment, so its tests can supply a
// private one and run in parallel instead of changing the process environment.
func TestIntegrationProductReadsInjectedEnvironment(t *testing.T) {
	t.Parallel()
	files, err := filepath.Glob("../../internal/selfishell/*.go")
	if err != nil || len(files) == 0 {
		t.Fatalf("product sources: %v %v", files, err)
	}
	banned := map[string]bool{"os.Getenv": true, "os.LookupEnv": true, "os.Environ": true, "os.Setenv": true, "os.Unsetenv": true, "os.Clearenv": true, "os.ExpandEnv": true, "exec.LookPath": true}
	fset := token.NewFileSet()
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range file.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && fn.Recv == nil && fn.Name.Name == "ProcessEnvironment" && filepath.Base(name) == "environment.go" {
				continue // the single default-environment constructor
			}
			ast.Inspect(decl, func(n ast.Node) bool {
				if sel, ok := n.(*ast.SelectorExpr); ok {
					if pkg, ok := sel.X.(*ast.Ident); ok && banned[pkg.Name+"."+sel.Sel.Name] {
						t.Errorf("%s: %s.%s; read the injected Environment instead", fset.Position(sel.Pos()), pkg.Name, sel.Sel.Name)
					}
				}
				return true
			})
		}
	}
}
