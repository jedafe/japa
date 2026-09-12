//go:build ignore

// de_tenant_all_go updates Go call sites for all de-tenanted sqlc queries.
// It reads internal/db/gen/*.sql.go to learn the new method signatures, then
// walks internal/ and cmd/ packages and rewrites calls to gen.Queries methods.
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

type methodMeta struct {
	Name       string
	ArgStruct  string // e.g. "GetUserByIDParams"; empty for positional
	Positional []string
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	genFiles, err := filepath.Glob("internal/db/gen/*.sql.go")
	if err != nil {
		return err
	}

	methods := map[string]methodMeta{}
	structFields := map[string][]string{}

	for _, gf := range genFiles {
		m, sf, err := parseGenFile(gf)
		if err != nil {
			return err
		}
		for k, v := range m {
			methods[k] = v
		}
		for k, v := range sf {
			structFields[k] = v
		}
	}

	dirs := []string{"internal", "cmd"}
	for _, dir := range dirs {
		if err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") {
				return err
			}
			return processFile(path, methods, structFields)
		}); err != nil {
			return err
		}
	}
	return nil
}

func parseGenFile(path string) (map[string]methodMeta, map[string][]string, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return nil, nil, fmt.Errorf("parsing %s: %w", path, err)
	}

	structFields := map[string][]string{}
	for _, decl := range f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.TYPE {
			continue
		}
		for _, spec := range gd.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			st, ok := ts.Type.(*ast.StructType)
			if !ok {
				continue
			}
			var fields []string
			for _, field := range st.Fields.List {
				for _, name := range field.Names {
					fields = append(fields, name.Name)
				}
			}
			structFields[ts.Name.Name] = fields
		}
	}

	methods := map[string]methodMeta{}
	for _, decl := range f.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok || fd.Recv == nil || len(fd.Recv.List) == 0 {
			continue
		}
		meta := methodMeta{Name: fd.Name.Name}
		if len(fd.Type.Params.List) < 2 {
			methods[fd.Name.Name] = meta
			continue
		}
		// first param is context, second is the query arg.
		second := fd.Type.Params.List[1]
		switch t := second.Type.(type) {
		case *ast.Ident:
			if strings.HasSuffix(t.Name, "Params") {
				meta.ArgStruct = t.Name
			} else {
				for _, name := range second.Names {
					meta.Positional = append(meta.Positional, name.Name)
				}
			}
		case *ast.StarExpr:
			if id, ok := t.X.(*ast.Ident); ok && strings.HasSuffix(id.Name, "Params") {
				meta.ArgStruct = id.Name
			} else {
				for _, name := range second.Names {
					meta.Positional = append(meta.Positional, name.Name)
				}
			}
		default:
			// positional: collect param names
			for _, name := range second.Names {
				meta.Positional = append(meta.Positional, name.Name)
			}
		}
		methods[fd.Name.Name] = meta
	}

	return methods, structFields, nil
}

func receiverIsGen(expr ast.Expr) bool {
	switch x := expr.(type) {
	case *ast.Ident:
		// common variable names for *gen.Queries
		switch x.Name {
		case "q", "queries", "Queries", "read", "Read", "write", "Write", "tx", "txq", "txQueries", "db", "r", "w", "qr", "qw", "query":
			return true
		}
	case *ast.SelectorExpr:
		final := x.Sel.Name
		switch final {
		case "read", "Read", "write", "Write", "queries", "Queries", "tx", "txq", "txQueries", "db", "q", "r", "w", "qr", "qw", "query":
			return true
		}
	case *ast.CallExpr:
		// gen.New(...) or gen.NewWithOptions(...) return *Queries;
		// *.Queries(), *.Read(), *.Write() also return *Queries.
		if sel, ok := x.Fun.(*ast.SelectorExpr); ok {
			if ident, ok := sel.X.(*ast.Ident); ok && ident.Name == "gen" {
				switch sel.Sel.Name {
				case "New", "NewWithOptions":
					return true
				}
			}
			// any expression ending in a known Queries/Read/Write call
			switch sel.Sel.Name {
			case "Queries", "Query", "Read", "Write", "Reader", "Writer":
				return true
			}
		}
	}
	return false
}

func paramsTypeName(comp *ast.CompositeLit) string {
	ident, ok := comp.Type.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	xIdent, ok := ident.X.(*ast.Ident)
	if !ok || xIdent.Name != "gen" {
		return ""
	}
	return ident.Sel.Name
}

func restructureComposite(comp *ast.CompositeLit, structFields map[string][]string) bool {
	typeName := paramsTypeName(comp)
	if typeName == "" {
		return false
	}
	newFields, ok := structFields[typeName]
	if !ok {
		return false
	}

	var oldKVs []*ast.KeyValueExpr
	for _, elt := range comp.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		k, ok := kv.Key.(*ast.Ident)
		if !ok {
			continue
		}
		if k.Name == "TenantID" {
			continue
		}
		oldKVs = append(oldKVs, kv)
	}

	newFieldSet := map[string]bool{}
	for _, f := range newFields {
		newFieldSet[f] = true
	}

	var newElts []ast.Expr
	for i, kv := range oldKVs {
		k := kv.Key.(*ast.Ident)
		if strings.HasPrefix(k.Name, "Column") {
			if i < len(newFields) {
				newElts = append(newElts, &ast.KeyValueExpr{
					Key:   &ast.Ident{Name: newFields[i]},
					Value: kv.Value,
					Colon: kv.Colon,
				})
			}
			continue
		}
		if newFieldSet[k.Name] {
			newElts = append(newElts, kv)
			continue
		}
		if i < len(newFields) {
			newElts = append(newElts, &ast.KeyValueExpr{
				Key:   &ast.Ident{Name: newFields[i]},
				Value: kv.Value,
				Colon: kv.Colon,
			})
		} else {
			fmt.Fprintf(os.Stderr, "warn: cannot map field %s in %s in .go\n", k.Name, typeName)
		}
	}
	comp.Elts = newElts
	return true
}

