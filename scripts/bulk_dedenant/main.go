// bulk_dedenant aggressively removes tenantID identifiers from Go source.
// It is intended for an in-development single-tenant migration where a broken
// build during the pass is acceptable.
package main

import (
	"fmt"
	"go/ast"
	"go/format"
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
		Tests: true,
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
		if err := dedenantPackage(p, written); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	fmt.Println("done")
}

func dedenantPackage(p *packages.Package, written map[string]bool) error {
	for _, f := range p.Syntax {
		tok := p.Fset.File(f.Pos())
		path := tok.Name()
		if written[path] {
			continue
		}

		// First pass: remove tenantID from parameter lists and struct fields,
		// and remove corresponding call arguments.
		astutil.Apply(f, func(c *astutil.Cursor) bool {
			switch n := c.Node().(type) {
			case *ast.FuncDecl:
				n.Type.Params = removeTenantIDParams(n.Type.Params)
			case *ast.FuncLit:
				n.Type.Params = removeTenantIDParams(n.Type.Params)
			case *ast.FuncType:
				// Only rewrite Params, never Results. FuncTypes appear in type
				// declarations, interface methods, and function variables.
				n.Params = removeTenantIDParams(n.Params)
			case *ast.InterfaceType:
				// Interface methods: remove tenantID from each method's parameter list.
				n.Methods = removeTenantIDFromInterfaceMethods(n.Methods)
			case *ast.StructType:
				n.Fields = removeTenantIDFromStructFields(n.Fields)
			case *ast.CallExpr:
				n.Args = removeTenantIDCallArgs(p, n)
				rewriteTenantIDCallExpr(p, c, n)
			case *ast.KeyValueExpr:
				if id, ok := n.Key.(*ast.Ident); ok && isTenantID(id.Name) {
					c.Delete()
					return false
				}
			}
			return true
		}, nil)

		// Second pass: remove variable declarations and references.
		astutil.Apply(f, func(c *astutil.Cursor) bool {
			switch n := c.Node().(type) {
			case *ast.Ident:
				if !isTenantID(n.Name) {
					return true
				}
				return replaceOrRemoveIdent(p, c, n)
			case *ast.ValueSpec:
				if removeTenantIDValueSpec(c, n) {
					return false
				}
			case *ast.AssignStmt:
				if removeTenantIDAssign(c, n) {
					return false
				}
			case *ast.RangeStmt:
				if removeTenantIDRange(c, n) {
					return false
				}
			case *ast.SelectorExpr:
				if isTenantID(n.Sel.Name) {
					// x.tenantID (struct field selector)
					c.Replace(zeroValueForExpr(p, n))
					return false
				}
				if id, ok := n.X.(*ast.Ident); ok && isTenantID(id.Name) {
					// tenantID.String, tenantID.Bytes, etc. (method/field value)
					c.Replace(zeroValueForExpr(p, n))
					return false
				}
			}
			return true
		}, nil)

		out, err := formatNode(p.Fset, f)
		if err != nil {
			// We are doing an aggressive automated pass; if a single file can't
			// be formatted (usually because the AST became invalid), skip it and
			// leave it for manual cleanup. Don't stop the rest of the run.
			fmt.Fprintf(os.Stderr, "warning: formatting %s: %v (skipped)\n", path, err)
			written[path] = true
			continue
		}
		if err := os.WriteFile(path, out, 0644); err != nil {
			return fmt.Errorf("writing %s: %w", path, err)
		}
		written[path] = true
	}
	return nil
}

func isTenantID(name string) bool {
	return name == "tenantID" || name == "tenantIDs"
}

func removeTenantIDParams(fl *ast.FieldList) *ast.FieldList {
	if fl == nil {
		return nil
	}
	fl.List = filterTenantIDNamedFields(fl.List)
	return fl
}

