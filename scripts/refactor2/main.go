// remove_tenantid_fields removes TenantID struct fields (and their JSON tags),
// TenantID key/value pairs in struct literals, and TenantID selector
// assignments. It replaces remaining TenantID selector expressions with a
// suitable zero value so the code continues to compile.
package main

import (
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/tools/go/ast/astutil"
	"golang.org/x/tools/go/packages"
)

func main() {
	root := "../.."
	absRoot, err := filepath.Abs(root)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	cfg := &packages.Config{
		Mode: packages.NeedName |
			packages.NeedFiles |
			packages.NeedSyntax |
			packages.NeedTypes |
			packages.NeedTypesInfo |
			packages.NeedImports |
			packages.NeedDeps,
		Dir:   absRoot,
		Tests: false,
	}

	pkgs, err := packages.Load(cfg, "./...")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if packages.PrintErrors(pkgs) > 0 {
		fmt.Fprintln(os.Stderr, "warnings: some packages have errors, continuing with available type info")
	}

	written := make(map[string]bool)
	for _, p := range pkgs {
		if err := removeTenantIDFields(p, written); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	fmt.Println("done")
}

func removeTenantIDFields(p *packages.Package, written map[string]bool) error {
	for _, f := range p.Syntax {
		tok := p.Fset.File(f.Pos())
		path := tok.Name()
		if written[path] {
			continue
		}

		// First pass: remove struct fields and handle non-assignment references.
		astutil.Apply(f, func(c *astutil.Cursor) bool {
			switch n := c.Node().(type) {
			case *ast.StructType:
				removeTenantIDFromStruct(n)
			case *ast.KeyValueExpr:
				if id, ok := n.Key.(*ast.Ident); ok && id.Name == "TenantID" {
					c.Delete()
					return false
				}
			case *ast.SelectorExpr:
				if n.Sel.Name != "TenantID" || !isTenantIDField(p, n) {
					return true
				}
				if isTenantIDLHS(c.Parent(), n) {
					return true
				}
				// Parent is a KeyValueExpr value: the KeyValueExpr deletion above
				// would have already removed it, but if we reach here it means
				// the key was not "TenantID". That can't happen for struct
				// literals (key is the field name), so skip to be safe.
				if _, ok := c.Parent().(*ast.KeyValueExpr); ok {
					return true
				}
				c.Replace(zeroValueExpr(fieldType(p, n)))
			}
			return true
		}, nil)

		// Second pass: remove simple TenantID selector assignments.
		removeTenantIDAssignments(f)

		out, err := formatNode(p.Fset, f)
		if err != nil {
			return fmt.Errorf("formatting %s: %w", path, err)
		}
		if err := os.WriteFile(path, out, 0644); err != nil {
			return fmt.Errorf("writing %s: %w", path, err)
		}
		written[path] = true
	}
	return nil
}

func removeTenantIDFromStruct(st *ast.StructType) {
	if st.Fields == nil {
		return
	}
	var newList []*ast.Field
	for _, field := range st.Fields.List {
		if len(field.Names) == 0 {
			if embeddedName(field.Type) != "TenantID" {
				newList = append(newList, field)
			}
			continue
		}
		kept := make([]*ast.Ident, 0, len(field.Names))
		for _, name := range field.Names {
			if name.Name != "TenantID" {
				kept = append(kept, name)
			}
		}
		if len(kept) == 0 {
			continue
		}
		if len(kept) < len(field.Names) {
			field.Names = kept
		}
		newList = append(newList, field)
	}
	st.Fields.List = newList
}

func embeddedName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return embeddedName(t.X)
	case *ast.SelectorExpr:
		return t.Sel.Name
	default:
		return ""
	}
}

func isTenantIDField(p *packages.Package, sel *ast.SelectorExpr) bool {
	s, ok := p.TypesInfo.Selections[sel]
	if !ok {
		return false
	}
	field, ok := s.Obj().(*types.Var)
	return ok && field.Name() == "TenantID"
}

func fieldType(p *packages.Package, sel *ast.SelectorExpr) types.Type {
	s, ok := p.TypesInfo.Selections[sel]
	if !ok {
		return types.Typ[types.UntypedNil]
	}
	if field, ok := s.Obj().(*types.Var); ok {
		return field.Type()
	}
	return types.Typ[types.UntypedNil]
}