func processFile(path string, methods map[string]methodMeta, structFields map[string][]string) error {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return fmt.Errorf("parsing %s: %w", path, err)
	}

	changed := false

	// First pass: restructure any gen.*Params composite literal, not only
	// those that are direct arguments to gen.Queries methods.
	ast.Inspect(f, func(n ast.Node) bool {
		comp, ok := n.(*ast.CompositeLit)
		if !ok {
			return true
		}
		if restructureComposite(comp, structFields) {
			changed = true
		}
		return true
	})

	// Second pass: update gen.Queries method calls.
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		meta, ok := methods[sel.Sel.Name]
		if !ok {
			return true
		}
		if len(call.Args) < 2 {
			return true
		}

		oldArg := call.Args[1]
		comp, ok := oldArg.(*ast.CompositeLit)
		if !ok {
			// Named methods may pass a pre-built Params variable; leave it.
			if meta.ArgStruct != "" {
				return true
			}
			// Positional call. Only rewrite if the receiver is a *gen.Queries.
			if !receiverIsGen(sel.X) {
				return true
			}
			oldCount := len(call.Args) - 1
			newCount := len(meta.Positional)
			if oldCount > newCount {
				call.Args = append([]ast.Expr{call.Args[0]}, call.Args[1+oldCount-newCount:]...)
				changed = true
			} else if oldCount > 0 && newCount > 0 && oldCount == newCount {
				fmt.Fprintf(os.Stderr, "warn: positional call %s has same arg count but tenant may be first arg in %s\n", sel.Sel.Name, path)
			}
			return true
		}

		// Named arg call (composite). Only rewrite if it's a gen.*Params literal.
		ident, ok := comp.Type.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		xIdent, ok := ident.X.(*ast.Ident)
		if !ok || xIdent.Name != "gen" {
			return true
		}

		if meta.ArgStruct != "" {
			// remove TenantID and rename ColumnN fields to match the new Params struct order
			var oldKVs []*ast.KeyValueExpr
			for _, elt := range comp.Elts {
				kv, ok := elt.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				k, ok := kv.Key.(*ast.Ident)
				if !ok {
					continue
				}
				if k.Name == "TenantID" {
					continue
				}
				oldKVs = append(oldKVs, kv)
			}
			newFields := structFields[meta.ArgStruct]
			newFieldSet := map[string]bool{}
			for _, f := range newFields {
				newFieldSet[f] = true
			}
			var newElts []ast.Expr
			for i, kv := range oldKVs {
				k := kv.Key.(*ast.Ident)
				if strings.HasPrefix(k.Name, "Column") {
					if i < len(newFields) {
						newElts = append(newElts, &ast.KeyValueExpr{
							Key:   &ast.Ident{Name: newFields[i]},
							Value: kv.Value,
							Colon: kv.Colon,
						})
					}
					continue
				}
				if newFieldSet[k.Name] {
					newElts = append(newElts, kv)
					continue
				}
				if i < len(newFields) {
					newElts = append(newElts, &ast.KeyValueExpr{
						Key:   &ast.Ident{Name: newFields[i]},
						Value: kv.Value,
						Colon: kv.Colon,
					})
				} else {
					fmt.Fprintf(os.Stderr, "warn: cannot map field %s in %s.%s in %s\n", k.Name, meta.ArgStruct, meta.Name, path)
				}
			}
			comp.Elts = newElts
			changed = true
		} else {
			// positional method: replace composite lit with positional args
			if !receiverIsGen(sel.X) {
				return true
			}
			fieldMap := map[string]ast.Expr{}
			for _, elt := range comp.Elts {
				kv, ok := elt.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				k, ok := kv.Key.(*ast.Ident)
				if !ok {
					continue
				}
				fieldMap[k.Name] = kv.Value
			}
			var newArgs []ast.Expr
			newArgs = append(newArgs, call.Args[0]) // ctx
			skip := false
			for _, param := range meta.Positional {
				field := param
				if param == "id" {
					field = "ID"
				} else {
					field = strings.ToUpper(param[:1]) + param[1:]
				}
				val, ok := fieldMap[field]
				if !ok {
					val, ok = fieldMap[param]
					if !ok {
						fmt.Fprintf(os.Stderr, "warn: cannot map positional param %s for %s in %s\n", param, meta.Name, path)
						skip = true
						break
					}
				}
				newArgs = append(newArgs, val)
			}
			if skip {
				return true
			}
			changed = true
			call.Args = newArgs
		}
		return true
	})

	if !changed {
		return nil
	}

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