func removeTenantIDFromInterfaceMethods(fl *ast.FieldList) *ast.FieldList {
	if fl == nil {
		return nil
	}
	for _, field := range fl.List {
		for _, m := range field.Names {
			if isTenantID(m.Name) {
				// Method itself is named tenantID (unlikely); leave as-is.
				continue
			}
		}
		if ft, ok := field.Type.(*ast.FuncType); ok {
			ft.Params = removeTenantIDParams(ft.Params)
		}
	}
	return fl
}

func removeTenantIDFromStructFields(fl *ast.FieldList) *ast.FieldList {
	if fl == nil {
		return nil
	}
	fl.List = filterTenantIDNamedFields(fl.List)
	return fl
}

// filterTenantIDNamedFields removes fields whose every name is tenantID/tenantIDs.
// Fields with no names (typical for parameter/return type-only entries) are kept.
func filterTenantIDNamedFields(list []*ast.Field) []*ast.Field {
	var out []*ast.Field
	for _, field := range list {
		if len(field.Names) == 0 {
			out = append(out, field)
			continue
		}
		var names []*ast.Ident
		for _, n := range field.Names {
			if !isTenantID(n.Name) {
				names = append(names, n)
			}
		}
		if len(names) == 0 {
			continue
		}
		field.Names = names
		out = append(out, field)
	}
	return out
}

func removeTenantIDCallArgs(p *packages.Package, n *ast.CallExpr) []ast.Expr {
	if len(n.Args) == 0 {
		return n.Args
	}

	// Try to use type info to map arg positions to parameter names.
	remove := make(map[int]bool)
	if p.TypesInfo != nil {
		if sig, ok := callSignature(p, n); ok && sig != nil {
			params := sig.Params()
			for i := range n.Args {
				if i >= params.Len() {
					break
				}
				if isTenantID(params.At(i).Name()) {
					remove[i] = true
				}
			}
		}
	}

	// Also remove any argument that is an identifier named tenantID/tenantIDs,
	// in case type info is missing.
	for i, arg := range n.Args {
		if id, ok := arg.(*ast.Ident); ok && isTenantID(id.Name) {
			remove[i] = true
		}
	}

	var out []ast.Expr
	for i, arg := range n.Args {
		if !remove[i] {
			out = append(out, arg)
		}
	}
	return out
}

func rewriteTenantIDCallExpr(p *packages.Package, c *astutil.Cursor, n *ast.CallExpr) {
	// tenantID.Scan(...) -> (error)(nil) (typed nil error, not a type assertion)
	if isTenantIDScanCall(n) {
		c.Replace(&ast.CallExpr{
			Fun:  &ast.ParenExpr{X: ast.NewIdent("error")},
			Args: []ast.Expr{&ast.Ident{Name: "nil"}},
		})
		return
	}

	// fmt.Sprintf(fmt, tenantID...) and fmt.Errorf(fmt, tenantID...)
	if isSprintfLike(n) {
		n.Args = removeTenantIDFromFormat(p, n.Args)
		return
	}

	// tenantID.String() and other method calls -> zero of the call's result.
	// Skip if this call is an argument to a format-key function; the parent
	// will remove the placeholder and argument together.
	if sel, ok := n.Fun.(*ast.SelectorExpr); ok {
		if id, ok := sel.X.(*ast.Ident); ok && isTenantID(id.Name) {
			if parent, ok := c.Parent().(*ast.CallExpr); ok && isSprintfLike(parent) {
				return
			}
			c.Replace(zeroValueForExpr(p, n))
			return
		}
	}
}

func isTenantIDScanCall(n *ast.CallExpr) bool {
	sel, ok := n.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	if sel.Sel.Name != "Scan" {
		return false
	}
	id, ok := sel.X.(*ast.Ident)
	if !ok {
		return false
	}
	return isTenantID(id.Name)
}