func isTenantIDLHS(parent ast.Node, sel *ast.SelectorExpr) bool {
	assign, ok := parent.(*ast.AssignStmt)
	if !ok {
		return false
	}
	for _, e := range assign.Lhs {
		if e == sel {
			return true
		}
	}
	return false
}

func removeTenantIDAssignments(f *ast.File) {
	astutil.Apply(f, func(c *astutil.Cursor) bool {
		assign, ok := c.Node().(*ast.AssignStmt)
		if !ok {
			return true
		}
		if len(assign.Lhs) == 0 {
			return true
		}
		for _, e := range assign.Lhs {
			sel, ok := e.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "TenantID" {
				return true
			}
		}
		c.Replace(&ast.EmptyStmt{})
		return false
	}, nil)
}

func zeroValueExpr(t types.Type) ast.Expr {
	t = types.Default(t)
	switch t.Underlying().(type) {
	case *types.Pointer, *types.Slice, *types.Map, *types.Chan, *types.Interface, *types.Signature:
		return ast.NewIdent("nil")
	}

	switch underlying := t.Underlying().(type) {
	case *types.Basic:
		switch underlying.Kind() {
		case types.String, types.UntypedString:
			return &ast.BasicLit{Kind: token.STRING, Value: `""`}
		case types.Int, types.Int8, types.Int16, types.Int32, types.Int64,
			types.Uint, types.Uint8, types.Uint16, types.Uint32, types.Uint64,
			types.Uintptr, types.UnsafePointer:
			return &ast.BasicLit{Kind: token.INT, Value: "0"}
		case types.Float32, types.Float64:
			return &ast.BasicLit{Kind: token.FLOAT, Value: "0"}
		case types.Complex64, types.Complex128:
			return &ast.BasicLit{Kind: token.IMAG, Value: "0i"}
		case types.Bool:
			return ast.NewIdent("false")
		default:
			return ast.NewIdent("nil")
		}
	case *types.Named:
		return namedZero(underlying)
	case *types.Array, *types.Struct:
		expr, err := compositeExpr(t)
		if err != nil {
			return ast.NewIdent("nil")
		}
		return expr
	default:
		return ast.NewIdent("nil")
	}
}

func namedZero(n *types.Named) ast.Expr {
	q := func(pkg *types.Package) string {
		return pkg.Name()
	}
	ts := types.TypeString(n, q)

	switch underlying := n.Underlying().(type) {
	case *types.Basic:
		var lit ast.Expr
		switch underlying.Kind() {
		case types.String, types.UntypedString:
			lit = &ast.BasicLit{Kind: token.STRING, Value: `""`}
		case types.Int, types.Int8, types.Int16, types.Int32, types.Int64,
			types.Uint, types.Uint8, types.Uint16, types.Uint32, types.Uint64,
			types.Uintptr, types.UnsafePointer:
			lit = &ast.BasicLit{Kind: token.INT, Value: "0"}
		case types.Float32, types.Float64:
			lit = &ast.BasicLit{Kind: token.FLOAT, Value: "0"}
		case types.Complex64, types.Complex128:
			lit = &ast.BasicLit{Kind: token.IMAG, Value: "0i"}
		case types.Bool:
			lit = ast.NewIdent("false")
		default:
			return ast.NewIdent("nil")
		}
		fun, err := parser.ParseExpr(ts)
		if err != nil {
			return ast.NewIdent("nil")
		}
		return &ast.CallExpr{
			Fun:  fun,
			Args: []ast.Expr{lit},
		}
	case *types.Array, *types.Struct:
		expr, err := compositeExpr(n)
		if err != nil {
			return ast.NewIdent("nil")
		}
		return expr
	case *types.Pointer, *types.Slice, *types.Map, *types.Chan, *types.Interface, *types.Signature:
		return ast.NewIdent("nil")
	case *types.Named:
		return namedZero(underlying)
	default:
		return ast.NewIdent("nil")
	}
}

func compositeExpr(t types.Type) (ast.Expr, error) {
	q := func(pkg *types.Package) string {
		return pkg.Name()
	}
	ts := types.TypeString(t, q)
	exprStr := "(" + ts + "{})"
	expr, err := parser.ParseExpr(exprStr)
	if err != nil {
		return nil, err
	}
	return expr, nil
}

func formatNode(fset *token.FileSet, f *ast.File) ([]byte, error) {
	var buf strings.Builder
	if err := format.Node(&buf, fset, f); err != nil {
		return nil, err
	}
	return []byte(buf.String()), nil
}
