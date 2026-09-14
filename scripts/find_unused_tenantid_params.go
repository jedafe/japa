//go:build ignore

// find_unused_tenantid_params lists function/method parameters named tenantID
// that are not referenced in the function body.
package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	base := "/home/jmobile/websites/japa/japaconnect"
	if err := filepath.Walk(base, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") || strings.Contains(path, "/scripts/") {
			return err
		}
		return processFile(path)
	}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func processFile(path string) error {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return nil
	}

	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		if fn.Body == nil {
			continue
		}

		for _, p := range params(fn.Type.Params) {
			if p.name != "tenantID" {
				continue
			}
			if !usedIn(fn.Body, "tenantID") {
				fmt.Printf("%s: %s param %d unused\n", path, fnName(fn), p.pos)
			}
		}
	}
	return nil
}

type paramInfo struct {
	name string
	pos  int
}

func params(fl *ast.FieldList) []paramInfo {
	if fl == nil {
		return nil
	}
	var out []paramInfo
	pos := 0
	for _, f := range fl.List {
		for _, n := range f.Names {
			out = append(out, paramInfo{name: n.Name, pos: pos})
			pos++
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

func fnName(fn *ast.FuncDecl) string {
	if fn.Recv != nil && len(fn.Recv.List) > 0 {
		recv := fn.Recv.List[0]
		var rname string
		if len(recv.Names) > 0 {
			rname = recv.Names[0].Name
		}
		return rname + "." + fn.Name.Name
	}
	return fn.Name.Name
}
