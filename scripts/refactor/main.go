// refactor removes unused tenantID parameters from functions, methods, and
// the interfaces they implement, updating all call sites using go/types.
package main

import (
	"fmt"
	"go/ast"
	"go/format"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/tools/go/ast/astutil"
	"golang.org/x/tools/go/packages"
)

type removeParams struct {
	obj       types.Object
	positions []int
}

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

	removals := findUnusedTenantIDParams(pkgs)
	if len(removals) == 0 {
		fmt.Println("no unused tenantID parameters found")
		return
	}
	fmt.Printf("found %d functions/methods with unused tenantID\n", len(removals))

	propagateInterfaces(pkgs, removals)
	fmt.Printf("after interface propagation: %d objects\n", len(removals))

	written := make(map[string]bool)
	for _, p := range pkgs {
		if err := refactorPackage(p, removals, written); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	fmt.Println("done")
}

func findUnusedTenantIDParams(pkgs []*packages.Package) map[types.Object][]int {
	removals := make(map[types.Object][]int)
	for _, p := range pkgs {
		for _, f := range p.Syntax {
			ast.Inspect(f, func(n ast.Node) bool {
				fn, ok := n.(*ast.FuncDecl)
				if !ok || fn.Body == nil {
					return true
				}
				obj := p.TypesInfo.Defs[fn.Name]
				if obj == nil {
					return true
				}
				params := paramNames(fn.Type.Params)
				var pos []int
				for i, name := range params {
					if name != "tenantID" {
						continue
					}
					if !usedIn(p, fn.Body, obj, i) {
						pos = append(pos, i)
					}
				}
				if len(pos) > 0 {
					removals[obj] = pos
				}
				return true
			})
		}
	}
	return removals
}

func usedIn(p *packages.Package, node ast.Node, obj types.Object, i int) bool {
	fn, ok := obj.(*types.Func)
	if !ok {
		return false
	}
	sig := fn.Type().(*types.Signature)
	if i < 0 || i >= sig.Params().Len() {
		return false
	}
	param := sig.Params().At(i)

	found := false
	ast.Inspect(node, func(n ast.Node) bool {
		if found {
			return false
		}
		ident, ok := n.(*ast.Ident)
		if !ok {
			return true
		}
		if use, ok := p.TypesInfo.Uses[ident]; ok && use == param {
			found = true
			return false
		}
		return true
	})
	return found
}

func paramNames(fl *ast.FieldList) []string {
	if fl == nil {
		return nil
	}
	var out []string
	for _, f := range fl.List {
		for _, n := range f.Names {
			out = append(out, n.Name)
		}
	}
	return out
}

func propagateInterfaces(pkgs []*packages.Package, removals map[types.Object][]int) {
	// Build a list of all interface types and all named concrete types.
	var interfaces []*types.Interface
	namedByPkg := make(map[*packages.Package][]*types.Named)
	for _, p := range pkgs {
		for _, f := range p.Syntax {
			for _, d := range f.Decls {
				gen, ok := d.(*ast.GenDecl)
				if !ok || gen.Tok != token.TYPE {
					continue
				}
				for _, s := range gen.Specs {
					ts, ok := s.(*ast.TypeSpec)
					if !ok {
						continue
					}
					obj := p.TypesInfo.Defs[ts.Name]
					if obj == nil {
						continue
					}
					named, ok := obj.Type().(*types.Named)
					if !ok {
						continue
					}
					namedByPkg[p] = append(namedByPkg[p], named)
					if iface, ok := named.Underlying().(*types.Interface); ok {
						interfaces = append(interfaces, iface)
					}
				}
			}
		}
	}

	// For every concrete method being removed, find all interfaces it
	// implements. Remove tenantID from an interface method only when every
	// known concrete implementation of that interface also removes it.
	for obj, pos := range removals {
		fn, ok := obj.(*types.Func)
		if !ok || fn.Type().(*types.Signature).Recv() == nil {
			continue
		}
		recv := fn.Type().(*types.Signature).Recv().Type()
		name := fn.Name()

		for _, iface := range interfaces {
			if !types.Implements(recv, iface) {
				continue
			}
			for i := 0; i < iface.NumMethods(); i++ {
				im := iface.Method(i)
				if im.Name() != name {
					continue
				}
				if allImplementersRemove(pkgs, namedByPkg, iface, name, pos, removals) {
					removals[im] = pos
				}
			}
		}
	}
}

