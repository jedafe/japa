//go:build ignore

// remove_unused_tenantid_params removes function/method parameters named tenantID
// that are not used in the body, and updates same-package calls and interfaces.
package main

import (
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func main() {
	base := "/home/jmobile/websites/japa/japaconnect"

	// Gather package groups
	pkgs := make(map[string][]string)
	filepath.Walk(base, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") || strings.Contains(path, "/scripts/") {
			return err
		}
		if strings.HasSuffix(path, "_test.go") {
			return nil
		}
		dir := filepath.Dir(path)
		pkgs[dir] = append(pkgs[dir], path)
		return nil
	})

	for _, files := range pkgs {
		sort.Strings(files)
		if err := processPackage(files); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
}

func processPackage(files []string) error {
	type fileInfo struct {
		path string
		fset *token.FileSet
		file *ast.File
	}
	var infos []*fileInfo
	fset := token.NewFileSet()

	for _, p := range files {
		f, err := parser.ParseFile(fset, p, nil, parser.ParseComments)
		if err != nil {
			return err
		}
		infos = append(infos, &fileInfo{path: p, fset: fset, file: f})
	}

	// Map of function/method signatures to the parameter position to remove.
	// Key: "FuncName" or "Receiver.FuncName" (without tenantID argument)
	// We store the position to remove and whether it's a method.
	type sigKey struct {
		recv    string // empty for func, receiver type name for method
		name    string
		numArgs int
	}
	var toRemove = make(map[sigKey][]int) // positions to remove

	for _, fi := range infos {
		ast.Inspect(fi.file, func(n ast.Node) bool {
			fn, ok := n.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				return true
			}
			params := paramNames(fn.Type.Params)
			var unused []int
			for i, n := range params {
				if n == "tenantID" && !usedIn(fn.Body, "tenantID") {
					unused = append(unused, i)
				}
			}
			if len(unused) == 0 {
				return true
			}

			key := sigKey{
				name:    fn.Name.Name,
				numArgs: len(params),
			}
			if fn.Recv != nil && len(fn.Recv.List) > 0 {
				key.recv = receiverTypeName(fn.Recv.List[0].Type)
			}

			toRemove[key] = unused
			return true
		})
	}

	if len(toRemove) == 0 {
		return nil
	}

	// Helper to update a function declaration signature.
	removeParams := func(fl *ast.FieldList, positions []int) *ast.FieldList {
		if fl == nil {
			return nil
		}
		var newList []*ast.Field
		pos := 0
		for _, f := range fl.List {
			for _, n := range f.Names {
				if !containsInt(positions, pos) {
					// keep the field, but if it has multiple names we need split
					// For simplicity, create a new field with a single name.
					newList = append(newList, &ast.Field{
						Names: []*ast.Ident{n},
						Type:  f.Type,
						Tag:   f.Tag,
					})
				}
				pos++
			}
		}
		fl.List = newList
		return fl
	}

	// Apply to declarations (including interfaces)
	for _, fi := range infos {
		ast.Inspect(fi.file, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.FuncDecl:
				key := sigKey{name: x.Name.Name, numArgs: len(paramNames(x.Type.Params))}
				if x.Recv != nil && len(x.Recv.List) > 0 {
					key.recv = receiverTypeName(x.Recv.List[0].Type)
				}
				if positions, ok := toRemove[key]; ok {
					x.Type.Params = removeParams(x.Type.Params, positions)
					// also remove from results? no
				}
			case *ast.FuncLit:
				// We don't handle anonymous functions here.
			case *ast.InterfaceType:
				for _, m := range x.Methods.List {
					fn, ok := m.Type.(*ast.FuncType)
					if !ok {
						continue
					}
					if len(m.Names) != 1 {
						continue
					}
					key := sigKey{recv: "", name: m.Names[0].Name, numArgs: len(paramNames(fn.Params))}
					if positions, ok := toRemove[key]; ok {
						fn.Params = removeParams(fn.Params, positions)
					}
				}
			}
			return true
		})

		// Update call sites.
		ast.Inspect(fi.file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			key := sigKey{}
			switch fn := call.Fun.(type) {
			case *ast.Ident:
				key.name = fn.Name
			case *ast.SelectorExpr:
				if id, ok := fn.X.(*ast.Ident); ok {
					key.recv = id.Name // variable name, not type; weak heuristic
					key.name = fn.Sel.Name
				}
			}
			if key.name == "" {
				return true
			}
			// The function may have other signatures; we match by original arg count.
			// We iterate all toRemove keys with matching name and call len.
			for k, positions := range toRemove {
				if k.name != key.name || k.recv == "" {
					continue
				}
				// For methods, match by number of arguments (original signature).
				if len(call.Args) == k.numArgs {
					call.Args = removeArgs(call.Args, positions)
				}
			}
			// Also handle package-level functions.
			for k, positions := range toRemove {
				if k.name != key.name || k.recv != "" {
					continue
				}
				if len(call.Args) == k.numArgs {
					call.Args = removeArgs(call.Args, positions)
				}
			}
			return true
		})
	}

	// Write files
	for _, fi := range infos {
		tmp := fi.path + ".detmp"
		out, err := os.Create(tmp)
		if err != nil {
			return err
		}
		if err := format.Node(out, fi.fset, fi.file); err != nil {
			out.Close()
			os.Remove(tmp)
			return fmt.Errorf("formatting %s: %w", fi.path, err)
		}
		out.Close()
		if err := os.Rename(tmp, fi.path); err != nil {
			return err
		}
	}
	return nil
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

func usedIn(node ast.Node, name string) bool {
	found := false
	ast.Inspect(node, func(n ast.Node) bool {
		if found {
			return false
		}
		ident, ok := n.(*ast.Ident)
		if ok && ident.Name == name {
			found = true
			return false
		}
		return true
	})
	return found
}

func containsInt(a []int, v int) bool {
	for _, x := range a {
		if x == v {
			return true
		}
	}
	return false
}

func receiverTypeName(expr ast.Expr) string {
	switch x := expr.(type) {
	case *ast.StarExpr:
		return typeName(x.X)
	case *ast.Ident:
		return x.Name
	default:
		return ""
	}
}

func typeName(expr ast.Expr) string {
	switch x := expr.(type) {
	case *ast.Ident:
		return x.Name
	case *ast.StarExpr:
		return typeName(x.X)
	case *ast.SelectorExpr:
		return typeName(x.X) + "." + x.Sel.Name
	default:
		return ""
	}
}

func removeArgs(args []ast.Expr, positions []int) []ast.Expr {
	if len(positions) == 0 {
		return args
	}
	var out []ast.Expr
	for i, a := range args {
		if !containsInt(positions, i) {
			out = append(out, a)
		}
	}
	return out
}
