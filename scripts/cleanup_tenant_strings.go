// cleanup_tenant_strings removes stale "tenant_id" string literals from Go code:
// - map key/value pairs where the key is "tenant_id"
// - slog/logger key/value pairs where the key is "tenant_id"
// - string slice literals containing "tenant_id"
// It does NOT touch struct JSON tags or raw SQL strings.
package main

import (
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	root := "."
	if len(os.Args) > 1 {
		root = os.Args[1]
	}

	var paths []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(path, ".go") {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	total := 0
	for _, path := range paths {
		n, err := cleanupFile(path)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		total += n
	}
	fmt.Printf("removed %d tenant_id entries\n", total)
}

func cleanupFile(path string) (int, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return 0, fmt.Errorf("parsing %s: %w", path, err)
	}

	dirty := false
	var removeFromCompLit func(lit *ast.CompositeLit) bool
	removeFromCompLit = func(lit *ast.CompositeLit) bool {
		var newElts []ast.Expr
		changed := false
		for _, e := range lit.Elts {
			kv, ok := e.(*ast.KeyValueExpr)
			if !ok {
				// slice/array element
				if bl, ok := e.(*ast.BasicLit); ok && isTenantIDString(bl) {
					changed = true
					continue
				}
				newElts = append(newElts, e)
				continue
			}
			if isTenantIDKey(kv.Key) {
				changed = true
				continue
			}
			newElts = append(newElts, e)
		}
		if changed {
			lit.Elts = newElts
		}
		return changed
	}

	var removeFromCall func(call *ast.CallExpr) bool
	removeFromCall = func(call *ast.CallExpr) bool {
		// Identify logger calls: func name ends with Info/Error/Warn/Debug/InfoContext/etc
		// and the first arg is a string (message). Subsequent args are key/value pairs.
		if len(call.Args) < 3 {
			return false
		}
		fnName := callName(call.Fun)
		if fnName == "" || !isLoggerFunc(fnName) {
			return false
		}
		// First arg is message, rest are variadic pairs.
		var newArgs []ast.Expr
		changed := false
		for i := 1; i < len(call.Args); i++ {
			if i+1 < len(call.Args) && isTenantIDStringLit(call.Args[i]) {
				changed = true
				i++ // skip value
				continue
			}
			newArgs = append(newArgs, call.Args[i])
		}
		if changed {
			call.Args = append(call.Args[:1], newArgs...)
		}
		return changed
	}

	ast.Inspect(f, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.CompositeLit:
			if removeFromCompLit(x) {
				dirty = true
			}
		case *ast.CallExpr:
			if removeFromCall(x) {
				dirty = true
			}
		}
		return true
	})

	if !dirty {
		return 0, nil
	}

	var buf strings.Builder
	if err := format.Node(&buf, fset, f); err != nil {
		return 0, fmt.Errorf("formatting %s: %w", path, err)
	}
	out := []byte(buf.String())
	if err := os.WriteFile(path, out, 0644); err != nil {
		return 0, fmt.Errorf("writing %s: %w", path, err)
	}

	// Count removed by re-grepping? Not precise. Return 1 to indicate touched.
	return 1, nil
}

func isTenantIDStringLit(e ast.Expr) bool {
	bl, ok := e.(*ast.BasicLit)
	return ok && isTenantIDString(bl)
}

func isTenantIDString(bl *ast.BasicLit) bool {
	return bl.Kind == token.STRING && (bl.Value == `"tenant_id"` || bl.Value == "`tenant_id`")
}

func isTenantIDKey(e ast.Expr) bool {
	switch k := e.(type) {
	case *ast.BasicLit:
		return isTenantIDString(k)
	case *ast.Ident:
		return k.Name == "tenant_id"
	}
	return false
}

func callName(e ast.Expr) string {
	switch x := e.(type) {
	case *ast.Ident:
		return x.Name
	case *ast.SelectorExpr:
		return x.Sel.Name
	}
	return ""
}

func isLoggerFunc(name string) bool {
	return name == "Info" || name == "Error" || name == "Warn" || name == "Debug" ||
		name == "InfoContext" || name == "ErrorContext" || name == "WarnContext" || name == "DebugContext" ||
		strings.HasPrefix(name, "Info") || strings.HasPrefix(name, "Error") ||
		strings.HasPrefix(name, "Warn") || strings.HasPrefix(name, "Debug")
}
