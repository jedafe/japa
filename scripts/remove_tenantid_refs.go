// go:build ignore

// remove_tenantid_refs removes TenantID key/value pairs from all struct
// literals and TenantID assignment statements. It does not try to type-check;
// it simply targets the identifier "TenantID" in those two patterns.
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
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	for _, dir := range []string{"internal", "cmd"} {
		if err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") {
				return err
			}
			return processFile(path)
		}); err != nil {
			return err
		}
	}
	return nil
}

func isTenantIDField(e ast.Expr) bool {
	sel, ok := e.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	return sel.Sel.Name == "TenantID"
}

func processFile(path string) error {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return fmt.Errorf("parsing %s: %w", path, err)
	}

	changed := false

	ast.Inspect(f, func(n ast.Node) bool {
		if n == nil {
			return true
		}

		switch x := n.(type) {
		case *ast.KeyValueExpr:
			if ident, ok := x.Key.(*ast.Ident); ok && ident.Name == "TenantID" {
				// Mark for removal in the parent composite literal later.
				// We can't remove from here, so we set Value to a sentinel nil.
				// This is a hack; better to handle in CompositeLit below.
				return true
			}
		case *ast.CompositeLit:
			var newElts []ast.Expr
			for _, elt := range x.Elts {
				kv, ok := elt.(*ast.KeyValueExpr)
				if ok {
					if ident, ok := kv.Key.(*ast.Ident); ok && ident.Name == "TenantID" {
						changed = true
						continue
					}
				}
				newElts = append(newElts, elt)
			}
			x.Elts = newElts
		case *ast.AssignStmt:
			if len(x.Lhs) == 1 && len(x.Rhs) == 1 {
				if isTenantIDField(x.Lhs[0]) {
					// Remove this statement. Replace with empty statement? Easier to nil it.
					// We'll replace with an empty statement in the parent block below.
					changed = true
					return true
				}
			}
		}
		return true
	})

	if !changed {
		return nil
	}

	// Remove assignment statements that are now just TenantID assignments.
	for _, decl := range f.Decls {
		removeTenantIDAssignments(decl)
	}

	// Walk again to remove or replace bare TenantID selector expressions
	// (e.g. in function args, comparisons, etc.) with a zero value.
	ast.Inspect(f, func(n ast.Node) bool {
		// We don't try to fix arbitrary expressions here.
		return true
	})

	tmp := path + ".detmp"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if err := format.Node(out, fset, f); err != nil {
		out.Close()
		os.Remove(tmp)
		return fmt.Errorf("formatting %s: %w", path, err)
	}
	out.Close()
	return os.Rename(tmp, path)
}

func removeTenantIDAssignments(node ast.Node) {
	if node == nil {
		return
	}
	switch n := node.(type) {
	case *ast.FuncDecl:
		if n.Body != nil {
			removeTenantIDAssignments(n.Body)
		}
	case *ast.BlockStmt:
		var newList []ast.Stmt
		for _, s := range n.List {
			assign, ok := s.(*ast.AssignStmt)
			if ok && len(assign.Lhs) == 1 && len(assign.Rhs) == 1 {
				if sel, ok := assign.Lhs[0].(*ast.SelectorExpr); ok {
					if sel.Sel.Name == "TenantID" {
						continue
					}
				}
			}
			newList = append(newList, s)
		}
		n.List = newList
		// Recurse into remaining statements
		for _, s := range n.List {
			removeTenantIDAssignments(s)
		}
	case *ast.IfStmt:
		removeTenantIDAssignments(n.Body)
		removeTenantIDAssignments(n.Else)
	case *ast.ForStmt:
		removeTenantIDAssignments(n.Body)
	case *ast.RangeStmt:
		removeTenantIDAssignments(n.Body)
	case *ast.SwitchStmt:
		removeTenantIDAssignments(n.Body)
	case *ast.TypeSwitchStmt:
		removeTenantIDAssignments(n.Body)
	case *ast.CaseClause:
		var newList []ast.Stmt
		for _, s := range n.Body {
			assign, ok := s.(*ast.AssignStmt)
			if ok && len(assign.Lhs) == 1 && len(assign.Rhs) == 1 {
				if sel, ok := assign.Lhs[0].(*ast.SelectorExpr); ok {
					if sel.Sel.Name == "TenantID" {
						continue
					}
				}
			}
			newList = append(newList, s)
		}
		n.Body = newList
		for _, s := range n.Body {
			removeTenantIDAssignments(s)
		}
	}
}