func isSprintfLike(n *ast.CallExpr) bool {
	sel, ok := n.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}

	// fmt.Sprintf / fmt.Errorf
	if pkg, ok := sel.X.(*ast.Ident); ok && pkg.Name == "fmt" {
		return sel.Sel.Name == "Sprintf" || sel.Sel.Name == "Errorf"
	}

	// valkey.MustKey(pattern string, args ...any) string
	if pkg, ok := sel.X.(*ast.Ident); ok && pkg.Name == "valkey" {
		return sel.Sel.Name == "MustKey"
	}

	return false
}

func removeTenantIDFromFormat(p *packages.Package, args []ast.Expr) []ast.Expr {
	if len(args) == 0 {
		return args
	}
	fmtLit, ok := args[0].(*ast.BasicLit)
	if !ok || fmtLit.Kind != token.STRING {
		return args
	}

	remove := make(map[int]bool)
	for i, arg := range args[1:] {
		if isTenantIDExpr(arg) {
			remove[i+1] = true
		}
	}
	if len(remove) == 0 {
		return args
	}

	format := strings.Trim(fmtLit.Value, `"`)
	for i := 1; i < len(args); i++ {
		if !remove[i] {
			continue
		}
		format = removeNthFormatVerb(format, i)
	}

	var out []ast.Expr
	out = append(out, &ast.BasicLit{Kind: token.STRING, Value: fmt.Sprintf("%q", format)})
	for i, arg := range args[1:] {
		if !remove[i+1] {
			out = append(out, arg)
		}
	}
	return out
}

func isTenantIDExpr(e ast.Expr) bool {
	switch x := e.(type) {
	case *ast.Ident:
		return isTenantID(x.Name)
	case *ast.SelectorExpr:
		if id, ok := x.X.(*ast.Ident); ok && isTenantID(id.Name) {
			return true
		}
	case *ast.CallExpr:
		// tenantID.String(), tenantID.Bytes(), tenantID.Scan(...), etc.
		if sel, ok := x.Fun.(*ast.SelectorExpr); ok {
			if id, ok := sel.X.(*ast.Ident); ok && isTenantID(id.Name) {
				return true
			}
		}
	case *ast.UnaryExpr:
		if x.Op == token.AND {
			if id, ok := x.X.(*ast.Ident); ok && isTenantID(id.Name) {
				return true
			}
		}
	}
	return false
}

func removeNthFormatVerb(format string, n int) string {
	// Remove the n-th format verb from a fmt.Sprintf format string.
	// Treats %% as an escaped percent, not a verb.
	var b strings.Builder
	i := 0
	count := 0
	for i < len(format) {
		if format[i] != '%' {
			b.WriteByte(format[i])
			i++
			continue
		}
		if i+1 < len(format) && format[i+1] == '%' {
			b.WriteString("%%")
			i += 2
			continue
		}
		count++
		j := i + 1
		for j < len(format) && isFormatFlagRune(format[j]) {
			j++
		}
		if j < len(format) && isFormatVerb(format[j]) {
			j++
		}
		if count != n {
			b.WriteString(format[i:j])
		}
		i = j
	}
	return b.String()
}

func isFormatFlagRune(r byte) bool {
	return (r >= '0' && r <= '9') || r == '.' || r == '-' || r == '+' || r == '#' || r == ' ' || r == '[' || r == ']'
}

func isFormatVerb(r byte) bool {
	return strings.ContainsRune("v d b o O x X e E f F g G q s t T p", rune(r))
}

func callSignature(p *packages.Package, n *ast.CallExpr) (*types.Signature, bool) {
	if sel, ok := n.Fun.(*ast.SelectorExpr); ok {
		if obj, ok := p.TypesInfo.Selections[sel]; ok {
			if fn, ok := obj.Obj().(*types.Func); ok {
				return fn.Type().(*types.Signature), true
			}
		}
	}
	if t, ok := p.TypesInfo.Types[n.Fun]; ok {
		if sig, ok := t.Type.(*types.Signature); ok {
			return sig, true
		}
	}
	return nil, false
}