func allImplementersRemove(pkgs []*packages.Package, namedByPkg map[*packages.Package][]*types.Named, iface *types.Interface, name string, pos []int, removals map[types.Object][]int) bool {
	for _, p := range pkgs {
		for _, named := range namedByPkg[p] {
			// Interfaces implement themselves, which would make the interface
			// method look like a non-removed implementation. Skip named types
			// whose underlying type is an interface.
			if _, ok := named.Underlying().(*types.Interface); ok {
				continue
			}

			// Test both value and pointer method sets.
			matched := false
			for _, t := range []types.Type{named, types.NewPointer(named)} {
				if !types.Implements(t, iface) {
					continue
				}
				mset := types.NewMethodSet(t)
				if mset == nil {
					continue
				}
				var found bool
				for i := 0; i < mset.Len(); i++ {
					m := mset.At(i)
					if m.Obj().Name() != name {
						continue
					}
					if _, ok := removals[m.Obj()]; !ok {
						return false
					}
					found = true
					break
				}
				if found {
					matched = true
					break
				}
			}
			if !matched {
				// named does not implement iface, so it is irrelevant.
				continue
			}
		}
	}
	return true
}

func refactorPackage(p *packages.Package, removals map[types.Object][]int, written map[string]bool) error {
	for _, f := range p.Syntax {
		tok := p.Fset.File(f.Pos())
		path := tok.Name()
		if written[path] {
			continue
		}
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		astutil.Apply(f, func(c *astutil.Cursor) bool {
			switch n := c.Node().(type) {
			case *ast.FuncDecl:
				obj := p.TypesInfo.Defs[n.Name]
				if pos, ok := removals[obj]; ok {
					n.Type.Params = removeFields(n.Type.Params, pos)
				}
			case *ast.FuncLit:
				if n.Type == nil || n.Type.Params == nil {
					break
				}
				if t := p.TypesInfo.Types[n.Type]; t.Type != nil {
					if sig, ok := t.Type.(*types.Signature); ok {
						if obj := sig.Recv(); obj != nil {
							if pos, found := removals[obj]; found {
								n.Type.Params = removeFields(n.Type.Params, pos)
							}
						}
					}
				}
			case *ast.CallExpr:
				callee := calleeObject(p, n)
				if pos, ok := removals[callee]; ok {
					n.Args = removeArgs(n.Args, pos)
				}
			case *ast.InterfaceType:
				for _, m := range n.Methods.List {
					ft, ok := m.Type.(*ast.FuncType)
					if !ok || len(m.Names) != 1 {
						continue
					}
					obj := p.TypesInfo.Defs[m.Names[0]]
					if obj == nil {
						continue
					}
					if pos, ok := removals[obj]; ok {
						ft.Params = removeFields(ft.Params, pos)
					}
				}
			}
			return true
		}, nil)

		// Remove local variables that became unused after call arguments
		// were removed.
		removeDeadLocals(p, f)

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

func calleeObject(p *packages.Package, call *ast.CallExpr) types.Object {
	switch fn := call.Fun.(type) {
	case *ast.Ident:
		if obj, ok := p.TypesInfo.Uses[fn]; ok {
			return obj
		}
	case *ast.SelectorExpr:
		if sel, ok := p.TypesInfo.Selections[fn]; ok {
			return sel.Obj()
		}
		if obj, ok := p.TypesInfo.Uses[fn.Sel]; ok {
			return obj
		}
	}
	return nil
}

func removeFields(fl *ast.FieldList, positions []int) *ast.FieldList {
	if fl == nil {
		return nil
	}
	sort.Ints(positions)
	var newList []*ast.Field
	pos := 0
	for _, f := range fl.List {
		var keptNames []*ast.Ident
		for _, n := range f.Names {
			if !containsInt(positions, pos) {
				keptNames = append(keptNames, n)
			}
			pos++
		}
		if len(keptNames) > 0 {
			newList = append(newList, &ast.Field{
				Doc:     f.Doc,
				Names:   keptNames,
				Type:    f.Type,
				Tag:     f.Tag,
				Comment: f.Comment,
			})
		}
	}
	fl.List = newList
	return fl
}

func removeArgs(args []ast.Expr, positions []int) []ast.Expr {
	sort.Ints(positions)
	var out []ast.Expr
	for i, a := range args {
		if !containsInt(positions, i) {
			out = append(out, a)
		}
	}
	return out
}

func containsInt(a []int, v int) bool {
	for _, x := range a {
		if x == v {
			return true
		}
	}
	return false
}

func formatNode(fset *token.FileSet, f *ast.File) ([]byte, error) {
	var buf strings.Builder
	if err := format.Node(&buf, fset, f); err != nil {
		return nil, err
	}
	return []byte(buf.String()), nil
}

// removeDeadLocals removes local variable declarations whose only uses were
// arguments that have just been deleted.
func removeDeadLocals(p *packages.Package, f *ast.File) {
	astutil.Apply(f, func(c *astutil.Cursor) bool {
		switch n := c.Node().(type) {
		case *ast.FuncDecl:
			if n.Body != nil {
				removeDeadLocalsInFunc(p, n, n.Body)
			}
		case *ast.FuncLit:
			if n.Body != nil {
				removeDeadLocalsInFunc(p, nil, n.Body)
			}
		}
		return true
	}, nil)
}

func removeDeadLocalsInFunc(p *packages.Package, fn *ast.FuncDecl, body *ast.BlockStmt) {
	var decls []localDecl
	ast.Inspect(body, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.FuncDecl, *ast.FuncLit:
			return false
		case *ast.AssignStmt:
			if x.Tok == token.DEFINE {
				decls = append(decls, localDecl{stmt: x})
			}
		}
		return true
	})

	for _, d := range decls {
		names := d.names()
		if len(names) == 0 {
			continue
		}
		dead := make(map[string]bool)
		for _, name := range names {
			if !usedInBody(body, d, name) {
				dead[name] = true
			}
		}
		if len(dead) == 0 {
			continue
		}
		d.remove(p, dead)
	}
}

