//go:build ignore

// de_tenant_users_go updates Go call sites for the de-tenanted users.sql queries.
// It reads internal/db/gen/users.sql.go to learn the new method signatures, then
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
	methods, structFields, err := parseUsersSQLGo("internal/db/gen/users.sql.go")
	if err != nil {
		return err
	}

	dirs := []string{"internal", "cmd"}
	for _, dir := range dirs {
		if err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") {
				return err
			}
			if strings.HasSuffix(path, "_test.go") {
				return nil
			}
			return processFile(path, methods, structFields)
		}); err != nil {
			return err
		}
	}
	return nil
}

func parseUsersSQLGo(path string) (map[string]methodMeta, map[string][]string, error) {
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
		if len(fd.Type.Params.List) < 2 {
			continue
		}
		// first param is context, second is the query arg.
		second := fd.Type.Params.List[1]
		meta := methodMeta{Name: fd.Name.Name}
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

func processFile(path string, methods map[string]methodMeta, structFields map[string][]string) error {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return fmt.Errorf("parsing %s: %w", path, err)
	}

	changed := false
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

		// ctx is first arg
		oldArg := call.Args[1]
		comp, ok := oldArg.(*ast.CompositeLit)
		if !ok {
			return true
		}
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
			// Build a map from field name -> value
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
					// fallback: maybe the field name already matches
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