func replaceOrRemoveIdent(p *packages.Package, c *astutil.Cursor, n *ast.Ident) bool {
	parent := c.Parent()

	switch parent.(type) {
	// Ident is the declaration site or a key/label/etc. Already handled elsewhere.
	case *ast.Field, *ast.ValueSpec, *ast.KeyValueExpr, *ast.FuncDecl, *ast.TypeSpec, *ast.LabeledStmt:
		return true
	}

	// If this ident is the LHS of an assignment, the parent is AssignStmt (handled).
	if isTenantIDLHS(parent, n) {
		return true
	}

	// If this ident is the base of a selector, replace the whole selector
	// with a zero of the selector's result type.
	if sel, ok := parent.(*ast.SelectorExpr); ok && isSelectorBase(sel, n) {
		zero := zeroValueForExpr(p, sel)
		c.Replace(zero)
		return false
	}

	// Replace with a zero value.
	zero := zeroValueExpr(p, n)
	c.Replace(zero)
	return false
}

func isSelectorBase(sel *ast.SelectorExpr, n *ast.Ident) bool {
	if id, ok := sel.X.(*ast.Ident); ok && id == n {
		return true
	}
	return false
}

func zeroValueForExpr(p *packages.Package, e ast.Expr) ast.Expr {
	if p.TypesInfo != nil {
		if t, ok := p.TypesInfo.Types[e]; ok {
			return zeroForType(t.Type)
		}
	}
	return &ast.CompositeLit{Type: ast.NewIdent("pgtype.UUID")}
}

func isTenantIDLHS(parent ast.Node, n *ast.Ident) bool {
	switch p := parent.(type) {
	case *ast.AssignStmt:
		for _, e := range p.Lhs {
			if e == n {
				return true
			}
		}
	}
	return false
}

func removeTenantIDValueSpec(c *astutil.Cursor, n *ast.ValueSpec) bool {
	// Remove the spec entirely if all names are tenantID.
	if len(n.Names) == 0 {
		return false
	}
	for _, name := range n.Names {
		if !isTenantID(name.Name) {
			return false
		}
	}
	c.Delete()
	return true
}

func removeTenantIDAssign(c *astutil.Cursor, n *ast.AssignStmt) bool {
	if n.Tok != token.DEFINE && n.Tok != token.ASSIGN {
		return false
	}

	hasNonTenant := false
	hasTenant := false
	for _, e := range n.Lhs {
		switch x := e.(type) {
		case *ast.Ident:
			if isTenantID(x.Name) {
				hasTenant = true
			} else {
				hasNonTenant = true
			}
		case *ast.SelectorExpr:
			if isTenantID(x.Sel.Name) {
				hasTenant = true
			} else {
				hasNonTenant = true
			}
		default:
			return false
		}
	}
	if !hasTenant {
		return false
	}
	if !hasNonTenant {
		c.Delete()
		return true
	}

	// If a struct field assignment is involved, delete the whole statement;
	// it's too complex to rewrite while preserving multi-value semantics.
	for _, e := range n.Lhs {
		if sel, ok := e.(*ast.SelectorExpr); ok && isTenantID(sel.Sel.Name) {
			c.Delete()
			return true
		}
	}

	// Multi-value assignment with tenant and non-tenant variables.
	// Replace tenant identifiers with blank identifiers.
	for _, e := range n.Lhs {
		id, ok := e.(*ast.Ident)
		if ok && isTenantID(id.Name) {
			id.Name = "_"
		}
	}
	return true
}

func removeTenantIDRange(c *astutil.Cursor, n *ast.RangeStmt) bool {
	// Leave range statements alone for manual cleanup. The body may need to
	// run once (not zero times) in a single-tenant world, and the loop variable
	// may still be used in the body.
	return false
}

func zeroValueExpr(p *packages.Package, n *ast.Ident) ast.Expr {
	if p.TypesInfo == nil {
		return &ast.CompositeLit{Type: ast.NewIdent("pgtype.UUID")}
	}
	obj := p.TypesInfo.Uses[n]
	if obj == nil {
		obj = p.TypesInfo.Defs[n]
	}
	if obj == nil {
		return &ast.CompositeLit{Type: ast.NewIdent("pgtype.UUID")}
	}
	return zeroForType(obj.Type())
}

