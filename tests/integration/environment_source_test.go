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
	var files []string
	for _, dir := range []string{"selfishell", "release", "terminal", "process", "managed", "report"} {
		matches, err := filepath.Glob("../../internal/" + dir + "/*.go")
		if err != nil || len(matches) == 0 {
			t.Fatalf("%s sources: %v %v", dir, matches, err)
		}
		files = append(files, matches...)
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
			if fn, ok := decl.(*ast.FuncDecl); ok && fn.Recv == nil && fn.Name.Name == "HostEnvironment" && filepath.ToSlash(name) == "../../internal/process/environment.go" {
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