type localDecl struct {
	stmt *ast.AssignStmt
}

func (d localDecl) names() []string {
	var out []string
	for _, e := range d.stmt.Lhs {
		if id, ok := e.(*ast.Ident); ok && id.Name != "_" {
			out = append(out, id.Name)
		}
	}
	return out
}

func usedInBody(body *ast.BlockStmt, d localDecl, name string) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		if found {
			return false
		}
		if n == d.stmt {
			return false
		}
		if id, ok := n.(*ast.Ident); ok && id.Name == name {
			found = true
			return false
		}
		return true
	})
	return found
}

func (d localDecl) remove(p *packages.Package, dead map[string]bool) {
	stmt := d.stmt
	origCount := len(stmt.Lhs)
	newLHS := make([]ast.Expr, 0, origCount)
	hasNonBlank := false
	hasNew := false
	for _, e := range stmt.Lhs {
		id, ok := e.(*ast.Ident)
		if !ok {
			newLHS = append(newLHS, e)
			hasNonBlank = true
			continue
		}
		if dead[id.Name] {
			newLHS = append(newLHS, ast.NewIdent("_"))
		} else {
			newLHS = append(newLHS, id)
			hasNonBlank = true
			if p.TypesInfo.Defs[id] != nil {
				hasNew = true
			}
		}
	}

	if !hasNonBlank {
		// Keep the RHS as a multi-value assignment to blanks so side effects
		// are preserved: _, _ = f() is a valid statement.
		stmt.Lhs = make([]ast.Expr, origCount)
		for i := range stmt.Lhs {
			stmt.Lhs[i] = ast.NewIdent("_")
		}
		stmt.Tok = token.ASSIGN
		return
	}

	stmt.Lhs = newLHS
	if !hasNew {
		stmt.Tok = token.ASSIGN
	}
}