func zeroForType(t types.Type) ast.Expr {
	t = types.Unalias(t)
	switch typ := t.(type) {
	case *types.Array:
		if typ.Len() == 0 {
			return &ast.CompositeLit{Type: arrayTypeExpr(typ)}
		}
		return &ast.CompositeLit{
			Type: arrayTypeExpr(typ),
			Elts: []ast.Expr{},
		}
	case *types.Slice:
		return &ast.CompositeLit{Type: sliceTypeExpr(typ)}
	case *types.Map:
		return &ast.CompositeLit{Type: mapTypeExpr(typ)}
	case *types.Pointer:
		return &ast.Ident{Name: "nil"}
	case *types.Chan:
		return &ast.Ident{Name: "nil"}
	case *types.Interface:
		return &ast.Ident{Name: "nil"}
	case *types.Signature:
		return &ast.Ident{Name: "nil"}
	case *types.Named:
		// Return the zero composite literal for struct/basic named types.
		// Interface/function named types get nil.
		switch typ.Underlying().(type) {
		case *types.Interface, *types.Signature:
			return &ast.Ident{Name: "nil"}
		}
		pkg := typ.Obj().Pkg()
		name := typ.Obj().Name()
		if pkg == nil {
			return &ast.CompositeLit{Type: ast.NewIdent(name)}
		}
		return &ast.CompositeLit{Type: &ast.SelectorExpr{X: ast.NewIdent(pkg.Name()), Sel: ast.NewIdent(name)}}
	case *types.Basic:
		switch typ.Kind() {
		case types.Bool:
			return &ast.Ident{Name: "false"}
		case types.Int, types.Int8, types.Int16, types.Int32, types.Int64,
			types.Uint, types.Uint8, types.Uint16, types.Uint32, types.Uint64,
			types.Float32, types.Float64, types.Complex64, types.Complex128:
			return &ast.BasicLit{Kind: token.INT, Value: "0"}
		case types.String:
			return &ast.BasicLit{Kind: token.STRING, Value: `""`}
		default:
			return &ast.Ident{Name: "nil"}
		}
	}
	return &ast.Ident{Name: "nil"}
}

func arrayTypeExpr(t *types.Array) ast.Expr {
	return &ast.ArrayType{Len: &ast.BasicLit{Kind: token.INT, Value: fmt.Sprint(t.Len())}, Elt: typeExpr(t.Elem())}
}

func sliceTypeExpr(t *types.Slice) ast.Expr {
	return &ast.ArrayType{Elt: typeExpr(t.Elem())}
}

func mapTypeExpr(t *types.Map) ast.Expr {
	return &ast.MapType{Key: typeExpr(t.Key()), Value: typeExpr(t.Elem())}
}

func typeExpr(t types.Type) ast.Expr {
	t = types.Unalias(t)
	switch typ := t.(type) {
	case *types.Basic:
		return ast.NewIdent(typ.Name())
	case *types.Named:
		pkg := typ.Obj().Pkg()
		name := typ.Obj().Name()
		if pkg == nil {
			return ast.NewIdent(name)
		}
		return &ast.SelectorExpr{X: ast.NewIdent(pkg.Name()), Sel: ast.NewIdent(name)}
	case *types.Pointer:
		return &ast.StarExpr{X: typeExpr(typ.Elem())}
	case *types.Array:
		return arrayTypeExpr(typ)
	case *types.Slice:
		return sliceTypeExpr(typ)
	case *types.Map:
		return mapTypeExpr(typ)
	}
	return ast.NewIdent("any")
}

func formatNode(fset *token.FileSet, f *ast.File) ([]byte, error) {
	var buf strings.Builder
	if err := format.Node(&buf, fset, f); err != nil {
		return nil, err
	}
	return []byte(buf.String()), nil
}
